package model

import (
	"regexp"
	"strconv"
	"strings"
)

// TOKEN: logs customers see are in English with a normal "$" sign, and each
// consume log names the provider that served the request.
//
// New API writes some log messages in Chinese and formats amounts as
// "＄0.500000 额度". createLog passes every log through TokenLogText, which
// rewrites amounts to "$0.50" and the known Chinese templates to English,
// keeping their values. Anything else is left unchanged.

// TokenStarterCreditLog is the log text of the starter credit (written in
// Chinese by New API, see tokenLogTemplates); rows written before the
// translation still start with tokenStarterCreditLogZh.
const (
	TokenStarterCreditLog   = "Starter credit: "
	tokenStarterCreditLogZh = "新用户注册赠送"
)

var tokenLogMoney = regexp.MustCompile(`[＄$]([0-9]+\.[0-9]+)(?: 额度)?`)

var tokenLogTemplates = []struct {
	zh *regexp.Regexp
	en string
}{
	{regexp.MustCompile(`^新用户注册赠送 (.+)$`), TokenStarterCreditLog + "${1}"},
	{regexp.MustCompile(`^使用邀请码赠送 (.+)$`), "Invitation credit: ${1}"},
	{regexp.MustCompile(`^邀请用户赠送 (.+)$`), "Credit for inviting a user: ${1}"},
	{regexp.MustCompile(`^用户签到，获得额度 (.+)$`), "Check-in credit: ${1}"},
	{regexp.MustCompile(`^通过兑换码充值 (.+)，兑换码ID (\d+)$`), "Redemption code ${2} added ${1}"},
	{regexp.MustCompile(`^使用在线充值成功，充值金额: (.+)，支付金额：(.+)$`), "Online top-up: ${1} added, paid ${2}"},
	{regexp.MustCompile(`^管理员补单成功，充值金额: (.+)，支付金额：(.+)$`), "Top-up completed by an administrator: ${1} added, paid ${2}"},
	{regexp.MustCompile(`^使用Creem充值成功，充值额度: (.+)，支付金额：(.+)$`), "Creem top-up: ${1} added, paid ${2}"},
	{regexp.MustCompile(`^Waffo Pancake充值成功，充值额度: (.+)，支付金额: (.+)$`), "Waffo Pancake top-up: ${1} added, paid ${2}"},
	{regexp.MustCompile(`^Waffo充值成功，充值额度: (.+)，支付金额: (.+)$`), "Waffo top-up: ${1} added, paid ${2}"},
	{regexp.MustCompile(`^订阅购买成功，套餐: (.+)，支付金额: (.+)，支付方式: (.+)$`), "Subscription purchased: ${1}, paid ${2} by ${3}"},
	{regexp.MustCompile(`^使用余额购买订阅成功，套餐: (.+)，支付金额: (.+)，扣除额度: (.+)$`), "Subscription bought from the balance: ${1}, price ${2}, ${3} credit used"},
}

// TokenLogText is a log message in English with "$" amounts.
func TokenLogText(content string) string {
	if content == "" {
		return content
	}
	content = tokenLogMoney.ReplaceAllStringFunc(content, func(m string) string {
		amount := tokenLogMoney.FindStringSubmatch(m)[1]
		v, err := strconv.ParseFloat(amount, 64)
		if err != nil {
			return m
		}
		// Trailing zeros dropped, at least cents: $0.50, $0.00245, $299.995782.
		s := strings.TrimRight(strconv.FormatFloat(v, 'f', 6, 64), "0")
		if whole, frac, _ := strings.Cut(s, "."); len(frac) < 2 {
			s = whole + "." + frac + strings.Repeat("0", 2-len(frac))
		}
		return "$" + s
	})
	for _, t := range tokenLogTemplates {
		if t.zh.MatchString(content) {
			return t.zh.ReplaceAllString(content, t.en)
		}
	}
	return content
}

// tokenLogProvider records in a consume log the provider that served it: the
// tag of TOKEN's catalog channels (token:<provider>:<model>) is the catalog's
// provider name, set when the operator creates the channel. Other channels
// are left unnamed.
func tokenLogProvider(channelID int, other *LogOther) *LogOther {
	ch, err := CacheGetChannel(channelID)
	if err != nil || ch == nil || !strings.HasPrefix(ch.Name, "token:") || ch.Tag == nil || *ch.Tag == "" {
		return other
	}
	if other == nil {
		other = NewLogOther()
	}
	other.SetPublic("provider", *ch.Tag)
	return other
}

// tokenStarterCreditLogged reports whether the account's starter credit was
// logged (in English, or in Chinese before the translation).
func tokenStarterCreditLogged(userID int) (bool, error) {
	var n int64
	err := LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ? AND (content LIKE ? OR content LIKE ?)",
		userID, LogTypeSystem, TokenStarterCreditLog+"%", tokenStarterCreditLogZh+"%").Count(&n).Error
	return n > 0, err
}

// tokenUnbilledChannelTest reports a consume log written by a channel test
// (controller/channel-test.go: token "模型测试", no API key). Tests are not
// billed, so they stay out of the usage dashboards (quota_data); otherwise the
// root account's "Last 24h usage" counted every health check while its total
// consumed did not. The log itself is kept for administrators.
func tokenUnbilledChannelTest(params RecordConsumeLogParams) bool {
	return params.TokenId == 0 && params.TokenName == "模型测试"
}
