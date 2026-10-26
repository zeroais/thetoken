package service

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/go-redis/redis/v8"
)

// TOKEN: operator alerts by email (token_alerts.* options). Every
// interval_minutes one instance (a Redis lock) checks the last hour for:
//   - an account or an API key spending far above its own normal (and above a
//     floor), the sign of a stolen key or of abuse;
//   - a jump in sign-ups;
//   - fast spending through one provider, a provider balance below a floor
//     or falling fast (providers with a balance API in
//     token_alerts.provider_balances; the others are covered by the spending
//     check);
//   - an API key used from many client networks (a leaked key).
//
// Each finding is mailed once per 6 hours. Checks read aggregates only.
const tokenAlertRepeat = 6 * time.Hour

type tokenAlert struct {
	Key    string // dedup key: kind and subject
	Text   string
	Repeat time.Duration // how often it is mailed again (0: tokenAlertRepeat)
}

// StartTokenAlerts runs the checks in the background for the process's life:
// once per interval, looked up every minute so a changed interval applies
// without a restart.
func StartTokenAlerts() {
	go func() {
		var lastSlot int64
		for range time.Tick(time.Minute) {
			setting := operation_setting.GetTokenAlertSetting()
			interval := int64(setting.IntervalMinutes) * 60
			if interval <= 0 {
				interval = 600
			}
			slot := time.Now().Unix() / interval
			if slot == lastSlot || !setting.Enabled || setting.Email == "" {
				continue
			}
			lastSlot = slot
			if !tokenAlertsTurn(slot, interval) {
				continue
			}
			alerts := tokenCheckAlerts(context.Background(), time.Now(), setting)
			if err := tokenSendAlerts(alerts, setting.Email); err != nil {
				common.SysError("TOKEN alerts: " + err.Error())
			}
		}
	}()
}

// tokenAlertsTurn reports whether this instance runs this round (one per
// interval across instances); without Redis every instance runs.
func tokenAlertsTurn(slot, intervalSeconds int64) bool {
	if !common.RedisEnabled || common.RDB == nil {
		return true
	}
	key := fmt.Sprintf("token:alerts:run:%d:%d", intervalSeconds, slot)
	ok, err := common.RDB.SetNX(context.Background(), key, 1, time.Duration(intervalSeconds)*time.Second).Result()
	return err == nil && ok
}

func tokenUSD(quota int64) float64 { return float64(quota) / common.QuotaPerUnit }

func tokenQuotaOf(usd float64) int64 { return int64(usd * common.QuotaPerUnit) }

