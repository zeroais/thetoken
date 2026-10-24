package perfmetrics

import (
	"context"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/alicebob/miniredis/v2"
	"github.com/glebarez/sqlite"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestFlushAllKeepsTheOpenHourAtShutdown(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.PerfMetric{}, &model.TokenPerfHistogram{}))
	previousDB, previousRedis, previousRDB := model.DB, common.RedisEnabled, common.RDB
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	model.DB, common.RedisEnabled, common.RDB = db, true, client
	t.Cleanup(func() {
		model.DB, common.RedisEnabled, common.RDB = previousDB, previousRedis, previousRDB
		_ = client.Close()
		hotBuckets.Range(func(k, _ any) bool { hotBuckets.Delete(k); return true })
	})

	for range 12 {
		Record(Sample{Model: "moonshotai/kimi-k3", Group: "default", LatencyMs: 2300, HasTtft: true, TtftMs: 300,
			Success: true, OutputTokens: 100, GenerationMs: 2000})
	}
	key := bucketKey{model: "moonshotai/kimi-k3", group: "default", bucketTs: bucketStart(time.Now().Unix())}
	require.Eventually(t, func() bool {
		n, _ := client.HGet(context.Background(), redisBucketKey(key), "req").Int64()
		return n == 12
	}, 5*time.Second, 10*time.Millisecond, "samples reach the current hour's Redis counter")

	FlushAll()

	var row model.PerfMetric
	require.NoError(t, db.Where("model_name = ? AND bucket_ts = ?", key.model, key.bucketTs).First(&row).Error)
	assert.EqualValues(t, 12, row.RequestCount, "the open hour is written, not dropped")
	assert.EqualValues(t, 12, row.SuccessCount)
	req, err := client.HGet(context.Background(), redisBucketKey(key), "req").Int64()
	require.NoError(t, err)
	assert.Zero(t, req, "flushed counts are taken out of Redis, so the hour is not counted twice")

	var hist model.TokenPerfHistogram
	require.NoError(t, db.Where("model_name = ?", key.model).First(&hist).Error)
	assert.Equal(t, key.bucketTs, hist.BucketTs)
}
