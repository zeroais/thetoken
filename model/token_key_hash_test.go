package model

import (
	"os"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestHashTokenKey(t *testing.T) {
	assert.Equal(t, "", HashTokenKey(""))
	assert.Equal(t, "", HashTokenKey("sk-"))
	h := HashTokenKey("abc123")
	assert.True(t, strings.HasPrefix(h, "h1:"))
	assert.Len(t, h, 3+64)
	assert.Equal(t, h, HashTokenKey("sk-abc123"), "sk- prefix is optional")
	assert.NotEqual(t, h, HashTokenKey(h), "a stored hash is never accepted as a key")
	assert.Equal(t, "abcd…wxyz", TokenKeyHint("sk-abcd0123456789wxyz"))
	assert.Equal(t, "…", TokenKeyHint("short"))
}

func TestTokenKeyStoredHashedAndLookedUpByHash(t *testing.T) {
	truncateTables(t)
	raw := "hashed-key-0123456789abcdefghijklmnopqrstuvwx"
	token := Token{UserId: 3, Name: "h", Key: raw, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, token.Insert())

	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	assert.Equal(t, HashTokenKey(raw), stored.Key)
	assert.Equal(t, "hash…uvwx", stored.KeyHint)
	assert.Equal(t, "hash…uvwx", stored.GetMaskedKey())
	assert.NotContains(t, stored.Key, raw)

	found, err := ValidateUserToken(HashTokenKey("sk-" + raw))
	require.NoError(t, err)
	assert.Equal(t, token.Id, found.Id)

	_, err = ValidateUserToken(HashTokenKey(stored.Key))
	assert.ErrorIs(t, err, ErrTokenInvalid, "the hash itself must not authenticate")
}

// TOKEN: new keys have the distinctive "thetoken_" format (prefix, random
// part, checksum); they authenticate as sent, and keep their prefix in the
// display hint. Older "sk-" keys keep working.
func TestTokenAPIKeyFormat(t *testing.T) {
	truncateTables(t)
	raw, err := common.GenerateTokenAPIKey()
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(raw, "thetoken_"))
	assert.Len(t, raw, len("thetoken_")+43+6)
	assert.True(t, common.IsTokenAPIKey(raw))
	other, err := common.GenerateTokenAPIKey()
	require.NoError(t, err)
	assert.NotEqual(t, raw, other)
	tampered := raw[:20] + map[bool]string{true: "A", false: "B"}[raw[20] != 'A'] + raw[21:]
	assert.False(t, common.IsTokenAPIKey(tampered), "the checksum catches a changed character")
	assert.False(t, common.IsTokenAPIKey("thetoken_short"))
	assert.False(t, common.IsTokenAPIKey("sk-"+raw[len("thetoken_"):]))

	token := Token{UserId: 3, Name: "new", Key: raw, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, token.Insert())
	var stored Token
	require.NoError(t, DB.First(&stored, token.Id).Error)
	assert.Equal(t, "thetoken_"+raw[9:13]+"…"+raw[len(raw)-4:], stored.KeyHint)
	found, err := ValidateUserToken(HashTokenKey(raw))
	require.NoError(t, err, "a new key authenticates as sent")
	assert.Equal(t, token.Id, found.Id)
	found, err = ValidateUserToken(HashTokenKey("sk-" + raw))
	require.NoError(t, err, "clients that add sk- still work")
	assert.Equal(t, token.Id, found.Id)

	legacy := "legacyKey0123456789abcdefghijklmnopqrstuvwxyzAB"
	old := Token{UserId: 3, Name: "old", Key: legacy, Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, old.Insert())
	found, err = ValidateUserToken(HashTokenKey("sk-" + legacy))
	require.NoError(t, err, "existing sk- keys keep working")
	assert.Equal(t, old.Id, found.Id)
}

func TestMigrateTokenKeyHashes(t *testing.T) {
	truncateTables(t)
	legacy := "legacy-plaintext-key-0123456789abcdefghijklmn"
	require.NoError(t, DB.Exec("INSERT INTO tokens (user_id, name, "+commonKeyCol+", status, expired_time) VALUES (?, ?, ?, ?, ?)",
		5, "legacy", legacy, common.TokenStatusEnabled, -1).Error)
	require.NoError(t, DB.Exec("INSERT INTO tokens (user_id, name, "+commonKeyCol+", status, expired_time) VALUES (?, ?, ?, ?, ?)",
		5, "empty", "", common.TokenStatusEnabled, -1).Error)

	for range 2 { // idempotent
		require.NoError(t, migrateTokenKeyHashes())
	}

	var migrated Token
	require.NoError(t, DB.Where("name = ?", "legacy").First(&migrated).Error)
	assert.Equal(t, HashTokenKey(legacy), migrated.Key)
	assert.Equal(t, TokenKeyHint(legacy), migrated.KeyHint)
	found, err := GetTokenByKey(HashTokenKey(legacy), true)
	require.NoError(t, err)
	assert.Equal(t, migrated.Id, found.Id)
}

