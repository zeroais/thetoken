package model

import (
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TOKEN: the starter credit (QuotaForNewUser) is given once per mailbox,
// ever, and only for an email address the user verified. The ledger keeps a
// keyed hash of the canonical address (never the address itself), and it
// outlives account deletion so deleting and re-registering, or registering
// an alias of the same mailbox, gets no second credit.
type TokenStarterGrant struct {
	EmailHash string `gorm:"primaryKey;type:varchar(64)"` // HMAC-SHA256 (CRYPTO_SECRET) of TokenCanonicalEmail
	UserId    int    `gorm:"index"`
	CreatedAt int64
}

// TokenCanonicalEmail maps every alias of one mailbox to the same address:
// lower case, "+tag" removed, and for Gmail the dots too (Gmail ignores them).
func TokenCanonicalEmail(email string) string {
	email = NormalizeEmail(email)
	at := strings.LastIndexByte(email, '@')
	if at <= 0 {
		return email
	}
	local, domain := email[:at], email[at+1:]
	if i := strings.IndexByte(local, '+'); i > 0 {
		local = local[:i]
	}
	if domain == "googlemail.com" {
		domain = "gmail.com"
	}
	if domain == "gmail.com" {
		local = strings.ReplaceAll(local, ".", "")
	}
	return local + "@" + domain
}

func tokenStarterEmailHash(email string) string {
	return common.GenerateHMAC(TokenCanonicalEmail(email))
}

// tokenStarterQuota claims the starter credit for a new user's mailbox inside
// the user's insert transaction and returns the quota to give: 0 when the
// address was not verified or its mailbox already had the credit. The
// primary key makes concurrent claims for one mailbox give it only once.
func tokenStarterQuota(tx *gorm.DB, email string, verified bool) (int, error) {
	if common.QuotaForNewUser <= 0 || !verified || NormalizeEmail(email) == "" {
		return 0, nil
	}
	grant := TokenStarterGrant{EmailHash: tokenStarterEmailHash(email), CreatedAt: common.GetTimestamp()}
	res := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&grant)
	if res.Error != nil {
		return 0, res.Error
	}
	if res.RowsAffected == 0 {
		return 0, nil
	}
	return common.QuotaForNewUser, nil
}

// tokenStarterGrantOwner records which user received the credit.
func tokenStarterGrantOwner(tx *gorm.DB, user *User) error {
	if user.Quota <= 0 || user.Email == "" {
		return nil
	}
	return tx.Model(&TokenStarterGrant{}).Where("email_hash = ?", tokenStarterEmailHash(user.Email)).Update("user_id", user.Id).Error
}

// migrateTokenStarterGrants creates the ledger; when it is new, the mailbox of
// every existing customer account counts as already credited. Staff accounts
// (administrators, root) never receive the starter credit, so they never hold
// a mailbox (see TokenReleaseStaffStarterGrants).
func migrateTokenStarterGrants() error {
	existed := DB.Migrator().HasTable(&TokenStarterGrant{})
	if err := DB.AutoMigrate(&TokenStarterGrant{}); err != nil {
		return err
	}
	if existed {
		return nil
	}
	var users []User
	if err := DB.Unscoped().Select("id", "email").Where("email <> '' AND role < ?", common.RoleAdminUser).Find(&users).Error; err != nil {
		return err
	}
	now := common.GetTimestamp()
	for _, u := range users {
		g := TokenStarterGrant{EmailHash: tokenStarterEmailHash(u.Email), UserId: u.Id, CreatedAt: now}
		if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&g).Error; err != nil {
			return err
		}
	}
	return nil
}

