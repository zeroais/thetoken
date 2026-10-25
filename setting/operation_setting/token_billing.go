package operation_setting

import (
	"math"

	"github.com/QuantumNous/new-api/setting/config"
)

// TOKEN fork: credit purchase fee on wallet top-ups. The customer pays the
// top-up amount plus this fee; the credited balance is the amount itself.
// Option key: token_billing.topup_fee_rate (TOKEN sets 0.055 = 5.5% in
// its console settings). The code default is 0 so upstream behavior and
// tests are unchanged until configured.
type TokenBillingSetting struct {
	TopupFeeRate float64 `json:"topup_fee_rate"`
	// WalletOnly bills every request from the prepaid wallet and skips the
	// per-request subscription lookup (TOKEN sells no subscriptions).
	// Option key: token_billing.wallet_only.
	WalletOnly bool `json:"wallet_only"`
	// OutputReservation reserves a request's worst-case output cost before it
	// runs, next to the input estimate, and lowers the request's maximum
	// output to what the balance and the key's spending limit can pay
	// (relay/token_output_reservation.go). Option key:
	// token_billing.output_reservation.
	OutputReservation bool `json:"output_reservation"`
	// ModelOutputTokens is the output a request may produce when the client
	// sets no maximum, per model name and alias (the catalog seed writes it).
	ModelOutputTokens map[string]int `json:"model_output_tokens"`
	// DefaultOutputTokens is used for models not in ModelOutputTokens.
	DefaultOutputTokens int `json:"default_output_tokens"`
	// MinOutputTokens: a request whose affordable output is smaller is refused
	// with an insufficient-balance error instead of an empty reply.
	MinOutputTokens int `json:"min_output_tokens"`
}

// Output reservation defaults when the options are unset or invalid.
const (
	tokenDefaultOutputTokens = 32768
	tokenMinOutputTokens     = 256
)

// maxTopupFeeRate bounds the fee so a mistyped option (5.5 instead of
// 0.055) cannot multiply a customer's charge.
const maxTopupFeeRate = 0.5

var tokenBillingSetting = TokenBillingSetting{}

func init() {
	config.GlobalConfig.Register("token_billing", &tokenBillingSetting)
}

// GetTokenBillingSetting exposes the setting (tests and admin tooling).
func GetTokenBillingSetting() *TokenBillingSetting {
	return &tokenBillingSetting
}

// TopupFeeRate returns the configured fee rate, clamped to [0, 0.5].
func TopupFeeRate() float64 {
	rate := tokenBillingSetting.TopupFeeRate
	if math.IsNaN(rate) || rate <= 0 {
		return 0
	}
	return min(rate, maxTopupFeeRate)
}

// WithTopupFee returns what the customer pays for a top-up whose price
// before the fee is payMoney.
func WithTopupFee(payMoney float64) float64 {
	return payMoney * (1 + TopupFeeRate())
}

// TopupFeeMinor is the fee in a currency's minor units (cents) for a
// top-up whose price before the fee is baseMinor, rounded half up.
func TopupFeeMinor(baseMinor int64) int64 {
	if baseMinor <= 0 {
		return 0
	}
	return int64(math.Floor(float64(baseMinor)*TopupFeeRate() + 0.5))
}

// WalletOnlyBilling reports whether all requests bill from the wallet.
func WalletOnlyBilling() bool {
	return tokenBillingSetting.WalletOnly
}

// TokenOutputTokens is the output a request to model may produce when the
// client sets no maximum.
func TokenOutputTokens(model string) int {
	if n := tokenBillingSetting.ModelOutputTokens[model]; n > 0 {
		return n
	}
	if n := tokenBillingSetting.DefaultOutputTokens; n > 0 {
		return n
	}
	return tokenDefaultOutputTokens
}

// TokenMinOutputTokens is the smallest output a request may be lowered to.
func TokenMinOutputTokens() int {
	if n := tokenBillingSetting.MinOutputTokens; n > 0 {
		return n
	}
	return tokenMinOutputTokens
}
