package model

// TOKEN: API keys are stored hashed ("show once, store hash"). The plaintext
// key is returned once, when the key is created or rotated, and never stored.
//
// The stored form is "h1:" + hex(sha256(key)) in the existing key column, so
// every internal lookup (Redis cache, quota reservation, billing) keeps using
// token.Key unchanged. Plaintext keys arriving from clients are hashed at the
// authentication boundary (HashTokenKey). A short hint ("abcd…wxyz") is
// kept for display.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const tokenKeyHashPrefix = "h1:"

// ErrTokenKeyShownOnce is returned by the retired "reveal key" endpoints.
var ErrTokenKeyShownOnce = errors.New("API keys are shown only once, when they are created. Rotate the key to get a new one.")

// HashTokenKey turns a client-supplied key (with or without "sk-") into its
// stored form. It always hashes, so a stored hash sent as a key never matches.
func HashTokenKey(raw string) string {
	raw = strings.TrimPrefix(raw, "sk-")
	if raw == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(raw))
	return tokenKeyHashPrefix + hex.EncodeToString(sum[:])
}

// IsHashedTokenKey reports whether a stored key is already hashed.
func IsHashedTokenKey(stored string) bool {
	return strings.HasPrefix(stored, tokenKeyHashPrefix)
}

// TokenKeyHint is the display form of a plaintext key, "abcd…wxyz" (without
// "sk-", like upstream's masked key, which clients prefix). Keys in TOKEN's
// format keep their prefix, "thetoken_abcd…wxyz", so the console shows them
// as they are.
func TokenKeyHint(raw string) string {
	if body, ok := strings.CutPrefix(raw, common.TokenAPIKeyPrefix); ok && len(body) > 8 {
		return common.TokenAPIKeyPrefix + body[:4] + "…" + body[len(body)-4:]
	}
	raw = strings.TrimPrefix(raw, "sk-")
	if len(raw) <= 8 {
		return "…"
	}
	return raw[:4] + "…" + raw[len(raw)-4:]
}

// BeforeCreate stores every new key hashed, wherever the token is created.
func (token *Token) BeforeCreate(tx *gorm.DB) error {
	if token.Key != "" && !IsHashedTokenKey(token.Key) {
		token.KeyHint = TokenKeyHint(token.Key)
		token.Key = HashTokenKey(token.Key)
	}
	return nil
}

// RotateTokenKey gives a token a new key and returns the plaintext once.
// The old key stops working immediately (cache invalidated).
func RotateTokenKey(id, userId int) (string, error) {
	token, err := GetTokenByIds(id, userId)
	if err != nil {
		return "", err
	}
	raw, err := common.GenerateTokenAPIKey()
	if err != nil {
		return "", err
	}
	oldKey := token.Key
	res := DB.Model(&Token{}).Where("id = ? AND user_id = ?", id, userId).
		Updates(map[string]any{"key": HashTokenKey(raw), "key_hint": TokenKeyHint(raw)})
	if res.Error != nil {
		return "", res.Error
	}
	if res.RowsAffected == 0 {
		return "", gorm.ErrRecordNotFound
	}
	if err := invalidateTokenCacheForMutation(oldKey); err != nil {
		common.SysError("failed to invalidate rotated token cache: " + err.Error())
	}
	return raw, nil
}

// migrateTokenKeyHashes hashes keys stored in plaintext by earlier versions.
// Idempotent; runs on the master node at startup, before Redis is connected.
// Redis entries cached under plaintext keys become unreachable (every lookup
// now uses the hash) and expire on their own TTL.
func migrateTokenKeyHashes() error {
	for {
		var tokens []Token
		if err := DB.Unscoped().Select("id", "key").
			Where(commonKeyCol+" <> ? AND "+commonKeyCol+" NOT LIKE ?", "", tokenKeyHashPrefix+"%").
			Limit(500).Find(&tokens).Error; err != nil {
			return err
		}
		if len(tokens) == 0 {
			return nil
		}
		for _, t := range tokens {
			if err := DB.Unscoped().Model(&Token{}).Where("id = ?", t.Id).
				Updates(map[string]any{"key": HashTokenKey(t.Key), "key_hint": TokenKeyHint(t.Key)}).Error; err != nil {
				return err
			}
		}
		common.SysLog("TOKEN: hashed stored API keys: " + common.Interface2String(len(tokens)))
	}
}