// tokenCheckAlerts returns the findings for the hour before now.
func tokenCheckAlerts(ctx context.Context, now time.Time, s *operation_setting.TokenAlertSetting) []tokenAlert {
	var alerts []tokenAlert
	hourAgo, weekAgo := now.Add(-time.Hour).Unix(), now.Add(-7*24*time.Hour).Unix()
	fail := func(what string, err error) {
		common.SysError(fmt.Sprintf("TOKEN alerts: %s: %v", what, err))
	}

	// Spending far above an account's or a key's own normal.
	for _, group := range []struct {
		column, label string
		floorUSD      float64
	}{{"user_id", "Account", s.UserHourlyUSD}, {"token_id", "API key", s.KeyHourlyUSD}} {
		if group.floorUSD <= 0 {
			continue
		}
		rows, err := model.TokenSpendByGroup(model.LOG_DB, group.column, hourAgo, now.Unix(), tokenQuotaOf(group.floorUSD))
		if err != nil {
			fail(group.column+" spending", err)
			continue
		}
		for _, row := range rows {
			if group.column == "token_id" && slices.Contains(s.IgnoreKeyIDs, row.ID) {
				continue
			}
			before, err := model.TokenSpendOf(model.LOG_DB, group.column, row.ID, weekAgo, hourAgo)
			if err != nil {
				fail(group.column+" baseline", err)
				continue
			}
			hourly := float64(before) / (7*24 - 1)
			if s.SpendFactor > 0 && float64(row.Quota) < s.SpendFactor*hourly {
				continue
			}
			alerts = append(alerts, tokenAlert{
				Key: fmt.Sprintf("spend:%s:%d", group.column, row.ID),
				Text: fmt.Sprintf("%s #%d spent $%.2f in the last hour (its hourly average over the previous 7 days: $%.2f).",
					group.label, row.ID, tokenUSD(row.Quota), tokenUSD(int64(hourly))),
			})
		}
	}

	// Sign-up spike.
	if s.SignupsPerHour > 0 {
		last, err1 := model.TokenSignupsBetween(model.DB, hourAgo, now.Unix())
		before, err2 := model.TokenSignupsBetween(model.DB, weekAgo, hourAgo)
		if err1 != nil || err2 != nil {
			fail("sign-ups", fmt.Errorf("%v %v", err1, err2))
		} else if hourly := float64(before) / (7*24 - 1); last >= int64(s.SignupsPerHour) && (s.SignupFactor <= 0 || float64(last) >= s.SignupFactor*hourly) {
			alerts = append(alerts, tokenAlert{Key: "signups",
				Text: fmt.Sprintf("%d new accounts in the last hour (hourly average over the previous 7 days: %.1f).", last, hourly)})
		}
	}

	// Spending through one provider.
	providers := tokenProviderChannels(s.ProviderBalances)
	if s.ProviderHourlyUSD > 0 {
		rows, err := model.TokenSpendByGroup(model.LOG_DB, "channel_id", hourAgo, now.Unix(), 1)
		if err != nil {
			fail("provider spending", err)
		}
		byProvider := map[string]int64{}
		for _, row := range rows {
			if p := providers.byChannel[row.ID]; p != "" {
				byProvider[p] += row.Quota
			}
		}
		for p, quota := range byProvider {
			if tokenUSD(quota) >= s.ProviderHourlyUSD {
				alerts = append(alerts, tokenAlert{Key: "provider-spend:" + p,
					Text: fmt.Sprintf("Customers were billed $%.2f through provider %s in the last hour.", tokenUSD(quota), p)})
			}
		}
	}

	// Provider balances.
	for p, key := range providers.keys {
		balance, ok := tokenProviderBalance(ctx, p, s.ProviderBalances[p], key)
		if !ok {
			continue
		}
		hourAgoBalance, known := tokenRememberBalance(p, now, balance)
		if s.ProviderLowBalanceUSD > 0 && balance < s.ProviderLowBalanceUSD {
			alerts = append(alerts, tokenAlert{Key: "provider-low:" + p,
				Text: fmt.Sprintf("Provider %s balance is $%.2f (alert below $%.2f). Top it up.", p, balance, s.ProviderLowBalanceUSD)})
		}
		if known && s.ProviderDropUSDPerHour > 0 && hourAgoBalance-balance >= s.ProviderDropUSDPerHour {
			alerts = append(alerts, tokenAlert{Key: "provider-drop:" + p,
				Text: fmt.Sprintf("Provider %s balance fell from $%.2f to $%.2f in about an hour.", p, hourAgoBalance, balance)})
		}
	}

	// Leaked keys: one key used from many networks.
	if s.KeyNetworksPerHour > 0 {
		rows, err := model.TokenSpendByGroup(model.LOG_DB, "token_id", hourAgo, now.Unix(), 0)
		if err != nil {
			fail("key networks", err)
		}
		for _, row := range rows {
			if slices.Contains(s.IgnoreKeyIDs, row.ID) {
				continue
			}
			n := max(TokenKeyNetworkCount(ctx, row.ID, now), TokenKeyNetworkCount(ctx, row.ID, now.Add(-time.Hour)))
			if n >= int64(s.KeyNetworksPerHour) {
				alerts = append(alerts, tokenAlert{Key: fmt.Sprintf("key-networks:%d", row.ID),
					Text: fmt.Sprintf("API key #%d was used from about %d different client networks within an hour: it may be leaked. Check it, and rotate or disable it.", row.ID, n)})
			}
		}
	}
	alerts = append(alerts, tokenBudgetAlerts(ctx, now, s, fail)...)
	sort.Slice(alerts, func(i, j int) bool { return alerts[i].Key < alerts[j].Key })
	return alerts
}

