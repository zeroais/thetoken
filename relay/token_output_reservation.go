package relay

import (
	"fmt"
	"math"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	hosttypes "github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

// TOKEN: prepaid credit must cover what a request can cost before it runs.
// Upstream reserves only the estimated input and charges the output at
// settlement whatever the balance, so many concurrent requests from a small
// balance could each generate a long (costly) reply and leave unrecoverable
// debt. With token_billing.output_reservation on, a request also reserves
// its worst-case output (its own maximum, or the model's when it sets none),
// and when the balance or the key's spending limit cannot pay that, the
// request's maximum output is lowered to what they can pay (as OpenRouter
// does). A request that cannot afford even a minimal reply is refused.
// Fixed-price and tiered-expression models keep upstream's reservation.
//
// The maximum is written into the parsed request, which every text handler
// deep-copies before converting it for the provider. Body pass-through (a
// global or channel setting TOKEN does not use) forwards the raw body and
// would bypass the lowered maximum.
func tokenReserveOutput(c *gin.Context, info *relaycommon.RelayInfo, priceData *hosttypes.PriceData, requestedMax int) *types.NewAPIError {
	if !operation_setting.GetTokenBillingSetting().OutputReservation || priceData.UsePrice || info.TieredBillingSnapshot != nil {
		return nil
	}
	perToken := priceData.ModelRatio * priceData.CompletionRatio * priceData.GroupRatioInfo.GroupRatio
	if perToken <= 0 || math.IsNaN(perToken) || math.IsInf(perToken, 0) {
		return nil
	}
	switch info.Request.(type) {
	case *dto.GeneralOpenAIRequest, *dto.OpenAIResponsesRequest, *dto.ClaudeRequest, *dto.GeminiChatRequest:
	default:
		return nil // embeddings, images, audio: no generated text to bound
	}

	// The balance the reservation checks (the Redis copy when Redis is on;
	// with batch updates the database copy lags behind).
	available, err := model.GetUserQuota(info.UserId, false)
	if err != nil {
		return types.NewError(err, types.ErrorCodeQueryDataError, types.ErrOptionWithSkipRetry())
	}
	if !info.TokenUnlimited {
		available = min(available, c.GetInt("token_quota"))
	}
	affordable := 0
	if spare := available - priceData.QuotaToPreConsume; spare > 0 {
		affordable = int(min(math.Floor(float64(spare)/perToken), math.MaxInt32))
	}

	output := requestedMax
	if output <= 0 {
		output = operation_setting.TokenOutputTokens(info.OriginModelName)
	}
	minimal := operation_setting.TokenMinOutputTokens()
	if requestedMax > 0 {
		minimal = min(minimal, requestedMax)
	}
	if affordable < minimal {
		need := priceData.QuotaToPreConsume + common.QuotaFromFloat(float64(minimal)*perToken)
		return types.NewErrorWithStatusCode(
			fmt.Errorf("insufficient balance: %s needs at least $%.4f for a reply of %d tokens; this key can spend $%.4f. Top up your credit or raise the key's spending limit",
				info.OriginModelName, float64(need)/common.QuotaPerUnit, minimal, float64(max(available, 0))/common.QuotaPerUnit),
			types.ErrorCodeInsufficientUserQuota, http.StatusForbidden,
			types.ErrOptionWithSkipRetry(), types.ErrOptionWithNoRecordErrorLog())
	}
	if affordable < output {
		output = affordable
		tokenSetMaxOutput(info.Request, output)
	}

	reserve, clamp := common.QuotaFromFloatChecked(float64(output) * perToken)
	if clamp != nil || reserve > math.MaxInt32-priceData.QuotaToPreConsume {
		return types.NewErrorWithStatusCode(fmt.Errorf("output reservation out of range for %d tokens", output),
			types.ErrorCodeModelPriceError, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
	}
	priceData.QuotaToPreConsume += reserve
	return nil
}

// tokenSetMaxOutput sets a request's maximum output tokens in the field its
// API format uses; for OpenAI chat, every maximum the client set (or
// max_tokens when it set none).
func tokenSetMaxOutput(request dto.Request, tokens int) {
	n := uint(tokens)
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		if r.MaxCompletionTokens != nil {
			r.MaxCompletionTokens = &n
		}
		if r.MaxTokens != nil || r.MaxCompletionTokens == nil {
			r.MaxTokens = &n
		}
	case *dto.OpenAIResponsesRequest:
		r.MaxOutputTokens = &n
	case *dto.ClaudeRequest:
		r.MaxTokens = &n
	case *dto.GeminiChatRequest:
		r.GenerationConfig.MaxOutputTokens = &n
	}
}
