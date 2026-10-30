package model

import (
	"errors"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TOKEN: email verification and password reset codes, shared by every
// instance through the main database (common/token_verification.go). Id is
// an HMAC of the purpose and the mailbox, CodeHash an HMAC of the code: the
// table holds neither addresses nor usable codes. One code per mailbox and
// purpose; expired codes are deleted whenever a new code is stored.
type TokenVerificationCode struct {
	Id        string `gorm:"primaryKey;type:varchar(64)"`
	CodeHash  string `gorm:"type:varchar(64);not null"`
	Attempts  int    `gorm:"not null"`
	ExpiresAt int64  `gorm:"index;not null"`
}

// SaveTokenVerificationCode stores id's code, replacing an earlier one and
// its attempt count.
func SaveTokenVerificationCode(db *gorm.DB, id, codeHash string, expiresAt, now int64) error {
	if err := db.Where("expires_at <= ?", now).Delete(&TokenVerificationCode{}).Error; err != nil {
		return err
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"code_hash", "attempts", "expires_at"}),
	}).Create(&TokenVerificationCode{Id: id, CodeHash: codeHash, ExpiresAt: expiresAt}).Error
}

// CheckTokenVerificationCode reports whether codeHash is id's unexpired code
// with attempts left. With consume the code is deleted by the same
// statement, so of concurrent callers only one succeeds. A failed check
// counts an attempt and deletes the code once it is used up or expired.
func CheckTokenVerificationCode(db *gorm.DB, id, codeHash string, now int64, consume bool) (bool, error) {
	live := "id = ? AND code_hash = ? AND expires_at > ? AND attempts < ?"
	maxAttempts := common.TokenVerificationMaxAttempts
	if consume {
		result := db.Where(live, id, codeHash, now, maxAttempts).Delete(&TokenVerificationCode{})
		if result.Error != nil {
			return false, result.Error
		}
		if result.RowsAffected == 1 {
			return true, nil
		}
	} else {
		var count int64
		if err := db.Model(&TokenVerificationCode{}).Where(live, id, codeHash, now, maxAttempts).Count(&count).Error; err != nil {
			return false, err
		}
		if count == 1 {
			return true, nil
		}
	}
	if err := db.Model(&TokenVerificationCode{}).Where("id = ?", id).
		Update("attempts", gorm.Expr("attempts + 1")).Error; err != nil {
		return false, err
	}
	return false, db.Where("id = ? AND (attempts >= ? OR expires_at <= ?)", id, maxAttempts, now).
		Delete(&TokenVerificationCode{}).Error
}

// tokenVerificationCodes is the store on the main database (the caller
// falls back to the instance's memory while there is none).
type tokenVerificationCodes struct{}

var errNoMainDatabase = errors.New("main database not open")

func (tokenVerificationCodes) Save(id, codeHash string, expiresAt, now int64) error {
	if DB == nil {
		return errNoMainDatabase
	}
	return SaveTokenVerificationCode(DB, id, codeHash, expiresAt, now)
}

func (tokenVerificationCodes) Check(id, codeHash string, now int64, consume bool) (bool, error) {
	if DB == nil {
		return false, errNoMainDatabase
	}
	return CheckTokenVerificationCode(DB, id, codeHash, now, consume)
}

func (tokenVerificationCodes) Delete(id string) error {
	if DB == nil {
		return errNoMainDatabase
	}
	return DB.Where("id = ?", id).Delete(&TokenVerificationCode{}).Error
}

func init() {
	common.TokenVerificationCodes = tokenVerificationCodes{}
}
