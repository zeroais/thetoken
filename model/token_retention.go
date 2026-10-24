package model

import (
	"time"

	"github.com/QuantumNous/new-api/common"
)

// TOKEN: data retention promised in legal/privacy-policy.md. Log rows older
// than their period are deleted in batches; the token-prod-retention Cloud Run
// job runs this daily (TOKEN_RETENTION_ONLY, common/token_deploy.go). Log types
// not listed are kept.
var TokenRetentionDays = map[int]int{
	LogTypeError:   30,
	LogTypeConsume: 400,
	LogTypeTopup:   400,
	LogTypeManage:  400,
	LogTypeSystem:  400,
	LogTypeRefund:  400,
}

// TokenRetentionResult is what one retention run found and removed for a type.
type TokenRetentionResult struct {
	LogType int
	Days    int
	Older   int64 // rows past the period when the run started
	Deleted int64
}

// TokenEnforceRetention deletes LOG_DB rows past their retention period, in
// batches of batch rows (ids are selected first, which works the same on
// SQLite, MySQL and PostgreSQL). With dryRun it only counts. ClickHouse log
// databases use a row TTL instead and are left alone.
func TokenEnforceRetention(now time.Time, batch int, dryRun bool) ([]TokenRetentionResult, error) {
	if common.UsingLogDatabase(common.DatabaseTypeClickHouse) {
		return nil, nil
	}
	results := make([]TokenRetentionResult, 0, len(TokenRetentionDays))
	for _, logType := range []int{LogTypeError, LogTypeConsume, LogTypeTopup, LogTypeManage, LogTypeSystem, LogTypeRefund} {
		days := TokenRetentionDays[logType]
		cutoff := now.AddDate(0, 0, -days).Unix()
		result := TokenRetentionResult{LogType: logType, Days: days}
		if err := LOG_DB.Model(&Log{}).Where("type = ? AND created_at < ?", logType, cutoff).Count(&result.Older).Error; err != nil {
			return results, err
		}
		for !dryRun && result.Older > 0 {
			var ids []int
			if err := LOG_DB.Model(&Log{}).Where("type = ? AND created_at < ?", logType, cutoff).
				Order("id").Limit(batch).Pluck("id", &ids).Error; err != nil {
				return results, err
			}
			if len(ids) == 0 {
				break
			}
			res := LOG_DB.Where("id IN ?", ids).Delete(&Log{})
			if res.Error != nil {
				return results, res.Error
			}
			result.Deleted += res.RowsAffected
		}
		results = append(results, result)
	}
	return results, nil
}
