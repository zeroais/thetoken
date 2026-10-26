package service

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TOKEN: the operator alerts flag an account or key spending far above its
// own normal, a sign-up jump, fast spending through a provider and a low or
// falling provider balance; normal activity is not flagged, and a finding is
// mailed once per 6 hours.
func TestOperatorAlertsFlagAbnormalActivityOnly(t *testing.T) {
	truncate(t)
	now := time.Now()
	usd := func(v float64) int { return int(v * common.QuotaPerUnit) }
	consume := func(user, key, channel int, at time.Time, dollars float64) {
		require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: user, TokenId: key, ChannelId: channel, Type: model.LogTypeConsume, CreatedAt: at.Unix(), Quota: usd(dollars)}).Error)
	}
	alpha := model.Channel{Name: "token:alpha:m", Key: "a-key", Status: common.ChannelStatusEnabled}
	beta := model.Channel{Name: "token:beta:m", Key: "b-key", Status: common.ChannelStatusEnabled}
	require.NoError(t, model.DB.Create(&alpha).Error)
	require.NoError(t, model.DB.Create(&beta).Error)

	// Account 1 (key 11): normally ~$0.01/hour, $8 in the last hour.
	consume(1, 11, alpha.Id, now.Add(-48*time.Hour), 1.67)
	consume(1, 11, alpha.Id, now.Add(-10*time.Minute), 8)
	// Account 2 (key 21): spends ~$9/hour every hour, $9 now: its normal.
	for h := 2; h < 7*24; h++ {
		consume(2, 21, beta.Id, now.Add(-time.Duration(h)*time.Hour), 9)
	}
	consume(2, 21, beta.Id, now.Add(-5*time.Minute), 9)
	// 25 sign-ups in the last hour, none before.
	for i := range 25 {
		require.NoError(t, model.DB.Create(&model.User{Username: "spike" + string(rune('a'+i)), AffCode: "spike" + string(rune('a'+i)), CreatedAt: now.Add(-time.Minute).Unix()}).Error)
	}

	// Provider balances: alpha $3.50 (low; a negative credit field minus
	// usage), beta $40 (1/10000 USD as a string) after $60 an hour ago.
	balances := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Header.Get("Authorization") == "Bearer a-key":
			_, _ = w.Write([]byte(`{"credit":-3.75,"recent":0.25}`))
		case r.Header.Get("Authorization") == "Bearer b-key":
			_, _ = w.Write([]byte(`{"availableBalance":"400000"}`))
		default:
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	t.Cleanup(balances.Close)
	tokenRememberBalance("beta", now.Add(-time.Hour), 60)

	setting := &operation_setting.TokenAlertSetting{
		UserHourlyUSD: 5, KeyHourlyUSD: 5, SpendFactor: 5,
		SignupsPerHour: 20, SignupFactor: 5,
		ProviderHourlyUSD: 7, ProviderLowBalanceUSD: 5, ProviderDropUSDPerHour: 10,
		ProviderBalances: map[string]operation_setting.TokenBalanceAPI{
			"alpha": {URL: balances.URL, Fields: []string{"-credit", "-recent"}},
			"beta":  {URL: balances.URL, Fields: []string{"availableBalance"}, Scale: 0.0001},
		},
	}
	alerts := tokenCheckAlerts(t.Context(), now, setting)
	var keys []string
	for _, a := range alerts {
		keys = append(keys, a.Key)
	}
	assert.ElementsMatch(t, []string{
		"spend:user_id:1", "spend:token_id:11", // far above its normal
		"signups",
		"provider-spend:alpha", "provider-spend:beta", // $8 and $9 billed in the hour
		"provider-low:alpha", "provider-drop:beta",
	}, keys, "account 2 and key 21 spend as usual")
	for _, a := range alerts {
		if a.Key == "provider-low:alpha" {
			assert.Contains(t, a.Text, "$3.50")
		}
		if a.Key == "spend:user_id:1" {
			assert.True(t, strings.HasPrefix(a.Text, "Account #1 spent $8.00 in the last hour"), a.Text)
		}
	}

	setting.IgnoreKeyIDs = []int{11} // a status-page monitor's key
	keys = nil
	for _, a := range tokenCheckAlerts(t.Context(), now, setting) {
		keys = append(keys, a.Key)
	}
	assert.NotContains(t, keys, "spend:token_id:11", "an ignored key raises no key alert")
	assert.Contains(t, keys, "spend:user_id:1", "its account is still checked")

	assert.True(t, tokenAlertNew("spend:user_id:1"))
	assert.False(t, tokenAlertNew("spend:user_id:1"), "a finding is mailed once per 6 hours")
}

// TOKEN: the monitors' key warns when its spending limit or its account's
// balance lasts fewer than budget_days at the last 7 days' rate.
func TestBudgetKeyWarnsBeforeItRunsOut(t *testing.T) {
	truncate(t)
	now := time.Now()
	usd := func(v float64) int { return int(v * common.QuotaPerUnit) }
	owner := model.User{Username: "budget-owner", AffCode: "budget-owner", Quota: usd(300)}
	require.NoError(t, model.DB.Create(&owner).Error)
	short := model.Token{UserId: owner.Id, Name: "status-monitors", Key: "budget-short-key", RemainQuota: usd(2.5), Status: common.TokenStatusEnabled}
	long := model.Token{UserId: owner.Id, Name: "plenty", Key: "budget-long-key", RemainQuota: usd(45), Status: common.TokenStatusEnabled}
	require.NoError(t, model.DB.Create(&short).Error)
	require.NoError(t, model.DB.Create(&long).Error)
	for d := range 7 { // $0.25 a day each
		for _, k := range []model.Token{short, long} {
			require.NoError(t, model.LOG_DB.Create(&model.Log{UserId: owner.Id, TokenId: k.Id, Type: model.LogTypeConsume,
				CreatedAt: now.Add(-time.Duration(d)*24*time.Hour - time.Hour).Unix(), Quota: usd(0.25)}).Error)
		}
	}
	setting := &operation_setting.TokenAlertSetting{BudgetKeyIDs: []int{short.Id, long.Id}, BudgetDays: 30}

	alerts := tokenCheckAlerts(t.Context(), now, setting)
	require.Len(t, alerts, 1, "the $2.50 key lasts 10 days; the $45 key lasts 180")
	assert.Equal(t, fmt.Sprintf("key-budget:%d", short.Id), alerts[0].Key)
	assert.Contains(t, alerts[0].Text, "about 10 days left: its spending limit has $2.50")
	assert.Equal(t, 7*24*time.Hour, alerts[0].Repeat, "a reminder a week, not every 6 hours")

	// The account's balance counts too: at $3 the long key's account runs dry first.
	require.NoError(t, model.DB.Model(&model.User{}).Where("id = ?", owner.Id).Update("quota", usd(3)).Error)
	alerts = tokenCheckAlerts(t.Context(), now, setting)
	require.Len(t, alerts, 2)
	assert.Contains(t, alerts[1].Text+alerts[0].Text, "its account's balance has $3.00")
}
