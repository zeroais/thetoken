package middleware

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
)

// TOKEN fork: mandatory second factor for admin and root accounts
// (OWASP ASVS 5.0 6.3.3; ASVS 4.0.3 4.3.1; MFA Cheat Sheet "Require MFA for
// administrative or other high privileged users").
//
// Checked on every authenticated dashboard request against live data
// (ASVS 8.3.1/8.3.2), so it covers browser sessions and personal access
// tokens alike, takes effect immediately for existing sessions, and needs
// no session flag. An unenrolled admin may only reach the routes below:
// enrolment, its proofs, the own session list and the self endpoint.

// MFAEnrollmentRequiredCode is the error code the console redirects on.
const MFAEnrollmentRequiredCode = "MFA_ENROLLMENT_REQUIRED"

var adminMFAEnrollmentRoutes = map[string]bool{
	"GET /api/user/self":                     true,
	"GET /api/user/2fa/status":               true,
	"POST /api/user/2fa/setup":               true,
	"POST /api/user/2fa/enable":              true,
	"GET /api/user/passkey":                  true,
	"POST /api/user/passkey/register/begin":  true,
	"POST /api/user/passkey/register/finish": true,
	"GET /api/user/sessions":                 true,
	"DELETE /api/user/sessions/:sid":         true,
	"POST /api/user/sessions/revoke-others":  true,
	"GET /api/verify/methods":                true,
	"POST /api/verify":                       true,
}

// enforceAdminSecondFactor aborts the request when an admin or root user
// without TOTP or a passkey calls anything but an enrolment route.
func enforceAdminSecondFactor(c *gin.Context, userID, role int) bool {
	if role < common.RoleAdminUser || !operation_setting.GetTokenSecuritySetting().RequireAdminMFA {
		return true
	}
	if adminMFAEnrollmentRoutes[c.Request.Method+" "+c.FullPath()] {
		return true
	}
	state, err := model.GetUserVerificationState(userID)
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"success": false, "code": "AUTH_INTERNAL_ERROR", "message": "could not verify account security state"})
		return false
	}
	if state.HasTwoFA || state.HasPasskey {
		return true
	}
	// ASVS 16.3.2: log authorization failures (no credentials logged).
	common.SysLog(fmt.Sprintf("admin MFA gate: user %d (role %d) denied %s %s: no second factor enrolled", userID, role, c.Request.Method, c.FullPath()))
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
		"success": false,
		"code":    MFAEnrollmentRequiredCode,
		"message": "Admin accounts must enable two-factor authentication or a passkey. Set one up in Security & Access.",
	})
	return false
}
