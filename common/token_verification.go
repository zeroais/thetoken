package common

import "time"

// TOKEN: email verification and password reset codes are kept in the main
// database (model/token_verification_code.go sets TokenVerificationCodes), so
// a code sent by one instance works on any other and survives an instance
// stopping (Cloud Run scales to zero; upstream keeps codes in each
// instance's memory). Only HMACs (CRYPTO_SECRET) of the mailbox and of the
// code are stored. A code expires after VerificationValidMinutes, a newer
// code replaces it, it works once (ConsumeCodeWithKey) and it is deleted
// after TokenVerificationMaxAttempts wrong guesses. Without a store, or if
// the database fails, the instance's memory is used as upstream does.

// TokenVerificationMaxAttempts is how many wrong codes a mailbox may try
// against one issued code before the code is deleted.
const TokenVerificationMaxAttempts = 5

// TokenVerificationCodeStore keeps codes where every instance sees them.
type TokenVerificationCodeStore interface {
	// Save stores the code of id, replacing any earlier one.
	Save(id, codeHash string, expiresAt, now int64) error
	// Check reports whether codeHash is id's unexpired code; consume deletes
	// it in the same statement, so only one caller can use it. A wrong code
	// counts as an attempt.
	Check(id, codeHash string, now int64, consume bool) (bool, error)
	Delete(id string) error
}

var TokenVerificationCodes TokenVerificationCodeStore

func tokenVerificationID(key, purpose string) string {
	return GenerateHMAC("token:verify:" + purpose + ":" + key)
}

// tokenStoreVerificationCode reports whether the shared store kept the code.
func tokenStoreVerificationCode(key, code, purpose string) bool {
	if TokenVerificationCodes == nil {
		return false
	}
	now := GetTimestamp()
	err := TokenVerificationCodes.Save(tokenVerificationID(key, purpose), GenerateHMAC(code), now+int64(VerificationValidMinutes)*60, now)
	if err != nil {
		SysError("TOKEN: verification code not stored in the database, kept on this instance: " + err.Error())
		return false
	}
	return true
}

// tokenCheckVerificationCode reports whether the shared store accepts the
// code (and, with consume, used it up).
func tokenCheckVerificationCode(key, code, purpose string, consume bool) bool {
	if TokenVerificationCodes == nil {
		return false
	}
	ok, err := TokenVerificationCodes.Check(tokenVerificationID(key, purpose), GenerateHMAC(code), GetTimestamp(), consume)
	if err != nil {
		SysError("TOKEN: verification code check failed: " + err.Error())
		return false
	}
	return ok
}

func tokenDeleteVerificationCode(key, purpose string) {
	if TokenVerificationCodes != nil {
		_ = TokenVerificationCodes.Delete(tokenVerificationID(key, purpose))
	}
}

// ConsumeCodeWithKey checks a code and uses it up: of several requests with
// the same code, only one succeeds.
func ConsumeCodeWithKey(key string, code string, purpose string) bool {
	if tokenCheckVerificationCode(key, code, purpose, true) {
		return true
	}
	verificationMutex.Lock()
	defer verificationMutex.Unlock()
	value, okay := verificationMap[purpose+key]
	if !okay || int(time.Since(value.time).Seconds()) >= VerificationValidMinutes*60 || code != value.code {
		return false
	}
	delete(verificationMap, purpose+key)
	return true
}
