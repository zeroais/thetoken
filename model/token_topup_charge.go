package model

import (
	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm/clause"
)

// TOKEN: what a top-up order charges, itemised in the payment currency, kept
// from the moment the order is created so the order history shows the exact
// amounts even if the fee rate changes later (TopUp keeps only the credit).
// Minor units of Currency (halalas for SAR).
type TokenTopUpCharge struct {
	TradeNo    string `json:"-" gorm:"primaryKey;type:varchar(255)"`
	Currency   string `json:"currency" gorm:"type:varchar(8)"`
	BaseMinor  int64  `json:"base_minor"`  // price of the credit
	FeeMinor   int64  `json:"fee_minor"`   // credit purchase fee
	TotalMinor int64  `json:"total_minor"` // amount charged
	CreatedAt  int64  `json:"-"`
}

// TokenRecordTopUpCharge stores an order's charge (once per order number).
func TokenRecordTopUpCharge(tradeNo, currency string, baseMinor, feeMinor, totalMinor int64) error {
	return DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&TokenTopUpCharge{
		TradeNo: tradeNo, Currency: currency, BaseMinor: baseMinor, FeeMinor: feeMinor, TotalMinor: totalMinor,
		CreatedAt: common.GetTimestamp(),
	}).Error
}

// TokenTopUpWithCharge is a top-up order as the order history lists it.
type TokenTopUpWithCharge struct {
	*TopUp
	Charge *TokenTopUpCharge `json:"charge,omitempty"`
}

// TokenWithCharges attaches each order's recorded charge (orders created
// before charges were recorded, or by gateways that do not record one, have
// none).
func TokenWithCharges(topups []*TopUp) ([]TokenTopUpWithCharge, error) {
	out := make([]TokenTopUpWithCharge, 0, len(topups))
	if len(topups) == 0 {
		return out, nil
	}
	tradeNos := make([]string, 0, len(topups))
	for _, t := range topups {
		tradeNos = append(tradeNos, t.TradeNo)
	}
	var charges []TokenTopUpCharge
	if err := DB.Where("trade_no IN ?", tradeNos).Find(&charges).Error; err != nil {
		return nil, err
	}
	byTradeNo := make(map[string]*TokenTopUpCharge, len(charges))
	for i := range charges {
		byTradeNo[charges[i].TradeNo] = &charges[i]
	}
	for _, t := range topups {
		out = append(out, TokenTopUpWithCharge{TopUp: t, Charge: byTradeNo[t.TradeNo]})
	}
	return out, nil
}
