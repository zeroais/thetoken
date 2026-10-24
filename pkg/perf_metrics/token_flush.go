package perfmetrics

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
)

// FlushAll writes every bucket, the current hour too, to the database before
// the process exits (TOKEN: Cloud Run replaces instances on each deploy and
// scale-in, and New API flushes an hour only after it ends, so a stopping
// instance lost its open hour). Queries add the current hour's Redis counter
// on top of database rows, so the counts flushed for it are taken out of
// Redis. Called from main after the HTTP server has shut down.
func FlushAll() {
	current := bucketStart(time.Now().Unix())
	hotBuckets.Range(func(key, value any) bool {
		k := key.(bucketKey)
		bucket := value.(*atomicBucket)
		drained := bucket.drain()
		if drained.requestCount == 0 {
			return true
		}
		err := model.UpsertPerfMetric(&model.PerfMetric{
			ModelName:      k.model,
			Group:          k.group,
			BucketTs:       k.bucketTs,
			RequestCount:   drained.requestCount,
			SuccessCount:   drained.successCount,
			TotalLatencyMs: drained.totalLatencyMs,
			TtftSumMs:      drained.ttftSumMs,
			TtftCount:      drained.ttftCount,
			OutputTokens:   drained.outputTokens,
			GenerationMs:   drained.generationMs,
		})
		if err != nil {
			bucket.addCounters(drained)
			common.SysError(fmt.Sprintf("TOKEN: perf metric flush at shutdown model=%s group=%s bucket=%d: %s", k.model, k.group, k.bucketTs, err.Error()))
			return true
		}
		if k.bucketTs >= current {
			tokenTakeFromRedis(k, drained)
		}
		return true
	})
	model.TokenFlushPerfHistograms(math.MaxInt64)
}

// tokenTakeFromRedis subtracts counts now in the database from the current
// hour's Redis counter.
func tokenTakeFromRedis(key bucketKey, c counters) {
	if !common.RedisEnabled || common.RDB == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	redisKey := redisBucketKey(key)
	pipe := common.RDB.TxPipeline()
	for field, n := range map[string]int64{"req": c.requestCount, "ok": c.successCount, "lat": c.totalLatencyMs,
		"ttft": c.ttftSumMs, "ttft_n": c.ttftCount, "out": c.outputTokens, "gen_ms": c.generationMs} {
		if n != 0 {
			pipe.HIncrBy(ctx, redisKey, field, -n)
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		common.SysError("TOKEN: perf metric Redis adjustment at shutdown: " + err.Error())
	}
}