// TokenReleaseStaffStarterGrants frees staff accounts' mailboxes, which the
// first version of the backfill counted as credited, so customer accounts on
// those mailboxes (plus-address sign-ups by the owner) were refused. For each
// staff mailbox without a customer's grant, in one transaction: the staff row
// is removed; the earliest enabled customer account on that mailbox keeps it
// without a second credit if it was already credited (its gift log), and is
// otherwise credited now through the same ledger claim. Afterwards every such
// mailbox has a customer's row (or none), so later runs change nothing. It
// reads gift logs, so it runs after the log database is up (main.go, on nodes
// that migrate).
func TokenReleaseStaffStarterGrants() error {
	var staff []User
	if err := DB.Unscoped().Select("id", "email").Where("role >= ?", common.RoleAdminUser).Find(&staff).Error; err != nil {
		return err
	}
	if len(staff) == 0 {
		return nil
	}
	staffIDs := make([]int, 0, len(staff))
	mailboxes := map[string]bool{}
	for _, u := range staff {
		staffIDs = append(staffIDs, u.Id)
		if NormalizeEmail(u.Email) != "" {
			mailboxes[tokenStarterEmailHash(u.Email)] = true
		}
	}
	var held []TokenStarterGrant
	if err := DB.Where("user_id IN ?", staffIDs).Find(&held).Error; err != nil {
		return err
	}
	for _, g := range held {
		mailboxes[g.EmailHash] = true
	}
	if len(mailboxes) == 0 {
		return nil
	}
	var customers []User
	if err := DB.Where("role < ? AND status = ? AND email <> ''", common.RoleAdminUser, common.UserStatusEnabled).Order("id").Find(&customers).Error; err != nil {
		return err
	}
	onMailbox := map[string][]User{}
	for _, u := range customers {
		if hash := tokenStarterEmailHash(u.Email); mailboxes[hash] {
			onMailbox[hash] = append(onMailbox[hash], u)
		}
	}
	for hash := range mailboxes {
		var rows []TokenStarterGrant
		if err := DB.Where("email_hash = ?", hash).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) > 0 && !slices.Contains(staffIDs, rows[0].UserId) {
			continue // a customer already holds this mailbox
		}
		owner, credited, err := tokenMailboxOwner(onMailbox[hash])
		if err != nil {
			return err
		}
		if err := tokenReassignStaffMailbox(hash, staffIDs, owner, credited); err != nil {
			return err
		}
	}
	return nil
}

// tokenMailboxOwner picks the customer account a freed mailbox goes to: the
// earliest one already credited (gift log), else the earliest one (nil: none).
func tokenMailboxOwner(accounts []User) (*User, bool, error) {
	for i := range accounts {
		logged, err := tokenStarterCreditLogged(accounts[i].Id)
		if err != nil {
			return nil, false, err
		}
		if logged {
			return &accounts[i], true, nil
		}
	}
	if len(accounts) == 0 {
		return nil, false, nil
	}
	return &accounts[0], false, nil
}

// tokenReassignStaffMailbox removes a staff account's row for a mailbox and, in
// the same transaction, gives the mailbox to its customer owner: as a plain row
// when already credited, or with the starter credit otherwise.
func tokenReassignStaffMailbox(hash string, staffIDs []int, owner *User, credited bool) error {
	quota := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("email_hash = ? AND user_id IN ?", hash, staffIDs).Delete(&TokenStarterGrant{}).Error; err != nil {
			return err
		}
		if owner == nil {
			return nil
		}
		if credited {
			return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&TokenStarterGrant{EmailHash: hash, UserId: owner.Id, CreatedAt: common.GetTimestamp()}).Error
		}
		var err error
		if quota, err = tokenStarterQuota(tx, owner.Email, true); err != nil || quota == 0 {
			return err
		}
		if err := tx.Model(&TokenStarterGrant{}).Where("email_hash = ?", hash).Update("user_id", owner.Id).Error; err != nil {
			return err
		}
		return tx.Model(&User{}).Where("id = ?", owner.Id).Update("quota", gorm.Expr("quota + ?", quota)).Error
	})
	if err != nil || quota == 0 {
		return err
	}
	owner.Quota += quota
	_ = invalidateUserCache(owner.Id) // the cached balance is reloaded from the database
	RecordLog(owner.Id, LogTypeSystem, TokenStarterCreditLog+logger.LogQuota(quota))
	common.SysLog(fmt.Sprintf("TOKEN: starter credit given to user %d (its mailbox was held by a staff account)", owner.Id))
	return nil
}
