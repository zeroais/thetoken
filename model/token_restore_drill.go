package model

import (
	"fmt"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// TOKEN: the tables the monthly restore drill compares between the live
// database and its point-in-time clone (service/token_restore_drill.go):
// money, identity and configuration.
var TokenDrillTables = []string{
	"users", "tokens", "top_ups", "logs", "options", "channels", "two_fas", "user_access_tokens",
	"token_starter_grants", "token_payment_reversals", "token_topup_charges",
}

// TokenTableCounts returns the row count of each drill table in db.
func TokenTableCounts(db *gorm.DB) (map[string]int64, error) {
	counts := make(map[string]int64, len(TokenDrillTables))
	for _, table := range TokenDrillTables {
		var n int64
		if err := db.Table(table).Count(&n).Error; err != nil {
			return nil, fmt.Errorf("count %s: %w", table, err)
		}
		counts[table] = n
	}
	return counts, nil
}

// TokenOpenPostgres opens a short-lived connection to another PostgreSQL
// database (the drill's clone).
func TokenOpenPostgres(dsn string) (*gorm.DB, func(), error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return nil, nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, err
	}
	sqlDB.SetMaxOpenConns(2)
	return db, func() { _ = sqlDB.Close() }, nil
}
