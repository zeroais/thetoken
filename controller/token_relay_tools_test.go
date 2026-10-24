package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/setting/config"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TOKEN: tool requests to a model listed in token_routing.models_without_tools
// get a clear 400 on the OpenAI, Anthropic and Responses APIs; the same
// request without tools, and tools for other models, pass.
func TestToolRequestsToModelsWithoutToolSupportAreRefused(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{
		"token_routing.models_without_tools": `["llama-4-maverick","meta-llama/llama-4-maverick"]`,
	}))
	t.Cleanup(func() {
		require.NoError(t, config.GlobalConfig.LoadFromDB(map[string]string{"token_routing.models_without_tools": `[]`}))
	})

	weather := []dto.ToolCallRequest{{Type: "function", Function: dto.FunctionRequest{Name: "get_weather"}}}
	cases := []struct {
		name    string
		model   string
		request dto.Request
		refused bool
	}{
		{"chat with tools", "meta-llama/llama-4-maverick", &dto.GeneralOpenAIRequest{Tools: weather}, true},
		{"chat with legacy functions, alias", "llama-4-maverick", &dto.GeneralOpenAIRequest{Functions: []byte(`[{"name":"f"}]`)}, true},
		{"messages with tools", "meta-llama/llama-4-maverick", &dto.ClaudeRequest{Tools: []any{map[string]any{"name": "get_weather"}}}, true},
		{"responses with tools", "meta-llama/llama-4-maverick", &dto.OpenAIResponsesRequest{Tools: []byte(`[{"type":"function","name":"f"}]`)}, true},
		{"chat without tools", "meta-llama/llama-4-maverick", &dto.GeneralOpenAIRequest{Functions: []byte(`[ ]`)}, false},
		{"messages without tools", "meta-llama/llama-4-maverick", &dto.ClaudeRequest{Tools: []any{}}, false},
		{"tools for another model", "moonshotai/kimi-k3", &dto.GeneralOpenAIRequest{Tools: weather}, false},
	}
	for _, tc := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Set("original_model", tc.model)
		err := tokenRejectUnsupportedTools(c, tc.request)
		if !tc.refused {
			assert.Nil(t, err, tc.name)
			continue
		}
		require.NotNil(t, err, tc.name)
		assert.Equal(t, http.StatusBadRequest, err.StatusCode, tc.name)
		assert.Contains(t, err.Error(), tc.model+" does not support tool calling", tc.name)
		assert.Contains(t, err.ToClaudeError().Message, "does not support tool calling", tc.name)
	}
}
