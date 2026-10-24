package controller

import (
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// TOKEN: Moyasar top-ups are priced in USD credit and charged in SAR at the
// fixed peg of 3.75 SAR per USD. Keys and limits come from the environment
// only (never the database or the console):
//
//	MOYASAR_SECRET_KEY       sk_test_… / sk_live_…  (server API calls)
//	MOYASAR_PUBLISHABLE_KEY  pk_test_… / pk_live_…  (must be the same mode)
//	MOYASAR_WEBHOOK_SECRET   shared secret configured on the Moyasar webhook
//	MOYASAR_MIN_TOPUP        minimum top-up in USD credit (default 5)
//	MOYASAR_API_BASE         test stacks only (mock); live keys are refused
const moyasarHalalasPerUSD = 375 // 3.75 SAR x 100 halalas

type moyasarConfig struct {
	secretKey      string
	publishableKey string
	webhookSecret  string
	apiBase        string
	minTopUp       int64
}

func loadMoyasarConfig() moyasarConfig {
	cfg := moyasarConfig{
		secretKey:      strings.TrimSpace(os.Getenv("MOYASAR_SECRET_KEY")),
		publishableKey: strings.TrimSpace(os.Getenv("MOYASAR_PUBLISHABLE_KEY")),
		webhookSecret:  strings.TrimSpace(os.Getenv("MOYASAR_WEBHOOK_SECRET")),
		apiBase:        strings.TrimRight(strings.TrimSpace(os.Getenv("MOYASAR_API_BASE")), "/"),
		minTopUp:       5,
	}
	if v, err := strconv.ParseInt(os.Getenv("MOYASAR_MIN_TOPUP"), 10, 64); err == nil && v > 0 {
		cfg.minTopUp = v
	}
	if cfg.apiBase == "" {
		cfg.apiBase = "https://api.moyasar.com/v1"
	}
	return cfg
}

// usable reports whether the keys are present, of the same mode (test or
// live), and allowed against the configured API base.
func (cfg moyasarConfig) usable() bool {
	if cfg.secretKey == "" || cfg.webhookSecret == "" {
		return false
	}
	live := strings.HasPrefix(cfg.secretKey, "sk_live_")
	if !live && !strings.HasPrefix(cfg.secretKey, "sk_test_") {
		return false
	}
	if cfg.publishableKey != "" && strings.HasPrefix(cfg.publishableKey, "pk_live_") != live {
		return false
	}
	// A mock API base is for test stacks; never send live keys to it.
	if live && cfg.apiBase != "https://api.moyasar.com/v1" {
		return false
	}
	return true
}

func isMoyasarTopUpEnabled() bool {
	return operation_setting.IsPaymentComplianceConfirmed() && loadMoyasarConfig().usable()
}

// moyasarTestMode: test keys (sk_test_) charge nothing.
func moyasarTestMode() bool {
	return !strings.HasPrefix(loadMoyasarConfig().secretKey, "sk_live_")
}

// isMoyasarTopUpEnabledFor: with test keys only administrators may pay (the
// production test top-ups), so customers never get credit for test cards;
// everyone may once the live keys are in (moyasar_mode in Terraform).
func isMoyasarTopUpEnabledFor(c *gin.Context) bool {
	return isMoyasarTopUpEnabled() && (!moyasarTestMode() || c.GetInt("role") >= common.RoleAdminUser)
}

// moyasarQuote is one top-up's price: the USD credit, and the SAR charge in
// halalas split into the credit itself and the credit purchase fee.
type moyasarQuote struct {
	CreditUSD    decimal.Decimal // credited to the wallet
	BaseHalalas  int64           // price of the credit
	FeeHalalas   int64           // credit purchase fee (token_billing.topup_fee_rate), rounded up
	TotalHalalas int64           // invoice amount
}

// quoteMoyasarTopUp prices a top-up of amount USD of credit for a user group,
// with the same group ratio and preset discount as the Stripe gateway.
func quoteMoyasarTopUp(amount int64, group string) (moyasarQuote, bool) {
	if amount <= 0 || amount > 10000 {
		return moyasarQuote{}, false
	}
	groupRatio := common.GetTopupGroupRatio(group)
	if groupRatio <= 0 {
		groupRatio = 1
	}
	discount := 1.0
	if ds, ok := operation_setting.GetPaymentSetting().AmountDiscount[int(amount)]; ok && ds > 0 {
		discount = ds
	}
	credit := decimal.NewFromInt(amount).Mul(decimal.NewFromFloat(groupRatio))
	payUSD := credit.Mul(decimal.NewFromFloat(discount))
	// Never round a charge down: the credit price and the fee each round up
	// to the next halala, so the total does too.
	base := payUSD.Mul(decimal.NewFromInt(moyasarHalalasPerUSD)).Ceil()
	if !base.IsPositive() || !base.LessThan(decimal.NewFromInt(1<<40)) {
		return moyasarQuote{}, false
	}
	fee := base.Mul(decimal.NewFromFloat(operation_setting.TopupFeeRate())).Ceil()
	q := moyasarQuote{CreditUSD: credit, BaseHalalas: base.IntPart(), FeeHalalas: fee.IntPart()}
	q.TotalHalalas = q.BaseHalalas + q.FeeHalalas
	return q, true
}

// sarString formats halalas as SAR with two decimals.
func sarString(halalas int64) string {
	return decimal.New(halalas, -2).StringFixed(2)
}

// usdFromHalalas is the USD equivalent at the fixed peg.
func usdFromHalalas(halalas int64) decimal.Decimal {
	return decimal.NewFromInt(halalas).Div(decimal.NewFromInt(moyasarHalalasPerUSD))
}
