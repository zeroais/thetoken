package service

import (
	"testing"

	"github.com/QuantumNous/new-api/model"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TOKEN: the restore drill passes only when every table of the clone holds
// between the live counts taken just before and just after the restore
// point, and it reaches the clone with the live credentials at its address.
func TestRestoreDrillComparesTheCloneWithTheLiveCounts(t *testing.T) {
	counts := func(n int64) map[string]int64 {
		m := map[string]int64{}
		for _, table := range model.TokenDrillTables {
			m[table] = n
		}
		return m
	}
	before, after := counts(10), counts(10)
	after["logs"] = 14 // requests logged while the restore point passed

	restored := counts(10)
	restored["logs"] = 12
	assert.NoError(t, tokenCheckDrillCounts(before, after, restored))

	restored["top_ups"] = 9 // a payment missing from the restore
	restored["logs"] = 15   // rows that never existed
	err := tokenCheckDrillCounts(before, after, restored)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "top_ups")
	assert.Contains(t, err.Error(), "logs")

	dsn, err := tokenDSNWithHost("postgresql://token:p%40ss@10.0.0.3:5432/token?sslmode=require", "10.0.0.9")
	require.NoError(t, err)
	assert.Equal(t, "postgresql://token:p%40ss@10.0.0.9:5432/token?sslmode=require", dsn)
}
