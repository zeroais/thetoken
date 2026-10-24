package model

import (
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenLogTextIsEnglishWithDollarAmounts(t *testing.T) {
	for zh, en := range map[string]string{
		"新用户注册赠送 ＄0.500000 额度":                                 "Starter credit: $0.50",
		"使用在线充值成功，充值金额: ＄10.000000 额度，支付金额：10.550000":          "Online top-up: $10.00 added, paid 10.550000",
		"通过兑换码充值 ＄2.000000 额度，兑换码ID 7":                         "Redemption code 7 added $2.00",
		"Moyasar top-up credited: ＄5.000000 (order moyasar_1)": "Moyasar top-up credited: $5.00 (order moyasar_1)",
		"Moyasar refund on order moyasar_1 (user 2): 18.75 SAR refunded so far, ＄5.000000 credit removed now, balance ＄299.995782, 0 API keys suspended.": "Moyasar refund on order moyasar_1 (user 2): 18.75 SAR refunded so far, $5.00 credit removed now, balance $299.995782, 0 API keys suspended.",
		"charged ＄0.002450":            "charged $0.00245",
		"upstream error: rate limited": "upstream error: rate limited",
		"":                             "",
	} {
		assert.Equal(t, en, TokenLogText(zh), zh)
	}
}

func TestStarterCreditIsLoggedInEnglishAndFoundInBothLanguages(t *testing.T) {
	useStarterCredit(t, true, func(t *testing.T) {
		credited := insertStarterUser(t, "credited", "credited@example.com")
		var log Log
		require.NoError(t, LOG_DB.Where("user_id = ? AND type = ?", credited.Id, LogTypeSystem).First(&log).Error)
		assert.Equal(t, "Starter credit: $0.50", log.Content)

		// A row written before the translation still counts.
		old := Log{UserId: 99, Type: LogTypeSystem, Content: "新用户注册赠送 ＄0.500000 额度", CreatedAt: 1}
		require.NoError(t, LOG_DB.Create(&old).Error)
		for _, id := range []int{credited.Id, 99} {
			logged, err := tokenStarterCreditLogged(id)
			require.NoError(t, err)
			assert.True(t, logged, id)
		}
	})
}

func TestConsumeLogNamesTheProviderOfTokenChannels(t *testing.T) {
	truncateTables(t)
	gin.SetMode(gin.TestMode)
	memoryCache, logConsume := common.MemoryCacheEnabled, common.LogConsumeEnabled
	common.MemoryCacheEnabled, common.LogConsumeEnabled = false, true
	t.Cleanup(func() { common.MemoryCacheEnabled, common.LogConsumeEnabled = memoryCache, logConsume })

	tag := "Provider A"
	catalog := Channel{Name: "token:provider-a:zai-org/glm-5.3", Tag: &tag, Key: "k1", Status: common.ChannelStatusEnabled}
	other := Channel{Name: "my own channel", Tag: &tag, Key: "k2", Status: common.ChannelStatusEnabled}
	require.NoError(t, DB.Create(&catalog).Error)
	require.NoError(t, DB.Create(&other).Error)

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	for _, ch := range []Channel{catalog, other} {
		RecordConsumeLog(c, 1, RecordConsumeLogParams{ChannelId: ch.Id, ModelName: "zai-org/glm-5.3", Quota: 10, Other: NewLogOther()})
	}

	var logs []Log
	require.NoError(t, LOG_DB.Where("type = ?", LogTypeConsume).Order("id").Find(&logs).Error)
	require.Len(t, logs, 2)
	var got map[string]any
	require.NoError(t, common.UnmarshalJsonStr(logs[0].Other, &got))
	assert.Equal(t, "Provider A", got["provider"])
	got = nil
	require.NoError(t, common.UnmarshalJsonStr(logs[1].Other, &got))
	assert.NotContains(t, got, "provider", "only TOKEN's catalog channels name a provider")
}

func TestChannelTestsStayOutOfTheUsageDashboards(t *testing.T) {
	truncateTables(t)
	gin.SetMode(gin.TestMode)
	export, logConsume := common.DataExportEnabled, common.LogConsumeEnabled
	common.DataExportEnabled, common.LogConsumeEnabled = true, true
	CacheQuotaDataLock.Lock()
	clear(CacheQuotaData)
	CacheQuotaDataLock.Unlock()
	t.Cleanup(func() {
		common.DataExportEnabled, common.LogConsumeEnabled = export, logConsume
		CacheQuotaDataLock.Lock()
		clear(CacheQuotaData)
		CacheQuotaDataLock.Unlock()
	})

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", nil)
	RecordConsumeLog(c, 1, RecordConsumeLogParams{ModelName: "zai-org/glm-5.3", TokenName: "模型测试", Quota: 500, Other: NewLogOther()})
	RecordConsumeLog(c, 1, RecordConsumeLogParams{ModelName: "zai-org/glm-5.3", TokenName: "my key", TokenId: 7, Quota: 300, Other: NewLogOther()})

	CacheQuotaDataLock.Lock()
	defer CacheQuotaDataLock.Unlock()
	require.Len(t, CacheQuotaData, 1, "only the billed request")
	for _, q := range CacheQuotaData {
		assert.Equal(t, 300, q.Quota)
	}
	var logs int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("type = ?", LogTypeConsume).Count(&logs).Error)
	assert.EqualValues(t, 2, logs, "the channel test is still logged for administrators")
}
