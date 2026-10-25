package controller

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TOKEN: a successful sign-up signs the new account in (the sign-in page
// would need a second, unspent Turnstile token), and its email code works
// once.
func TestSignUpSignsTheNewAccountInAndSpendsItsCode(t *testing.T) {
	setupSecurityEnrollmentTest(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Token{}, &model.TokenStarterGrant{}))
	require.NoError(t, model.LOG_DB.AutoMigrate(&model.Log{}))
	saved := []bool{common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled}
	common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled = true, true, true
	t.Cleanup(func() {
		common.RegisterEnabled, common.PasswordRegisterEnabled, common.EmailVerificationEnabled = saved[0], saved[1], saved[2]
	})

	const email = "beta1@example.com"
	common.RegisterVerificationCodeWithKey(email, "c0de42", common.EmailVerificationPurpose)
	body, err := common.Marshal(map[string]string{"username": "betatest1", "password": "Correct-horse-9", "email": email, "verification_code": "c0de42"})
	require.NoError(t, err)

	response := securityEnrollmentRequest("POST", "/api/user/register", string(body), "", service.AuthIdentity{}, Register)
	var result struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken string `json:"access_token"`
			User        struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	require.NoError(t, common.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, result.Success, response.Body.String())
	assert.NotEmpty(t, result.Data.AccessToken, "signed in by the sign-up response")
	assert.Equal(t, "betatest1", result.Data.User.Username)
	identity, err := service.ParseAccessToken(result.Data.AccessToken)
	require.NoError(t, err)
	var user model.User
	require.NoError(t, model.DB.Where("username = ?", "betatest1").First(&user).Error)
	assert.Equal(t, user.Id, identity.UserID)

	assert.False(t, common.VerifyCodeWithKey(email, "c0de42", common.EmailVerificationPurpose), "the code works once")
}
