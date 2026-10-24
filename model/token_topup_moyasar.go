package model

import (
	"errors"
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// TOKEN: Moyasar (Saudi Arabia) wallet top-ups, charged in SAR through
// Moyasar Invoices. The order's trade_no is "moyasar_<invoice id>", so the
// invoice id is the idempotency key: a paid invoice credits its order once,
// however many callbacks and webhooks report it.
const (
	PaymentMethodMoyasar   = "moyasar"
	PaymentProviderMoyasar = "moyasar"
)

// MoyasarTradeNo is the top-up order number for a Moyasar invoice.
func MoyasarTradeNo(invoiceID string) string {
	return "moyasar_" + invoiceID
}

// RechargeMoyasar credits a pending Moyasar top-up exactly once. The caller
// must already have confirmed with Moyasar's API that the invoice is paid in
// full. credited is false when the order was credited before (a duplicate
// notification); err is non-nil only when crediting failed and should be
// retried.
func RechargeMoyasar(tradeNo string, callerIp string) (credited bool, err error) {
	if tradeNo == "" {
		return false, ErrTopUpNotFound
	}
	var quota int
	topUp := &TopUp{}
	err = DB.Transaction(func(tx *gorm.DB) error {
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if topUp.PaymentProvider != PaymentProviderMoyasar {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status == common.TopUpStatusSuccess {
			return nil // already credited
		}
		if topUp.Status != common.TopUpStatusPending {
			return ErrTopUpStatusInvalid
		}

		var convErr error
		quota, convErr = common.WalletQuotaFromDecimalStrict(
			decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if convErr != nil || quota <= 0 {
			return ErrInvalidTopUpQuota
		}

		topUp.CompleteTime = common.GetTimestamp()
		topUp.Status = common.TopUpStatusSuccess
		if err := tx.Save(topUp).Error; err != nil {
			return err
		}
		credited = true
		return creditTopUpQuota(tx, topUp.UserId, quota, nil)
	})
	if err != nil {
		common.SysError("moyasar topup failed: trade_no=" + tradeNo + " error=" + err.Error())
		return false, err
	}
	if !credited {
		return false, nil
	}
	syncCreditUserQuotaCache(topUp.UserId, quota, "moyasar topup")
	RecordTopupLog(topUp.UserId, fmt.Sprintf("Moyasar top-up credited: %s (order %s)", logger.FormatQuota(quota), tradeNo),
		callerIp, topUp.PaymentMethod, PaymentMethodMoyasar)
	return true, nil
}
