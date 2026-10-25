package common

import (
	"crypto/hmac"
	"time"
)

// TOKEN: email verification and password reset codes are kept in Redis when
// it is on, so any instance can check a code another instance sent (upstream
// keeps them in each instance's memory; with several instances behind the
// load balancer a sign-up then fails whenever it reaches a different
// instance than the one that sent the code). Redis holds an HMAC of the code
// (CRYPTO_SECRET), never the code, and expires it after
// VerificationValidMinutes. If Redis fails, the instance's memory is used as
// before.

func tokenVerificationKey(key, purpose string) string {
	return "token:verify:" + purpose + ":" + GenerateHMAC(key)
}

// tokenStoreVerificationCode reports whether Redis stored the code.
func tokenStoreVerificationCode(key, code, purpose string) bool {
	if !RedisEnabled || RDB == nil {
		return false
	}
	err := RedisSet(tokenVerificationKey(key, purpose), GenerateHMAC(code), time.Duration(VerificationValidMinutes)*time.Minute)
	if err != nil {
		SysError("TOKEN: verification code not stored in Redis, kept on this instance: " + err.Error())
		return false
	}
	return true
}

// tokenVerificationCodeInRedis reports whether Redis holds this code.
func tokenVerificationCodeInRedis(key, code, purpose string) bool {
	if !RedisEnabled || RDB == nil {
		return false
	}
	stored, err := RedisGet(tokenVerificationKey(key, purpose))
	return err == nil && hmac.Equal([]byte(stored), []byte(GenerateHMAC(code)))
}

func tokenDeleteVerificationCode(key, purpose string) {
	if RedisEnabled && RDB != nil {
		_ = RedisDel(tokenVerificationKey(key, purpose))
	}
}