// tokenBudgetAlerts warns when a key in BudgetKeyIDs (the status-page
// monitors' key) has fewer than BudgetDays days of spending left: its
// remaining limit or its account's balance, whichever is lower, divided by
// its average daily spending over the last 7 days. Mailed once a week.
func tokenBudgetAlerts(ctx context.Context, now time.Time, s *operation_setting.TokenAlertSetting, fail func(string, error)) []tokenAlert {
	if len(s.BudgetKeyIDs) == 0 || s.BudgetDays <= 0 {
		return nil
	}
	var alerts []tokenAlert
	for _, id := range s.BudgetKeyIDs {
		key, err := model.GetTokenById(id)
		if err != nil {
			fail(fmt.Sprintf("budget key #%d", id), err)
			continue
		}
		spent, err := model.TokenSpendOf(model.LOG_DB, "token_id", id, now.Add(-7*24*time.Hour).Unix(), now.Unix())
		if err != nil {
			fail(fmt.Sprintf("budget key #%d spending", id), err)
			continue
		}
		if spent <= 0 {
			continue // idle: nothing runs out
		}
		balance, err := model.GetUserQuota(key.UserId, true)
		if err != nil {
			fail(fmt.Sprintf("budget key #%d balance", id), err)
			continue
		}
		left, limit := int64(balance), "its account's balance"
		if !key.UnlimitedQuota && int64(key.RemainQuota) < left {
			left, limit = int64(key.RemainQuota), "its spending limit"
		}
		daily := float64(spent) / 7
		days := float64(max(left, 0)) / daily
		if days >= float64(s.BudgetDays) {
			continue
		}
		alerts = append(alerts, tokenAlert{
			Key:    fmt.Sprintf("key-budget:%d", id),
			Repeat: 7 * 24 * time.Hour,
			Text: fmt.Sprintf("API key #%d (%s) has about %.0f days left: %s has $%.2f, and it spends $%.2f a day (last 7 days). Raise the limit or top up the account before it runs out.",
				id, key.Name, days, limit, tokenUSD(left), tokenUSD(int64(daily))),
		})
	}
	return alerts
}

// tokenProviders maps TOKEN's catalog channels (token:<provider>:<model>) to
// their provider, and keeps one key per provider for balance checks.
type tokenProviders struct {
	byChannel map[int]string
	keys      map[string]string // providers with a balance API only
}

func tokenProviderChannels(balanceAPIs map[string]operation_setting.TokenBalanceAPI) tokenProviders {
	out := tokenProviders{byChannel: map[int]string{}, keys: map[string]string{}}
	var channels []model.Channel
	if err := model.DB.Select("id", "name", "key", "status").Where("name LIKE ?", "token:%").Find(&channels).Error; err != nil {
		common.SysError("TOKEN alerts: channels: " + err.Error())
		return out
	}
	for _, ch := range channels {
		rest, _ := strings.CutPrefix(ch.Name, "token:")
		provider, _, _ := strings.Cut(rest, ":")
		out.byChannel[ch.Id] = provider
		if balanceAPIs[provider].URL != "" && ch.Status == common.ChannelStatusEnabled && out.keys[provider] == "" {
			out.keys[provider] = strings.TrimSpace(strings.Split(ch.Key, "\n")[0])
		}
	}
	return out
}

// tokenProviderBalance reads a provider's prepaid balance in USD.
func tokenProviderBalance(ctx context.Context, provider string, api operation_setting.TokenBalanceAPI, key string) (float64, bool) {
	if api.URL == "" || key == "" || len(api.Fields) == 0 {
		return 0, false
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api.URL, nil)
	if err != nil {
		return 0, false
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		common.SysError(fmt.Sprintf("TOKEN alerts: %s balance: %v", provider, err))
		return 0, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		common.SysError(fmt.Sprintf("TOKEN alerts: %s balance: HTTP %d", provider, resp.StatusCode))
		return 0, false
	}
	var body map[string]any
	if common.DecodeJson(resp.Body, &body) != nil {
		return 0, false
	}
	sum := 0.0
	for _, field := range api.Fields {
		name, negative := strings.CutPrefix(field, "-")
		var v float64
		switch value := body[name].(type) {
		case float64:
			v = value
		case string:
			if v, err = strconv.ParseFloat(value, 64); err != nil {
				return 0, false
			}
		default:
			common.SysError(fmt.Sprintf("TOKEN alerts: %s balance: no numeric field %q", provider, name))
			return 0, false
		}
		if negative {
			v = -v
		}
		sum += v
	}
	if api.Scale != 0 {
		sum *= api.Scale
	}
	return sum, true
}

