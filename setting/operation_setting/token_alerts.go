package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// TOKEN: email alerts to the operator about abuse and fast spending
// (service/token_alerts.go). Option keys token_alerts.*; off by default,
// TOKEN turns them on in its console settings. Money is in USD.
type TokenAlertSetting struct {
	Enabled         bool   `json:"enabled"`
	Email           string `json:"email"`            // where alerts go
	IntervalMinutes int    `json:"interval_minutes"` // how often the checks run (default 10)

	// Spending in the last hour by one account or one API key: alert when it
	// is at least the amount and at least SpendFactor times its average
	// hourly spending over the previous 7 days.
	UserHourlyUSD float64 `json:"user_hourly_usd"`
	KeyHourlyUSD  float64 `json:"key_hourly_usd"`
	SpendFactor   float64 `json:"spend_factor"`

	// Sign-ups in the last hour: at least this many and SignupFactor times
	// the hourly average of the previous 7 days.
	SignupsPerHour int     `json:"signups_per_hour"`
	SignupFactor   float64 `json:"signup_factor"`

	// Providers: billed through one provider in the last hour, a provider
	// balance below a floor, or a balance that fell this much in an hour
	// (providers listed in ProviderBalances).
	ProviderHourlyUSD      float64 `json:"provider_hourly_usd"`
	ProviderLowBalanceUSD  float64 `json:"provider_low_balance_usd"`
	ProviderDropUSDPerHour float64 `json:"provider_drop_usd_per_hour"`
	// Balance APIs by provider (the <provider> of token:<provider>:<model>
	// channels), read with the key of one of its enabled channels.
	ProviderBalances map[string]TokenBalanceAPI `json:"provider_balances"`

	// Leaked keys: one API key used from at least this many client networks
	// in an hour.
	KeyNetworksPerHour int `json:"key_networks_per_hour"`

	// API keys left out of the key checks (spending and networks): TOKEN's
	// own status-page monitors call from many probe addresses on purpose.
	IgnoreKeyIDs []int `json:"ignore_key_ids"`

	// API keys that must not run dry (the status-page monitors' key): alert
	// once a week when, at the last 7 days' spending rate, the key's spending
	// limit or its account's balance (whichever is lower) lasts fewer than
	// BudgetDays days.
	BudgetKeyIDs []int `json:"budget_key_ids"`
	BudgetDays   int   `json:"budget_days"`
}

// TokenBalanceAPI reads a prepaid balance: GET URL with the key as a bearer
// token; the balance is Scale (1 when 0) times the sum of the named top-level
// JSON fields (numbers or numeric strings), a "-" before a name subtracting it.
type TokenBalanceAPI struct {
	URL    string   `json:"url"`
	Fields []string `json:"fields"`
	Scale  float64  `json:"scale"`
}

var tokenAlertSetting = TokenAlertSetting{}

func init() {
	config.GlobalConfig.Register("token_alerts", &tokenAlertSetting)
}

// GetTokenAlertSetting exposes the setting.
func GetTokenAlertSetting() *TokenAlertSetting {
	return &tokenAlertSetting
}
