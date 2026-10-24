package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenEnforceRetentionDeletesOnlyRowsPastTheirPeriod(t *testing.T) {
	tokenDialects(t, []any{&Log{}}, func(t *testing.T) {
		now := time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC)
		daysAgo := func(d int) int64 { return now.AddDate(0, 0, -d).Unix() }
		rows := []Log{
			{Type: LogTypeError, CreatedAt: daysAgo(31)},    // past 30 days: deleted
			{Type: LogTypeError, CreatedAt: daysAgo(29)},    // kept
			{Type: LogTypeConsume, CreatedAt: daysAgo(401)}, // past 400 days: deleted
			{Type: LogTypeConsume, CreatedAt: daysAgo(31)},  // kept: usage records live 400 days
			{Type: LogTypeTopup, CreatedAt: daysAgo(399)},   // kept
			{Type: 99, CreatedAt: daysAgo(5000)},            // unlisted type: kept
		}
		require.NoError(t, LOG_DB.Create(&rows).Error)

		dry, err := TokenEnforceRetention(now, 1, true)
		require.NoError(t, err)
		assert.Equal(t, TokenRetentionResult{LogType: LogTypeError, Days: 30, Older: 1}, dry[0])
		var count int64
		require.NoError(t, LOG_DB.Model(&Log{}).Count(&count).Error)
		assert.Equal(t, int64(6), count, "a dry run deletes nothing")

		results, err := TokenEnforceRetention(now, 1, false)
		require.NoError(t, err)
		deleted := map[int]int64{}
		for _, r := range results {
			deleted[r.LogType] = r.Deleted
		}
		assert.Equal(t, map[int]int64{LogTypeError: 1, LogTypeConsume: 1, LogTypeTopup: 0, LogTypeManage: 0, LogTypeSystem: 0, LogTypeRefund: 0}, deleted)
		type row struct {
			Type    int
			Created int64
		}
		var kept []Log
		require.NoError(t, LOG_DB.Order("id").Find(&kept).Error)
		got := make([]row, 0, len(kept))
		for _, l := range kept {
			got = append(got, row{l.Type, l.CreatedAt})
		}
		assert.Equal(t, []row{
			{LogTypeError, daysAgo(29)},
			{LogTypeConsume, daysAgo(31)},
			{LogTypeTopup, daysAgo(399)},
			{99, daysAgo(5000)},
		}, got)
	})
}
