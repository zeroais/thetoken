package common

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TOKEN: without a shared store (model/token_verification_code.go, tested
// there on every database), codes stay in this instance's memory as upstream
// keeps them: the newest code counts, codes are per purpose, and a consumed
// code works once.
func TestVerificationCodesWithoutSharedStore(t *testing.T) {
	saved := TokenVerificationCodes
	TokenVerificationCodes = nil
	t.Cleanup(func() { TokenVerificationCodes = saved })

	const email = "contact+beta1@example.com"
	RegisterVerificationCodeWithKey(email, "aaa111", EmailVerificationPurpose)
	RegisterVerificationCodeWithKey(email, "bbb222", EmailVerificationPurpose)
	assert.False(t, VerifyCodeWithKey(email, "aaa111", EmailVerificationPurpose), "an older code is replaced")
	assert.False(t, VerifyCodeWithKey(email, "bbb222", PasswordResetPurpose), "codes are per purpose")
	assert.True(t, VerifyCodeWithKey(email, "bbb222", EmailVerificationPurpose))
	assert.True(t, ConsumeCodeWithKey(email, "bbb222", EmailVerificationPurpose))
	assert.False(t, ConsumeCodeWithKey(email, "bbb222", EmailVerificationPurpose), "a code works once")
	assert.False(t, VerifyCodeWithKey(email, "bbb222", EmailVerificationPurpose))
}
