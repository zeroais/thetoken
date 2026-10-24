package controller

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// TOKEN: a request with tools for a model whose providers cannot call tools
// (token_routing.models_without_tools, set by the operator) is
// refused before routing with a clear 400, in the client's API format,
// instead of the provider's own error after a charge was reserved.
func tokenRejectUnsupportedTools(c *gin.Context, request dto.Request) *types.NewAPIError {
	model := c.GetString("original_model")
	if operation_setting.TokenModelSupportsTools(model) {
		return nil
	}
	hasTools := false
	switch r := request.(type) {
	case *dto.GeneralOpenAIRequest:
		hasTools = len(r.Tools) > 0 || tokenJSONHasItems(r.Functions)
	case *dto.ClaudeRequest:
		if r.Tools != nil {
			v := reflect.ValueOf(r.Tools)
			hasTools = v.Kind() != reflect.Slice || v.Len() > 0
		}
	case *dto.OpenAIResponsesRequest:
		hasTools = tokenJSONHasItems(r.Tools)
	}
	if !hasTools {
		return nil
	}
	return types.NewErrorWithStatusCode(
		fmt.Errorf("%s does not support tool calling (function calling). Send the request without tools, or choose a model that supports them: https://thetoken.dev/models/", model),
		types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
}

// tokenJSONHasItems reports a JSON value other than null or an empty list.
func tokenJSONHasItems(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) && !bytes.Equal(bytes.Join(bytes.Fields(trimmed), nil), []byte("[]"))
}
