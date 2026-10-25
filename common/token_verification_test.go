package common

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TOKEN: with Redis, a code sent by one instance is accepted by another (the
// sign-up failure of 2026-10-25), only the newest code counts, it expires,
// and a deleted code stops working. Without Redis, codes stay in memory.
func TestVerificationCodesAreSharedByInstances(t *testing.T) {
	mr := miniredis.RunT(t)
	savedRDB, savedEnabled := RDB, RedisEnabled
	RDB, RedisEnabled = redis.NewClient(&redis.Options{Addr: mr.Addr()}), true
	t.Cleanup(func() { RDB, RedisEnabled = savedRDB, savedEnabled })
	otherInstance := func() { // an instance that never saw the code
		verificationMutex.Lock()
		verificationMap = map[string]verificationValue{}
		verificationMutex.Unlock()
	}

	const email = "contact+beta1@example.com"
	RegisterVerificationCodeWithKey(email, "aaa111", EmailVerificationPurpose)
	RegisterVerificationCodeWithKey(email, "bbb222", EmailVerificationPurpose)
	otherInstance()
	assert.True(t, VerifyCodeWithKey(email, "bbb222", EmailVerificationPurpose), "another instance accepts the newest code")
	assert.False(t, VerifyCodeWithKey(email, "aaa111", EmailVerificationPurpose), "an older code is replaced")
	assert.False(t, VerifyCodeWithKey(email, "bbb222", PasswordResetPurpose), "codes are per purpose")
	assert.False(t, VerifyCodeWithKey("contact@example.com", "bbb222", EmailVerificationPurpose), "codes are per address")
	assert.NotContains(t, mr.Dump(), "bbb222", "Redis holds no plain code")
	assert.NotContains(t, mr.Dump(), "beta1", "nor the address")

	DeleteKey(email, EmailVerificationPurpose)
	assert.False(t, VerifyCodeWithKey(email, "bbb222", EmailVerificationPurpose), "a used code stops working")

	RegisterVerificationCodeWithKey(email, "ccc333", PasswordResetPurpose)
	mr.FastForward(time.Duration(VerificationValidMinutes)*time.Minute + time.Second)
	assert.False(t, VerifyCodeWithKey(email, "ccc333", PasswordResetPurpose), "codes expire")

	RedisEnabled = false
	RegisterVerificationCodeWithKey(email, "ddd444", EmailVerificationPurpose)
	require.True(t, VerifyCodeWithKey(email, "ddd444", EmailVerificationPurpose), "without Redis, this instance's memory")
}
