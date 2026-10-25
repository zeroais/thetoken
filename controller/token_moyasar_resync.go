package controller

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// TOKEN: re-sync one Moyasar order from Moyasar, for a notification that
// never arrived (a webhook with the wrong secret, deleted with its attempt
// history). Root only: POST /api/user/moyasar/resync {"trade_no": ...}, from
// the console's billing history. It changes nothing Moyasar does not confirm
// and goes through the same paths as the notifications: a paid invoice of a
// pending order is credited once (settleMoyasarInvoice), and each refunded
// payment is reversed once per cumulative amount (handleMoyasarRefund,
// token_payment_reversals). Running it again is a no-op.
type moyasarResyncRequest struct {
	TradeNo string `json:"trade_no"`
}

func MoyasarResync(c *gin.Context) {
	ctx := c.Request.Context()
	var req moyasarResyncRequest
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid request"})
		return
	}
	cfg := loadMoyasarConfig()
	if !cfg.usable() {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Moyasar is not configured"})
		return
	}
	invoiceID, ok := strings.CutPrefix(strings.TrimSpace(req.TradeNo), "moyasar_")
	topUp := model.GetTopUpByTradeNo("moyasar_" + invoiceID)
	if !ok || invoiceID == "" || topUp == nil || topUp.PaymentProvider != model.PaymentProviderMoyasar {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "not a Moyasar order"})
		return
	}
	inv, err := fetchMoyasarInvoice(ctx, cfg, invoiceID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Moyasar did not return the invoice: " + err.Error()})
		return
	}

	before, err := model.GetUserQuota(topUp.UserId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if topUp.Status == common.TopUpStatusPending && inv.paidInFull() {
		if settleMoyasarInvoice(ctx, invoiceID, c.ClientIP()) {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "Moyasar or the database did not answer; try again"})
			return
		}
	}
	for _, p := range inv.Payments {
		if p.Refunded <= 0 {
			continue
		}
		if p.InvoiceID == "" {
			p.InvoiceID = invoiceID
		}
		if err := handleMoyasarRefund(ctx, cfg, p); err != nil {
			c.JSON(http.StatusOK, gin.H{"success": false, "message": "refund not applied: " + err.Error()})
			return
		}
	}
	after, err := model.GetUserQuota(topUp.UserId, true)
	if err != nil {
		common.ApiError(c, err)
		return
	}

	summary := fmt.Sprintf("Moyasar order %s re-synced from Moyasar by the root user: invoice %s, balance %s -> %s.",
		topUp.TradeNo, inv.Status, logger.FormatQuota(before), logger.FormatQuota(after))
	if before == after {
		summary = fmt.Sprintf("Moyasar order %s re-synced from Moyasar by the root user: invoice %s, already up to date.", topUp.TradeNo, inv.Status)
	}
	model.RecordLog(topUp.UserId, model.LogTypeManage, summary)
	logger.LogInfo(ctx, summary)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": summary, "data": gin.H{"invoice_status": inv.Status, "balance": after}})
}