// Balance samples: Redis (shared by instances), else this process.
var tokenBalanceSamples = struct {
	sync.Mutex
	samples map[string][][2]float64 // provider -> [unix, usd]
}{samples: map[string][][2]float64{}}

// tokenRememberBalance stores the balance seen now and returns the one seen
// about an hour ago (the oldest sample between 50 and 90 minutes old).
func tokenRememberBalance(provider string, now time.Time, usd float64) (float64, bool) {
	ctx := context.Background()
	if common.RedisEnabled && common.RDB != nil {
		key := "token:alerts:balance:" + provider
		common.RDB.ZAdd(ctx, key, &redis.Z{Score: float64(now.Unix()), Member: fmt.Sprintf("%d:%f", now.Unix(), usd)})
		common.RDB.ZRemRangeByScore(ctx, key, "-inf", strconv.FormatInt(now.Add(-3*time.Hour).Unix(), 10))
		common.RDB.Expire(ctx, key, 4*time.Hour)
		old, err := common.RDB.ZRangeByScore(ctx, key, &redis.ZRangeBy{
			Min: strconv.FormatInt(now.Add(-90*time.Minute).Unix(), 10), Max: strconv.FormatInt(now.Add(-50*time.Minute).Unix(), 10), Count: 1,
		}).Result()
		if err != nil || len(old) == 0 {
			return 0, false
		}
		_, value, _ := strings.Cut(old[0], ":")
		v, err := strconv.ParseFloat(value, 64)
		return v, err == nil
	}
	tokenBalanceSamples.Lock()
	defer tokenBalanceSamples.Unlock()
	kept := [][2]float64{}
	for _, s := range tokenBalanceSamples.samples[provider] {
		if s[0] >= float64(now.Add(-3*time.Hour).Unix()) {
			kept = append(kept, s)
		}
	}
	tokenBalanceSamples.samples[provider] = append(kept, [2]float64{float64(now.Unix()), usd})
	for _, s := range kept {
		age := now.Unix() - int64(s[0])
		if age >= 50*60 && age <= 90*60 {
			return s[1], true
		}
	}
	return 0, false
}

// tokenSendAlerts mails the findings not mailed in the last 6 hours.
func tokenSendAlerts(alerts []tokenAlert, to string) error {
	var fresh []tokenAlert
	for _, a := range alerts {
		if tokenAlertNewFor(a.Key, a.Repeat) {
			fresh = append(fresh, a)
		}
	}
	if len(fresh) == 0 {
		return nil
	}
	var body strings.Builder
	body.WriteString("<p>TOKEN found the following in the last hour:</p><ul>")
	for _, a := range fresh {
		body.WriteString("<li>" + html.EscapeString(a.Text) + "</li>")
	}
	body.WriteString("</ul><p>Each finding is sent once every 6 hours. Thresholds: token_alerts.* in the console settings.</p>")
	subject := fmt.Sprintf("TOKEN alert: %d finding(s)", len(fresh))
	return common.SendEmail(subject, to, body.String())
}

var tokenAlertsSent = struct {
	sync.Mutex
	at map[string]time.Time
}{at: map[string]time.Time{}}

func tokenAlertNew(key string) bool {
	return tokenAlertNewFor(key, 0)
}

// tokenAlertNewFor reports whether a finding is due to be mailed (once per
// repeat; 0 means tokenAlertRepeat).
func tokenAlertNewFor(key string, repeat time.Duration) bool {
	if repeat <= 0 {
		repeat = tokenAlertRepeat
	}
	if common.RedisEnabled && common.RDB != nil {
		ok, err := common.RDB.SetNX(context.Background(), "token:alerts:sent:"+key, 1, repeat).Result()
		return err != nil || ok
	}
	tokenAlertsSent.Lock()
	defer tokenAlertsSent.Unlock()
	if at, ok := tokenAlertsSent.at[key]; ok && time.Since(at) < repeat {
		return false
	}
	tokenAlertsSent.at[key] = time.Now()
	return true
}
