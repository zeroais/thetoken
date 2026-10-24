package model

import (
	"os"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestTokenCanonicalEmailFoldsAliasesOfOneMailbox(t *testing.T) {
	for in, want := range map[string]string{
		" John.Smith+promo@Gmail.com ": "johnsmith@gmail.com",
		"j.o.h.n.smith@googlemail.com": "johnsmith@gmail.com",
		"john.smith+x@company.com":     "john.smith@company.com",
		"john.smith@company.com":       "john.smith@company.com",
		"+tag@example.com":             "+tag@example.com",
		"not-an-email":                 "not-an-email",
	} {
		assert.Equal(t, want, TokenCanonicalEmail(in), in)
	}
}

// tokenDialects runs a TOKEN model test on SQLite (the package's test
// database) and, when TEST_MYSQL_DSN / TEST_POSTGRES_DSN are set, on MySQL and
// PostgreSQL, with DB and LOG_DB on that database and the given tables
// migrated and emptied.
func tokenDialects(t *testing.T, tables []any, test func(t *testing.T)) {
	for _, d := range []struct{ name, env string }{{"sqlite", ""}, {"mysql", "TEST_MYSQL_DSN"}, {"postgres", "TEST_POSTGRES_DSN"}} {
		t.Run(d.name, func(t *testing.T) {
			if d.env != "" {
				if os.Getenv(d.env) == "" {
					t.Skip(d.env + " is not set")
				}
				db, dbType, err := chooseDB(d.env, false)
				require.NoError(t, err)
				sqlDB, err := db.DB()
				require.NoError(t, err)
				previousDB, previousLogDB := DB, LOG_DB
				previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
				DB, LOG_DB = db, db
				common.SetDatabaseTypes(dbType, dbType)
				t.Cleanup(func() {
					DB, LOG_DB = previousDB, previousLogDB
					common.SetDatabaseTypes(previousMain, previousLog)
					_ = sqlDB.Close()
				})
			}
			require.NoError(t, DB.AutoMigrate(tables...))
			empty := func() {
				for _, table := range tables {
					DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Unscoped().Delete(table)
				}
			}
			empty()
			t.Cleanup(empty)
			test(t)
		})
	}
}

func useStarterCredit(t *testing.T, verified bool, test func(t *testing.T)) {
	quota, verification := common.QuotaForNewUser, common.EmailVerificationEnabled
	t.Cleanup(func() { common.QuotaForNewUser, common.EmailVerificationEnabled = quota, verification })
	tokenDialects(t, []any{&User{}, &Log{}, &TokenStarterGrant{}}, func(t *testing.T) {
		common.QuotaForNewUser, common.EmailVerificationEnabled = 250000, verified
		test(t)
	})
}

func insertStarterUser(t *testing.T, username, email string) User {
	t.Helper()
	u := &User{Username: username, Email: email, Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
	require.NoError(t, u.Insert(0))
	var stored User
	require.NoError(t, DB.Where("username = ?", username).First(&stored).Error)
	return stored
}

func TestStarterCreditIsGivenOncePerVerifiedMailbox(t *testing.T) {
	useStarterCredit(t, true, func(t *testing.T) {
		first := insertStarterUser(t, "first", "John.Smith+a@gmail.com")
		assert.Equal(t, 250000, first.Quota)
		var grant TokenStarterGrant
		require.NoError(t, DB.First(&grant).Error)
		assert.Equal(t, first.Id, grant.UserId)
		assert.NotContains(t, grant.EmailHash, "john", "only a keyed hash of the address is stored")

		alias := insertStarterUser(t, "alias", "johnsmith@googlemail.com")
		assert.Zero(t, alias.Quota, "an alias of a credited mailbox gets no second credit")

		// Deleting the account does not free the mailbox for another credit.
		require.NoError(t, DB.Unscoped().Delete(&User{}, first.Id).Error)
		again := insertStarterUser(t, "again", "john.smith@gmail.com")
		assert.Zero(t, again.Quota)

		other := insertStarterUser(t, "other", "someone@example.com")
		assert.Equal(t, 250000, other.Quota)

		var gifts int64
		require.NoError(t, DB.Model(&Log{}).Where("type = ? AND content LIKE ?", LogTypeSystem, TokenStarterCreditLog+"%").Count(&gifts).Error)
		assert.EqualValues(t, 2, gifts, "the gift is logged only when given")
	})
}

func TestStarterCreditNeedsAVerifiedEmail(t *testing.T) {
	useStarterCredit(t, false, func(t *testing.T) {
		assert.Zero(t, insertStarterUser(t, "unverified", "new@example.com").Quota, "email verification is off")

		common.EmailVerificationEnabled = true
		assert.Zero(t, insertStarterUser(t, "no-email", "").Quota)

		oauth := &User{Username: "oauth", Email: "oauth@example.com", Role: common.RoleCommonUser, Status: common.UserStatusEnabled}
		require.NoError(t, oauth.InsertWithTx(DB, 0))
		assert.Zero(t, oauth.Quota, "OAuth and admin-created accounts have no verified email at creation")

		var grants int64
		require.NoError(t, DB.Model(&TokenStarterGrant{}).Count(&grants).Error)
		assert.Zero(t, grants)
	})
}

func TestStarterGrantLedgerCountsExistingAccountsWhenCreated(t *testing.T) {
	useStarterCredit(t, true, func(t *testing.T) {
		common.QuotaForNewUser = 0 // existing account created before the ledger
		insertStarterUser(t, "existing", "existing@example.com")
		require.NoError(t, DB.Migrator().DropTable(&TokenStarterGrant{}))

		require.NoError(t, migrateTokenStarterGrants())
		require.NoError(t, migrateTokenStarterGrants(), "idempotent")

		common.QuotaForNewUser = 250000
		assert.Zero(t, insertStarterUser(t, "existing-alias", "existing+2@example.com").Quota)
	})
}

func createStaff(t *testing.T, email string) User {
	t.Helper()
	root := User{Username: "root", Email: email, Role: common.RoleRootUser, Status: common.UserStatusEnabled, AffCode: "root"}
	require.NoError(t, DB.Create(&root).Error)
	return root
}

func starterGifts(t *testing.T, userID int) int64 {
	t.Helper()
	var n int64
	require.NoError(t, LOG_DB.Model(&Log{}).Where("user_id = ? AND type = ? AND content LIKE ?", userID, LogTypeSystem, TokenStarterCreditLog+"%").Count(&n).Error)
	return n
}

func TestStaffMailboxDoesNotUseUpTheStarterCredit(t *testing.T) {
	useStarterCredit(t, true, func(t *testing.T) {
		createStaff(t, "owner@gmail.com")
		require.NoError(t, DB.Migrator().DropTable(&TokenStarterGrant{}))
		require.NoError(t, migrateTokenStarterGrants())

		assert.Equal(t, 250000, insertStarterUser(t, "journey", "owner+test1@gmail.com").Quota)
	})
}

func TestStaffHeldGrantIsReleasedToTheRefusedAccountOnce(t *testing.T) {
	useStarterCredit(t, true, func(t *testing.T) {
		root := createStaff(t, "owner@gmail.com")
		// The first backfill gave the staff account the mailbox, so this
		// customer alias was refused at sign-up.
		require.NoError(t, DB.Create(&TokenStarterGrant{EmailHash: tokenStarterEmailHash(root.Email), UserId: root.Id, CreatedAt: 1}).Error)
		refused := insertStarterUser(t, "journey", "owner+test1@gmail.com")
		require.Zero(t, refused.Quota)
		later := insertStarterUser(t, "journey2", "owner+test2@gmail.com")

		require.NoError(t, migrateTokenStarterGrants())
		require.NoError(t, TokenReleaseStaffStarterGrants())
		require.NoError(t, TokenReleaseStaffStarterGrants(), "runs once")

		var got User
		require.NoError(t, DB.First(&got, refused.Id).Error)
		assert.Equal(t, 250000, got.Quota)
		assert.EqualValues(t, 1, starterGifts(t, refused.Id))
		var other User
		require.NoError(t, DB.First(&other, later.Id).Error)
		assert.Zero(t, other.Quota, "one credit per mailbox")
		var grant TokenStarterGrant
		require.NoError(t, DB.Where("email_hash = ?", tokenStarterEmailHash(root.Email)).First(&grant).Error)
		assert.Equal(t, refused.Id, grant.UserId)
	})
}

func TestReleasedMailboxStaysWithAnAlreadyCreditedAccount(t *testing.T) {
	useStarterCredit(t, true, func(t *testing.T) {
		root := createStaff(t, "owner@gmail.com")
		credited := insertStarterUser(t, "early", "owner+early@gmail.com") // credited before the staff row existed
		require.Equal(t, 250000, credited.Quota)
		require.NoError(t, DB.Where("1 = 1").Delete(&TokenStarterGrant{}).Error)
		require.NoError(t, DB.Create(&TokenStarterGrant{EmailHash: tokenStarterEmailHash(root.Email), UserId: root.Id, CreatedAt: 1}).Error)

		require.NoError(t, TokenReleaseStaffStarterGrants())

		var got User
		require.NoError(t, DB.First(&got, credited.Id).Error)
		assert.Equal(t, 250000, got.Quota, "no second credit")
		assert.EqualValues(t, 1, starterGifts(t, credited.Id))
	})
}

func TestRefusedAccountIsCreditedAfterAnInterruptedRelease(t *testing.T) {
	useStarterCredit(t, true, func(t *testing.T) {
		// Production after the first repair crashed: the staff row was
		// already removed, the refused account not yet credited.
		root := createStaff(t, "owner@gmail.com")
		require.NoError(t, DB.Create(&TokenStarterGrant{EmailHash: tokenStarterEmailHash(root.Email), UserId: root.Id, CreatedAt: 1}).Error)
		refused := insertStarterUser(t, "journey", "owner+test1@gmail.com")
		require.Zero(t, refused.Quota)
		require.NoError(t, DB.Where("1 = 1").Delete(&TokenStarterGrant{}).Error)

		require.NoError(t, TokenReleaseStaffStarterGrants())
		require.NoError(t, TokenReleaseStaffStarterGrants(), "runs once")

		var got User
		require.NoError(t, DB.First(&got, refused.Id).Error)
		assert.Equal(t, 250000, got.Quota)
		assert.EqualValues(t, 1, starterGifts(t, refused.Id))
	})
}
