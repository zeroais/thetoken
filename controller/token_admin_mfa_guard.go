package controller

import (
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// TOKEN fork: with mandatory admin MFA on, an admin or root user may not
// remove their own last second factor (MFA Cheat Sheet "Resetting MFA";
// replace a factor by adding the new one before removing the old one).
// An admin resetting another user's factor stays allowed: that is the
// recovery path, and the target lands in enrolment-only mode.

const mfaLastFactorCode = "MFA_LAST_FACTOR"

// refuseRemovingLastAdminFactor writes 409 and returns true when userID is
// an admin whose only remaining factor is the one being removed
// ("totp" or "passkey").
func refuseRemovingLastAdminFactor(c *gin.Context, userID int, removing string) bool {
	if !operation_setting.GetTokenSecuritySetting().RequireAdminMFA {
		return false
	}
	state, err := model.GetUserVerificationState(userID)
	if err != nil || state.Role < common.RoleAdminUser {
		return false
	}
	remaining := state.HasPasskey
	if removing == "passkey" {
		remaining = state.HasTwoFA
	}
	if remaining {
		return false
	}
	c.AbortWithStatusJSON(http.StatusConflict, gin.H{
		"success": false,
		"code":    mfaLastFactorCode,
		"message": "Admin accounts must keep a second factor. Add a passkey or authenticator app before removing this one.",
	})
	return true
}
