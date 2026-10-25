package controller

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// TOKEN: Moyasar sends no dispute or chargeback events (its webhooks cover
// payments, refunds and voids), so a chargeback the bank or Moyasar reports
// is recorded here by the root user: POST /api/user/moyasar/chargeback with
// the invoice ID, the dispute reference and, for a partial chargeback, the
// SAR (in halalas) reversed on the payment in total so far, refunds
// included. It goes through the same reversal ledger as refunds: the credit
// is removed once per reference, never more than the order credited, the
// balance may go negative, and then the account's API keys are suspended.
type moyasarChargebackRequest struct {
	InvoiceID            string `json:"invoice_id"`
	Reference            string `json:"reference"`
	ReversedTotalHalalas int64  `json:"reversed_total_halalas"` // 0: the whole payment
}

func MoyasarChargeback(c *gin.Context) {
	var req moyasarChargebackRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid request"})
		return
	}
	req.InvoiceID, req.Reference = strings.TrimSpace(req.InvoiceID), strings.TrimSpace(req.Reference)
	if req.InvoiceID == "" || req.Reference == "" || req.ReversedTotalHalalas < 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invoice_id and reference are required"})
		return
	}
	tradeNo := model.MoyasarTradeNo(req.InvoiceID)
	topUp := model.GetTopUpByTradeNo(tradeNo)
	if topUp == nil || topUp.PaymentProvider != model.PaymentProviderMoyasar || topUp.Status != common.TopUpStatusSuccess {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "no credited Moyasar top-up for this invoice"})
		return
	}
	reversed := req.ReversedTotalHalalas
	if reversed == 0 {
		// Whole payment: at least the credit it bought (the fee is not credit).
		reversed = decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromInt(moyasarHalalasPerUSD)).Ceil().IntPart()
	}
	res, err := model.ApplyTopUpReversal(model.PaymentProviderMoyasar, "chargeback:"+req.Reference, "dispute", tradeNo,
		reversed, decimal.NewFromInt(1).Div(decimal.NewFromInt(moyasarHalalasPerUSD)))
	if errors.Is(err, model.ErrReversalAlreadyApplied) {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "this chargeback reference was already recorded"})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	suspended := 0
	if res.Balance < 0 {
		if suspended, err = suspendUserAPIKeys(res.UserId); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
			return
		}
	}
	summary := fmt.Sprintf("Moyasar chargeback %s on order %s (user %d): %s credit removed, balance %s, %d API keys suspended.",
		req.Reference, tradeNo, res.UserId, logger.FormatQuota(res.QuotaRemoved), logger.FormatQuota(res.Balance), suspended)
	model.RecordLog(res.UserId, model.LogTypeManage, summary)
	logger.LogWarn(c, summary)
	service.NotifyRootUser(tokenReversalNotifyType, "TOKEN chargeback recorded", summary)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": summary, "data": gin.H{
		"quota_removed": res.QuotaRemoved, "balance": res.Balance, "keys_suspended": suspended,
	}})
}
