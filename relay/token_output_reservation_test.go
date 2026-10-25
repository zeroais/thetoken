package relay

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TOKEN: a request reserves its input estimate plus its worst-case output,
// and its maximum output is lowered to what the balance and the key's limit
// can pay; below a minimal reply it is refused.
func TestOutputReservationCoversWorstCaseAndCapsTheMaximum(t *testing.T) {
	database := setupRelayChannelDB(t)
	require.NoError(t, database.AutoMigrate(&model.User{}))
	redisEnabled := common.RedisEnabled
	common.RedisEnabled = false // balances from the database
	t.Cleanup(func() { common.RedisEnabled = redisEnabled })
	setting := operation_setting.GetTokenBillingSetting()
	saved := *setting
	t.Cleanup(func() { *setting = saved })
	setting.OutputReservation = true
	setting.ModelOutputTokens = map[string]int{"m": 1000}
	setting.MinOutputTokens = 100

	// 1 quota per input token, 2 per output token; the input estimate is 50.
	price := func() hosttypes.PriceData {
		return hosttypes.PriceData{ModelRatio: 1, CompletionRatio: 2, GroupRatioInfo: hosttypes.GroupRatioInfo{GroupRatio: 1}, QuotaToPreConsume: 50}
	}
	maxTokens := func(n uint) *uint { return &n }

	cases := []struct {
		name        string
		balance     int
		keyLimit    int // 0: no limit
		request     dto.Request
		wantReserve int
		wantMax     *uint // the request's maximum after reservation
		wantError   bool
	}{
		{"enough balance, no maximum: model's output reserved, request untouched", 1_000_000, 0, &dto.GeneralOpenAIRequest{}, 2050, nil, false},
		{"enough balance, client maximum reserved as asked", 1_000_000, 0, &dto.GeneralOpenAIRequest{MaxTokens: maxTokens(300)}, 650, maxTokens(300), false},
		{"small balance: maximum lowered to what it pays, exactly the balance reserved", 650, 0, &dto.GeneralOpenAIRequest{}, 650, maxTokens(300), false},
		{"key limit below the balance bounds the maximum", 1_000_000, 450, &dto.GeneralOpenAIRequest{MaxCompletionTokens: maxTokens(900)}, 450, maxTokens(200), false},
		{"Anthropic format", 650, 0, &dto.ClaudeRequest{MaxTokens: maxTokens(4096)}, 650, maxTokens(300), false},
		{"Responses format", 650, 0, &dto.OpenAIResponsesRequest{}, 650, maxTokens(300), false},
		{"cannot pay a minimal reply: refused", 200, 0, &dto.GeneralOpenAIRequest{}, 0, nil, true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			user := model.User{Username: "reserve-" + string(rune('a'+i)), AffCode: "aff-" + string(rune('a'+i)), Quota: tc.balance, Status: 1}
			require.NoError(t, database.Create(&user).Error)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
			c.Set("token_quota", tc.keyLimit)
			info := &relaycommon.RelayInfo{UserId: user.Id, OriginModelName: "m", TokenUnlimited: tc.keyLimit == 0, Request: tc.request}
			requested := 0
			if meta := tc.request.GetTokenCountMeta(); meta != nil {
				requested = meta.MaxTokens
			}
			priceData := price()

			apiErr := tokenReserveOutput(c, info, &priceData, requested)
			if tc.wantError {
				require.NotNil(t, apiErr)
				assert.Equal(t, http.StatusForbidden, apiErr.StatusCode)
				assert.Contains(t, apiErr.Error(), "insufficient balance")
				return
			}
			require.Nil(t, apiErr)
			assert.Equal(t, tc.wantReserve, priceData.QuotaToPreConsume)
			var got *uint
			switch r := tc.request.(type) {
			case *dto.GeneralOpenAIRequest:
				got = r.MaxTokens
				if r.MaxCompletionTokens != nil {
					got = r.MaxCompletionTokens
				}
			case *dto.ClaudeRequest:
				got = r.MaxTokens
			case *dto.OpenAIResponsesRequest:
				got = r.MaxOutputTokens
			}
			assert.Equal(t, lo.FromPtr(tc.wantMax), lo.FromPtr(got))
		})
	}

	setting.OutputReservation = false
	priceData := price()
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	require.Nil(t, tokenReserveOutput(c, &relaycommon.RelayInfo{Request: &dto.GeneralOpenAIRequest{}}, &priceData, 0))
	assert.Equal(t, 50, priceData.QuotaToPreConsume, "off: upstream's input-only reservation")
}
