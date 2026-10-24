package operation_setting

import "github.com/QuantumNous/new-api/setting/config"

// TOKEN fork security settings. Option keys token_security.*; TOKEN enables
// them in its console settings. Code defaults keep upstream behavior.
type TokenSecuritySetting struct {
	// Admin and root accounts must have TOTP or a passkey. Until they enrol,
	// their sessions and access tokens can only reach enrolment routes.
	RequireAdminMFA bool `json:"require_admin_mfa"`
}

var tokenSecuritySetting = TokenSecuritySetting{}

func init() {
	config.GlobalConfig.Register("token_security", &tokenSecuritySetting)
}

// GetTokenSecuritySetting exposes the setting (middleware, controllers, tests).
func GetTokenSecuritySetting() *TokenSecuritySetting {
	return &tokenSecuritySetting
}
