package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// TOKEN: durable record of payment reversals (refunds, chargebacks) applied
// to wallet top-ups. ExternalID is unique per provider, so replaying the
// same notification (webhook retries, several nodes, a callback and a
// webhook for the same event) never removes credit twice.
type TokenPaymentReversal struct {
	Id           int    `json:"id"`
	Provider     string `json:"provider" gorm:"type:varchar(32);uniqueIndex:idx_token_reversal_ext,priority:1"`
	ExternalID   string `json:"external_id" gorm:"type:varchar(191);uniqueIndex:idx_token_reversal_ext,priority:2"`
	TradeNo      string `json:"trade_no" gorm:"type:varchar(255);index"`
	UserId       int    `json:"user_id" gorm:"index"`
	Kind         string `json:"kind" gorm:"type:varchar(16)"` // refund | dispute
	AmountMinor  int64  `json:"amount_minor"`                 // cumulative amount reversed at this point, minor units
	QuotaRemoved int    `json:"quota_removed"`                // credit removed by this record
	CreatedAt    int64  `json:"created_at"`
}

var ErrReversalAlreadyApplied = errors.New("payment reversal already applied")

// ReversalResult reports what ApplyTopUpReversal did.
type ReversalResult struct {
	QuotaRemoved int
	UserId       int
	Balance      int
}

// ApplyTopUpReversal removes wallet credit for a reversal of a successful
// top-up. cumulativeMinor is the total amount reversed on the payment so far
// (Moyasar reports refunds cumulatively); creditPerMinor converts minor
// units of the payment currency to USD credit. The credit removed over all
// reversals of an order never exceeds what the order credited. The balance
// may go negative (credit already spent). externalID must identify this
// reversal state uniquely; a repeat returns ErrReversalAlreadyApplied.
func ApplyTopUpReversal(provider, externalID, kind, tradeNo string, cumulativeMinor int64, creditPerMinor decimal.Decimal) (ReversalResult, error) {
	var res ReversalResult
	if externalID == "" || tradeNo == "" || cumulativeMinor <= 0 || !creditPerMinor.IsPositive() {
		return res, errors.New("invalid reversal")
	}
	err := DB.Transaction(func(tx *gorm.DB) error {
		// Serialize reversals of one order on its top-up row.
		topUp := &TopUp{}
		if err := lockForUpdate(tx).Where("trade_no = ?", tradeNo).First(topUp).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrTopUpNotFound
			}
			return err
		}
		if topUp.PaymentProvider != provider {
			return ErrPaymentMethodMismatch
		}
		if topUp.Status != common.TopUpStatusSuccess {
			return ErrTopUpStatusInvalid
		}
		res.UserId = topUp.UserId

		var existing int64
		if err := tx.Model(&TokenPaymentReversal{}).
			Where("provider = ? AND external_id = ?", provider, externalID).Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return ErrReversalAlreadyApplied
		}

		credited, err := common.WalletQuotaFromDecimalStrict(
			decimal.NewFromFloat(topUp.Money).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil {
			return ErrInvalidTopUpQuota
		}
		target, err := common.WalletQuotaFromDecimalStrict(
			decimal.NewFromInt(cumulativeMinor).Mul(creditPerMinor).Mul(decimal.NewFromFloat(common.QuotaPerUnit)),
		)
		if err != nil {
			return ErrInvalidTopUpQuota
		}
		target = min(target, credited)

		var already int64
		if err := tx.Model(&TokenPaymentReversal{}).Where("trade_no = ?", tradeNo).
			Select("COALESCE(SUM(quota_removed), 0)").Scan(&already).Error; err != nil {
			return err
		}
		remove := max(target-int(already), 0)

		if err := tx.Create(&TokenPaymentReversal{
			Provider: provider, ExternalID: externalID, TradeNo: tradeNo, UserId: topUp.UserId,
			Kind: kind, AmountMinor: cumulativeMinor, QuotaRemoved: remove, CreatedAt: common.GetTimestamp(),
		}).Error; err != nil {
			return err
		}
		if remove > 0 {
			if err := tx.Model(&User{}).Where("id = ?", topUp.UserId).
				Update("quota", gorm.Expr("quota - ?", remove)).Error; err != nil {
				return err
			}
		}
		res.QuotaRemoved = remove
		return nil
	})
	if err != nil {
		return res, err
	}
	if res.QuotaRemoved > 0 {
		if cacheErr := cacheDecrUserQuota(res.UserId, int64(res.QuotaRemoved)); cacheErr != nil {
			common.SysError("reversal: failed to update user quota cache: " + cacheErr.Error())
		}
	}
	res.Balance, _ = GetUserQuota(res.UserId, true)
	return res, nil
}