func TestRotateTokenKey(t *testing.T) {
	truncateTables(t)
	token := Token{UserId: 9, Name: "r", Key: "rotate-old-key-0123456789abcdefghijklmnopqrs", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true}
	require.NoError(t, token.Insert())

	_, err := RotateTokenKey(token.Id, 10)
	assert.Error(t, err, "only the owner can rotate")

	raw, err := RotateTokenKey(token.Id, 9)
	require.NoError(t, err)
	assert.True(t, common.IsTokenAPIKey(raw), "a rotated key is issued in TOKEN's format")

	_, err = GetTokenByKey(HashTokenKey("rotate-old-key-0123456789abcdefghijklmnopqrs"), true)
	assert.Error(t, err, "the old key stops working")
	found, err := GetTokenByKey(HashTokenKey(raw), true)
	require.NoError(t, err)
	assert.Equal(t, token.Id, found.Id)
	assert.Equal(t, TokenKeyHint(raw), found.KeyHint)
}

// TOKEN: the key-hash migration on MySQL and PostgreSQL (AGENTS.md database
// matrix). Runs when TEST_MYSQL_DSN / TEST_POSTGRES_DSN are set.
func TestMigrateTokenKeyHashesDatabaseMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, env, keyCol string
		open              func(string) gorm.Dialector
	}{
		{"mysql", "TEST_MYSQL_DSN", "`key`", func(dsn string) gorm.Dialector { return mysql.Open(dsn) }},
		{"postgres", "TEST_POSTGRES_DSN", `"key"`, func(dsn string) gorm.Dialector {
			return postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dsn := strings.TrimSpace(os.Getenv(tc.env))
			if dsn == "" {
				t.Skip(tc.env + " is not configured")
			}
			db, err := gorm.Open(tc.open(dsn), &gorm.Config{})
			require.NoError(t, err)
			prevDB, prevCol := DB, commonKeyCol
			DB, commonKeyCol = db, tc.keyCol
			t.Cleanup(func() {
				_ = db.Migrator().DropTable(&Token{})
				DB, commonKeyCol = prevDB, prevCol
				if sqlDB, err := db.DB(); err == nil {
					_ = sqlDB.Close()
				}
			})
			_ = db.Migrator().DropTable(&Token{})
			require.NoError(t, db.AutoMigrate(&Token{}))

			legacy := "matrix-legacy-key-0123456789abcdefghijklmnopq"
			require.NoError(t, db.Exec("INSERT INTO tokens (user_id, name, "+tc.keyCol+", status, expired_time, remain_quota, used_quota, unlimited_quota, model_limits_enabled, cross_group_retry) VALUES (?, ?, ?, 1, -1, 0, 0, ?, ?, ?)",
				1, "legacy", legacy, true, false, false).Error)
			fresh := Token{UserId: 1, Name: "fresh", Key: "matrix-fresh-key-0123456789abcdefghijklmnopqr", Status: 1, ExpiredTime: -1}
			require.NoError(t, db.Create(&fresh).Error)

			for range 2 {
				require.NoError(t, migrateTokenKeyHashes())
			}
			var rows []Token
			require.NoError(t, db.Order("id").Find(&rows).Error)
			require.Len(t, rows, 2)
			assert.Equal(t, HashTokenKey(legacy), rows[0].Key)
			assert.Equal(t, TokenKeyHint(legacy), rows[0].KeyHint)
			assert.Equal(t, HashTokenKey("matrix-fresh-key-0123456789abcdefghijklmnopqr"), rows[1].Key)

			raw, err := RotateTokenKey(rows[0].Id, 1)
			require.NoError(t, err)
			var rotated Token
			require.NoError(t, db.Where(tc.keyCol+" = ?", HashTokenKey(raw)).First(&rotated).Error)
			assert.Equal(t, rows[0].Id, rotated.Id)
		})
	}
}
