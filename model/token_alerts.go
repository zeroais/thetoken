package model

import (
	"fmt"

	"gorm.io/gorm"
)

// TOKEN: aggregates for the operator alerts (service/token_alerts.go). The
// database is a parameter so the queries are tested on every engine.

// TokenSpend is the quota charged to one account, key or channel.
type TokenSpend struct {
	ID    int   `gorm:"column:id"`
	Quota int64 `gorm:"column:quota"`
}

// tokenSpendColumns are the log columns spending can be grouped by.
var tokenSpendColumns = map[string]bool{"user_id": true, "token_id": true, "channel_id": true}

// TokenSpendByGroup sums the quota of consume logs created in [since, until)
// per value of column (user_id, token_id or channel_id; 0 is skipped) and
// returns the groups that reached minQuota.
func TokenSpendByGroup(db *gorm.DB, column string, since, until, minQuota int64) ([]TokenSpend, error) {
	if !tokenSpendColumns[column] {
		return nil, fmt.Errorf("cannot group spending by %q", column)
	}
	var rows []TokenSpend
	err := db.Model(&Log{}).
		Select(column+" AS id, SUM(quota) AS quota").
		Where("type = ? AND created_at >= ? AND created_at < ? AND "+column+" > 0", LogTypeConsume, since, until).
		Group(column).
		Having("SUM(quota) >= ?", minQuota).
		Scan(&rows).Error
	return rows, err
}

// TokenSpendOf sums the quota of consume logs created in [since, until) with
// column = id.
func TokenSpendOf(db *gorm.DB, column string, id int, since, until int64) (int64, error) {
	if !tokenSpendColumns[column] {
		return 0, fmt.Errorf("cannot group spending by %q", column)
	}
	var total int64
	err := db.Model(&Log{}).
		Select("COALESCE(SUM(quota), 0)").
		Where("type = ? AND created_at >= ? AND created_at < ? AND "+column+" = ?", LogTypeConsume, since, until, id).
		Scan(&total).Error
	return total, err
}

// TokenSignupsBetween counts accounts created in [since, until), deleted
// ones included.
func TokenSignupsBetween(db *gorm.DB, since, until int64) (int64, error) {
	var n int64
	err := db.Unscoped().Model(&User{}).Where("created_at >= ? AND created_at < ?", since, until).Count(&n).Error
	return n, err
}
