package controller

import (
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
)

// tokenSetupComplete reports whether first-run setup is done. New API keeps
// that in a per-process flag, so with several instances only the one that
// served the setup request knows; the persisted setup record decides for
// the others (TOKEN fork: Cloud Run runs at least two instances).
func tokenSetupComplete() bool {
	if constant.Setup {
		return true
	}
	if model.GetSetup() == nil {
		return false
	}
	constant.Setup = true
	return true
}
