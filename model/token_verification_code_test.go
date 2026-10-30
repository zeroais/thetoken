package model

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TOKEN: verification codes in the main database (token_verification_code.go)
// on SQLite, MySQL and PostgreSQL (TEST_MYSQL_DSN / TEST_POSTGRES_DSN select
// the latter two; each is skipped when unset). A code sent by one instance is
// accepted by another that never saw it (sign-up across instances and after
// scale to zero), only HMACs are stored, and a code expires, is replaced by a
// newer one, works once even under concurrent use, and is deleted after
// TokenVerificationMaxAttempts wrong guesses.
func TestVerificationCodesInTheDatabase(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "codes.db") + "?_pragma=busy_timeout(5000)")
			case "mysql":
				dsn := os.Getenv("TEST_MYSQL_DSN")
				if dsn == "" {
					t.Skip("TEST_MYSQL_DSN is not configured")
				}
				driver = mysql.Open(dsn)
			case "postgres":
				dsn := os.Getenv("TEST_POSTGRES_DSN")
				if dsn == "" {
					t.Skip("TEST_POSTGRES_DSN is not configured")
				}
				driver = postgres.New(postgres.Config{DSN: dsn, PreferSimpleProtocol: true})
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() {
				_ = db.Migrator().DropTable(&TokenVerificationCode{})
				require.NoError(t, sqlDB.Close())
			})
			_ = db.Migrator().DropTable(&TokenVerificationCode{})
			for range 2 { // the migration is idempotent
				require.NoError(t, db.AutoMigrate(&TokenVerificationCode{}))
			}

			savedDB, savedStore := DB, common.TokenVerificationCodes
			DB, common.TokenVerificationCodes = db, tokenVerificationCodes{}
			t.Cleanup(func() { DB, common.TokenVerificationCodes = savedDB, savedStore })

			// Through the console's functions, with no code in this
			// instance's memory: another instance or a restarted one.
			const email = "contact+beta1@example.com"
			common.RegisterVerificationCodeWithKey(email, "aaa111", common.EmailVerificationPurpose)
			common.RegisterVerificationCodeWithKey(email, "bbb222", common.EmailVerificationPurpose)
			assert.False(t, common.VerifyCodeWithKey(email, "aaa111", common.EmailVerificationPurpose), "a newer code replaces an older one")
			assert.True(t, common.VerifyCodeWithKey(email, "bbb222", common.EmailVerificationPurpose), "checking does not use the code up")
			assert.False(t, common.VerifyCodeWithKey(email, "bbb222", common.PasswordResetPurpose), "codes are per purpose")
			assert.False(t, common.VerifyCodeWithKey("contact@example.com", "bbb222", common.EmailVerificationPurpose), "codes are per address")
			var rows []TokenVerificationCode
			require.NoError(t, db.Find(&rows).Error)
			require.Len(t, rows, 1, "one code per mailbox and purpose")
			assert.NotContains(t, rows[0].Id, "beta1", "no address is stored")
			assert.NotEqual(t, "bbb222", rows[0].CodeHash, "no usable code is stored")
			assert.True(t, common.ConsumeCodeWithKey(email, "bbb222", common.EmailVerificationPurpose))
			assert.False(t, common.ConsumeCodeWithKey(email, "bbb222", common.EmailVerificationPurpose), "a code works once")
			assert.False(t, common.VerifyCodeWithKey(email, "bbb222", common.EmailVerificationPurpose))

			// Concurrent use of one code: exactly one succeeds.
			common.RegisterVerificationCodeWithKey(email, "ccc333", common.PasswordResetPurpose)
			var wins atomic.Int32
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					if common.ConsumeCodeWithKey(email, "ccc333", common.PasswordResetPurpose) {
						wins.Add(1)
					}
				})
			}
			wg.Wait()
			assert.EqualValues(t, 1, wins.Load(), "one reset per link")

			const id, code = "mailbox", "code-hash"
			now := int64(1_000_000)

			// Expiry: a code is refused from its expiry time and deleted.
			require.NoError(t, SaveTokenVerificationCode(db, id, code, now+600, now))
			ok, err := CheckTokenVerificationCode(db, id, code, now+599, false)
			require.NoError(t, err)
			assert.True(t, ok)
			ok, err = CheckTokenVerificationCode(db, id, code, now+600, true)
			require.NoError(t, err)
			assert.False(t, ok, "expired")
			var count int64
			require.NoError(t, db.Model(&TokenVerificationCode{}).Where("id = ?", id).Count(&count).Error)
			assert.Zero(t, count, "an expired code is deleted when it is tried")

			// Wrong guesses: the code is deleted after the last allowed one,
			// so even the right code is then refused.
			require.NoError(t, SaveTokenVerificationCode(db, id, code, now+600, now))
			for i := range common.TokenVerificationMaxAttempts {
				ok, err = CheckTokenVerificationCode(db, id, "wrong", now, i%2 == 0)
				require.NoError(t, err)
				assert.False(t, ok)
			}
			ok, err = CheckTokenVerificationCode(db, id, code, now, true)
			require.NoError(t, err)
			assert.False(t, ok, "attempts used up")

			// A new code resets the attempts; storing it clears expired codes.
			require.NoError(t, SaveTokenVerificationCode(db, "old", code, now, now-600))
			require.NoError(t, SaveTokenVerificationCode(db, id, "new", now+600, now))
			ok, err = CheckTokenVerificationCode(db, id, "new", now, true)
			require.NoError(t, err)
			assert.True(t, ok)
			require.NoError(t, db.Model(&TokenVerificationCode{}).Where("id = ?", "old").Count(&count).Error)
			assert.Zero(t, count, "expired codes are cleared")
		})
	}
}
