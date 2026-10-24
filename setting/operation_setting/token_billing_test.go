package operation_setting

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTopupFee(t *testing.T) {
	saved := tokenBillingSetting
	t.Cleanup(func() { tokenBillingSetting = saved })

	for _, tc := range []struct {
		name      string
		rate      float64
		wantRate  float64
		pay       float64 // WithTopupFee(10)
		feeOn1000 int64   // TopupFeeMinor(1000 cents)
	}{
		{"default 5.5%", 0.055, 0.055, 10.55, 55},
		{"disabled", 0, 0, 10, 0},
		{"negative is disabled", -0.1, 0, 10, 0},
		{"NaN is disabled", math.NaN(), 0, 10, 0},
		{"mistyped percent is clamped", 5.5, 0.5, 15, 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tokenBillingSetting.TopupFeeRate = tc.rate
			assert.Equal(t, tc.wantRate, TopupFeeRate())
			assert.InDelta(t, tc.pay, WithTopupFee(10), 1e-9)
			assert.Equal(t, tc.feeOn1000, TopupFeeMinor(1000))
		})
	}

	tokenBillingSetting.TopupFeeRate = 0.055
	// $5 minimum top-up: 500 cents -> 27.5 -> 28 cents (half up).
	assert.Equal(t, int64(28), TopupFeeMinor(500))
	assert.Equal(t, int64(0), TopupFeeMinor(0))
}

func TestWalletOnlyBilling(t *testing.T) {
	prev := tokenBillingSetting.WalletOnly
	t.Cleanup(func() { tokenBillingSetting.WalletOnly = prev })
	tokenBillingSetting.WalletOnly = false
	if WalletOnlyBilling() {
		t.Fatal("wallet-only must be off by default (upstream behavior)")
	}
	tokenBillingSetting.WalletOnly = true
	if !WalletOnlyBilling() {
		t.Fatal("wallet-only must follow the option")
	}
}
