package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTopupFeeLineItem(t *testing.T) {
	setting := operation_setting.GetTokenBillingSetting()
	saved := *setting
	t.Cleanup(func() { *setting = saved })

	setting.TopupFeeRate = 0
	assert.Nil(t, topupFeeLineItem("usd", 2500), "no fee configured, no line item")

	// $25.00 of credit at TOKEN's 5.5% fee.
	setting.TopupFeeRate = 0.055
	item := topupFeeLineItem("usd", 2500)
	require.NotNil(t, item)
	assert.Equal(t, int64(1), *item.Quantity)
	assert.Equal(t, "usd", *item.PriceData.Currency)
	assert.Equal(t, int64(138), *item.PriceData.UnitAmount) // 137.5 -> 138 cents
	assert.Equal(t, "Credit purchase fee (5.5%)", *item.PriceData.ProductData.Name)

	assert.Nil(t, topupFeeLineItem("usd", 0))
}
