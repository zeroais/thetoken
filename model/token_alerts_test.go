package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// TOKEN: the alert aggregates (token_alerts.go) give the same sums on SQLite,
// MySQL and PostgreSQL (TEST_MYSQL_DSN / TEST_POSTGRES_DSN select the
// latter two; each is skipped when unset).
func TestAlertAggregatesOnEveryDatabase(t *testing.T) {
	for _, dialect := range []string{"sqlite", "mysql", "postgres"} {
		t.Run(dialect, func(t *testing.T) {
			var driver gorm.Dialector
			switch dialect {
			case "sqlite":
				driver = sqlite.Open(filepath.Join(t.TempDir(), "alerts.db"))
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
				driver = postgres.Open(dsn)
			}
			db, err := gorm.Open(driver, &gorm.Config{})
			require.NoError(t, err)
			sqlDB, err := db.DB()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			require.NoError(t, db.AutoMigrate(&Log{}, &User{}))
			t.Cleanup(func() {
				db.Exec("DELETE FROM logs")
				db.Exec("DELETE FROM users")
			})
			var version string
			if dialect == "sqlite" {
				require.NoError(t, db.Raw("select sqlite_version()").Scan(&version).Error)
			} else {
				require.NoError(t, db.Raw("select version()").Scan(&version).Error)
			}
			t.Logf("%s version: %s", dialect, version)

			logs := []Log{
				{UserId: 1, TokenId: 11, ChannelId: 5, Type: LogTypeConsume, CreatedAt: 1000, Quota: 300},
				{UserId: 1, TokenId: 12, ChannelId: 5, Type: LogTypeConsume, CreatedAt: 1500, Quota: 200},
				{UserId: 2, TokenId: 21, ChannelId: 6, Type: LogTypeConsume, CreatedAt: 1200, Quota: 100},
				{UserId: 2, TokenId: 21, ChannelId: 6, Type: LogTypeTopup, CreatedAt: 1300, Quota: 9999}, // not spending
				{UserId: 3, TokenId: 0, ChannelId: 0, Type: LogTypeConsume, CreatedAt: 1400, Quota: 700}, // no key, no channel
				{UserId: 1, TokenId: 11, ChannelId: 5, Type: LogTypeConsume, CreatedAt: 2000, Quota: 50}, // after the window
			}
			require.NoError(t, db.Create(&logs).Error)
			for i, at := range []int64{900, 1100, 1999, 2000} {
				require.NoError(t, db.Create(&User{Username: "alerts" + string(rune('a'+i)), AffCode: "alerts" + string(rune('a'+i)), CreatedAt: at}).Error)
			}

			byUser, err := TokenSpendByGroup(db, "user_id", 1000, 2000, 150)
			require.NoError(t, err)
			assert.ElementsMatch(t, []TokenSpend{{ID: 1, Quota: 500}, {ID: 3, Quota: 700}}, byUser)
			byKey, err := TokenSpendByGroup(db, "token_id", 1000, 2000, 0)
			require.NoError(t, err)
			assert.ElementsMatch(t, []TokenSpend{{ID: 11, Quota: 300}, {ID: 12, Quota: 200}, {ID: 21, Quota: 100}}, byKey)
			byChannel, err := TokenSpendByGroup(db, "channel_id", 1000, 2000, 1)
			require.NoError(t, err)
			assert.ElementsMatch(t, []TokenSpend{{ID: 5, Quota: 500}, {ID: 6, Quota: 100}}, byChannel)

			one, err := TokenSpendOf(db, "token_id", 11, 0, 3000)
			require.NoError(t, err)
			assert.Equal(t, int64(350), one)
			none, err := TokenSpendOf(db, "user_id", 99, 0, 3000)
			require.NoError(t, err)
			assert.Zero(t, none)

			signups, err := TokenSignupsBetween(db, 1000, 2000)
			require.NoError(t, err)
			assert.Equal(t, int64(2), signups)

			_, err = TokenSpendByGroup(db, "quota; DROP TABLE logs", 0, 1, 0)
			assert.Error(t, err, "only known columns")
		})
	}
}
