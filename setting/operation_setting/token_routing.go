package operation_setting

import (
	"slices"

	"github.com/QuantumNous/new-api/setting/config"
)

// TOKEN routing facts the relay enforces. Option keys token_routing.*,
// set by the operator through the admin API (TOKEN's tooling writes them from
// its model catalog); the code default enforces nothing (upstream behavior).
type TokenRoutingSetting struct {
	// Model names and aliases whose providers cannot call tools
	// (capabilities.tools: false); requests with tools get a clear 400.
	ModelsWithoutTools []string `json:"models_without_tools"`
}

var tokenRoutingSetting = TokenRoutingSetting{}

func init() {
	config.GlobalConfig.Register("token_routing", &tokenRoutingSetting)
}

// TokenModelSupportsTools reports whether requests with tools may go to model.
func TokenModelSupportsTools(model string) bool {
	return !slices.Contains(tokenRoutingSetting.ModelsWithoutTools, model)
}
