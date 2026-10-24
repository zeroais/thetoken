package model

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TOKEN: public per-model performance (GET /public/metrics). New API's
// perf_metrics rows hold sums, which give means; the website shows medians
// and p90s, so streamed requests are also counted in fixed histograms per
// model and hour: time to first token (ms) and output speed (tokens/s after
// the first token). Recorded where New API samples each finished request
// (pkg/perf_metrics), flushed with its completed hourly buckets, kept 31 days.
// Only aggregate counts are stored: no user, key, request or content data.

// Upper bounds of the histogram bins; the last bin holds everything above.
var (
	tokenTTFTBinsMs  = []float64{100, 200, 300, 400, 500, 600, 800, 1000, 1250, 1500, 2000, 2500, 3000, 4000, 5000, 7500, 10000, 15000, 20000, 30000, 60000}
	tokenSpeedBinsTS = []float64{5, 10, 15, 20, 25, 30, 40, 50, 60, 70, 80, 100, 125, 150, 200, 250, 300, 400, 500, 750, 1000}
)

const (
	tokenPerfRetentionDays = 31
	// Fewer streamed samples than this give no percentile (too noisy, and a
	// single customer's requests could be read from it).
	tokenPerfMinSamples = 10
)

type TokenPerfHistogram struct {
	Id        int    `gorm:"primaryKey"`
	ModelName string `gorm:"size:128;uniqueIndex:idx_token_perf_model_bucket,priority:1"`
	BucketTs  int64  `gorm:"uniqueIndex:idx_token_perf_model_bucket,priority:2;index"`
	TTFT      string `gorm:"column:ttft;type:text"`  // JSON array of counts per tokenTTFTBinsMs bin
	Speed     string `gorm:"column:speed;type:text"` // JSON array of counts per tokenSpeedBinsTS bin
}

type tokenPerfKey struct {
	model    string
	bucketTs int64
}

type tokenPerfCounts struct{ ttft, speed []int64 }

var tokenPerfHot = struct {
	sync.Mutex
	buckets map[tokenPerfKey]*tokenPerfCounts
}{buckets: map[tokenPerfKey]*tokenPerfCounts{}}

func tokenPerfBin(bounds []float64, v float64) int {
	for i, b := range bounds {
		if v <= b {
			return i
		}
	}
	return len(bounds)
}

// TokenRecordPerfSample counts one finished request. Only streamed requests
// that sent a first token have a time to first token and an output speed.
func TokenRecordPerfSample(modelName string, bucketTs int64, streamed bool, ttftMs, outputTokens, generationMs int64) {
	if modelName == "" || !streamed || ttftMs < 0 {
		return
	}
	tokenPerfHot.Lock()
	defer tokenPerfHot.Unlock()
	key := tokenPerfKey{modelName, bucketTs}
	c := tokenPerfHot.buckets[key]
	if c == nil {
		c = &tokenPerfCounts{ttft: make([]int64, len(tokenTTFTBinsMs)+1), speed: make([]int64, len(tokenSpeedBinsTS)+1)}
		tokenPerfHot.buckets[key] = c
	}
	c.ttft[tokenPerfBin(tokenTTFTBinsMs, float64(ttftMs))]++
	if outputTokens > 0 && generationMs > 0 {
		c.speed[tokenPerfBin(tokenSpeedBinsTS, float64(outputTokens)*1000/float64(generationMs))]++
	}
}

// TokenFlushPerfHistograms adds the completed buckets (bucketTs before
// currentBucket) to the database and drops rows past the retention.
func TokenFlushPerfHistograms(currentBucket int64) {
	tokenPerfHot.Lock()
	ready := map[tokenPerfKey]*tokenPerfCounts{}
	for k, c := range tokenPerfHot.buckets {
		if k.bucketTs < currentBucket {
			ready[k] = c
			delete(tokenPerfHot.buckets, k)
		}
	}
	tokenPerfHot.Unlock()
	for k, c := range ready {
		if err := tokenAddPerfHistogram(k, c); err != nil {
			common.SysError(fmt.Sprintf("TOKEN: perf histogram flush model=%s bucket=%d: %v", k.model, k.bucketTs, err))
			tokenPerfHot.Lock() // keep the counts for the next flush
			if cur := tokenPerfHot.buckets[k]; cur != nil {
				tokenPerfAdd(cur.ttft, c.ttft)
				tokenPerfAdd(cur.speed, c.speed)
			} else {
				tokenPerfHot.buckets[k] = c
			}
			tokenPerfHot.Unlock()
		}
	}
	cutoff := time.Now().Add(-tokenPerfRetentionDays * 24 * time.Hour).Unix()
	if err := DB.Where("bucket_ts < ?", cutoff).Delete(&TokenPerfHistogram{}).Error; err != nil {
		common.SysError("TOKEN: perf histogram cleanup: " + err.Error())
	}
}

func tokenPerfAdd(dst, src []int64) {
	for i := range min(len(dst), len(src)) {
		dst[i] += src[i]
	}
}

func tokenPerfDecode(s string, n int) []int64 {
	out := make([]int64, n)
	var stored []int64
	if s != "" && common.UnmarshalJsonStr(s, &stored) == nil {
		tokenPerfAdd(out, stored)
	}
	return out
}

// tokenAddPerfHistogram adds counts to a bucket row; instances flush the same
// hour concurrently, so the row is locked while it is updated.
func tokenAddPerfHistogram(k tokenPerfKey, c *tokenPerfCounts) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		empty := TokenPerfHistogram{ModelName: k.model, BucketTs: k.bucketTs, TTFT: "[]", Speed: "[]"}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&empty).Error; err != nil {
			return err
		}
		var row TokenPerfHistogram
		if err := lockForUpdate(tx).Where("model_name = ? AND bucket_ts = ?", k.model, k.bucketTs).First(&row).Error; err != nil {
			return err
		}
		ttft := tokenPerfDecode(row.TTFT, len(tokenTTFTBinsMs)+1)
		speed := tokenPerfDecode(row.Speed, len(tokenSpeedBinsTS)+1)
		tokenPerfAdd(ttft, c.ttft)
		tokenPerfAdd(speed, c.speed)
		ttftJSON, err := common.Marshal(ttft)
		if err != nil {
			return err
		}
		speedJSON, err := common.Marshal(speed)
		if err != nil {
			return err
		}
		return tx.Model(&TokenPerfHistogram{}).Where("id = ?", row.Id).
			Updates(map[string]any{"ttft": string(ttftJSON), "speed": string(speedJSON)}).Error
	})
}

// tokenPerfPercentile estimates a percentile from bin counts, interpolating
// linearly inside the bin (the open last bin reports its lower bound).
func tokenPerfPercentile(bounds []float64, counts []int64, q float64) *float64 {
	var total int64
	for _, n := range counts {
		total += n
	}
	if total < tokenPerfMinSamples {
		return nil
	}
	target := q * float64(total)
	var seen float64
	for i, n := range counts {
		if n == 0 {
			continue
		}
		if seen+float64(n) >= target {
			lo := 0.0
			if i > 0 {
				lo = bounds[i-1]
			}
			v := lo
			if i < len(bounds) {
				v = lo + (bounds[i]-lo)*(target-seen)/float64(n)
			}
			v = float64(int64(v*10+0.5)) / 10
			return &v
		}
		seen += float64(n)
	}
	return nil
}

// TokenPublicMetrics is the body of GET /public/metrics.
type TokenPublicMetrics struct {
	GeneratedAt string                    `json:"generated_at"`
	AsOf        string                    `json:"as_of"`
	Method      []string                  `json:"method"`
	Models      []TokenPublicModelMetrics `json:"models"`
}

type TokenPublicModelMetrics struct {
	ID      string                       `json:"id"`
	Windows map[string]TokenPublicWindow `json:"windows"`
	Daily   []TokenPublicDay             `json:"daily"`
}

type TokenPublicPercentiles struct {
	P50 *float64 `json:"p50"`
	P90 *float64 `json:"p90"`
}

type TokenPublicWindow struct {
	Requests        int64                  `json:"requests"`
	SuccessRate     *float64               `json:"success_rate"`
	StreamedSamples int64                  `json:"streamed_samples"`
	TTFTMs          TokenPublicPercentiles `json:"ttft_ms"`
	ThroughputTPS   TokenPublicPercentiles `json:"throughput_tps"`
}

type TokenPublicDay struct {
	Date        string   `json:"date"`
	Requests    int64    `json:"requests"`
	SuccessRate *float64 `json:"success_rate"`
}

var tokenPublicMethod = []string{
	"Request success: share of requests answered successfully, counted once per request after any failover to another provider; errors and refused requests count against it. Health checks are not counted.",
	"Time to first token and output speed (tokens per second after the first token) are measured on streamed requests and estimated from histograms; shown only with at least 10 samples.",
	"Windows (24h, 7d, 30d) end at as_of, the end of the last complete hour (UTC). Daily figures are UTC days. Data is cached for 5 minutes.",
}

func tokenRate(success, total int64) *float64 {
	if total == 0 {
		return nil
	}
	v := float64(int64(float64(success)/float64(total)*100000+0.5)) / 100000
	return &v
}

// TokenBuildPublicMetrics aggregates the public metrics for the catalog
// models: models maps each public name a customer may send (id or alias) to
// its catalog id, and ids lists the catalog ids in order. Windows end at the
// last complete hour before now.
func TokenBuildPublicMetrics(now time.Time, ids []string, models map[string]string) (TokenPublicMetrics, error) {
	asOf := now.UTC().Truncate(time.Hour)
	end := asOf.Unix()
	start30 := end - 30*24*3600
	out := TokenPublicMetrics{GeneratedAt: now.UTC().Format(time.RFC3339), AsOf: asOf.Format(time.RFC3339), Method: tokenPublicMethod, Models: []TokenPublicModelMetrics{}}
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	slices.Sort(names)

	type countRow struct {
		ModelName string
		BucketTs  int64
		Requests  int64
		Successes int64
	}
	var counts []countRow
	if len(names) > 0 {
		if err := DB.Model(&PerfMetric{}).
			Select("model_name, bucket_ts, SUM(request_count) AS requests, SUM(success_count) AS successes").
			Where("model_name IN ? AND bucket_ts >= ? AND bucket_ts < ?", names, start30, end).
			Group("model_name, bucket_ts").Scan(&counts).Error; err != nil {
			return out, err
		}
	}
	var hists []TokenPerfHistogram
	if len(names) > 0 {
		if err := DB.Where("model_name IN ? AND bucket_ts >= ? AND bucket_ts < ?", names, start30, end).Find(&hists).Error; err != nil {
			return out, err
		}
	}

	type agg struct {
		requests, successes int64
		ttft, speed         []int64
	}
	newAgg := func() *agg {
		return &agg{ttft: make([]int64, len(tokenTTFTBinsMs)+1), speed: make([]int64, len(tokenSpeedBinsTS)+1)}
	}
	day := func(ts int64) string { return time.Unix(ts, 0).UTC().Format(time.DateOnly) }
	win := map[string]map[string]*agg{} // id -> window -> agg
	daily := map[string]map[string]*agg{}
	for _, id := range ids {
		win[id] = map[string]*agg{"24h": newAgg(), "7d": newAgg(), "30d": newAgg()}
		daily[id] = map[string]*agg{}
	}
	pick := func(name string, ts int64) []*agg {
		id, ok := models[name]
		if !ok || win[id] == nil {
			return nil
		}
		d := daily[id][day(ts)]
		if d == nil {
			d = newAgg()
			daily[id][day(ts)] = d
		}
		targets := []*agg{win[id]["30d"], d}
		if ts >= end-7*24*3600 {
			targets = append(targets, win[id]["7d"])
		}
		if ts >= end-24*3600 {
			targets = append(targets, win[id]["24h"])
		}
		return targets
	}
	for _, r := range counts {
		for _, a := range pick(r.ModelName, r.BucketTs) {
			a.requests += r.Requests
			a.successes += r.Successes
		}
	}
	for _, h := range hists {
		ttft := tokenPerfDecode(h.TTFT, len(tokenTTFTBinsMs)+1)
		speed := tokenPerfDecode(h.Speed, len(tokenSpeedBinsTS)+1)
		for _, a := range pick(h.ModelName, h.BucketTs) {
			tokenPerfAdd(a.ttft, ttft)
			tokenPerfAdd(a.speed, speed)
		}
	}

	lastDay := time.Unix(end-1, 0).UTC().Truncate(24 * time.Hour)
	for _, id := range ids {
		m := TokenPublicModelMetrics{ID: id, Windows: map[string]TokenPublicWindow{}, Daily: []TokenPublicDay{}}
		for name, a := range win[id] {
			var streamed int64
			for _, n := range a.ttft {
				streamed += n
			}
			m.Windows[name] = TokenPublicWindow{
				Requests:        a.requests,
				SuccessRate:     tokenRate(a.successes, a.requests),
				StreamedSamples: streamed,
				TTFTMs:          TokenPublicPercentiles{tokenPerfPercentile(tokenTTFTBinsMs, a.ttft, 0.5), tokenPerfPercentile(tokenTTFTBinsMs, a.ttft, 0.9)},
				ThroughputTPS:   TokenPublicPercentiles{tokenPerfPercentile(tokenSpeedBinsTS, a.speed, 0.5), tokenPerfPercentile(tokenSpeedBinsTS, a.speed, 0.9)},
			}
		}
		for i := 29; i >= 0; i-- { // the last 30 UTC days, oldest first
			entry := TokenPublicDay{Date: lastDay.AddDate(0, 0, -i).Format(time.DateOnly)}
			if a := daily[id][entry.Date]; a != nil {
				entry.Requests, entry.SuccessRate = a.requests, tokenRate(a.successes, a.requests)
			}
			m.Daily = append(m.Daily, entry)
		}
		out.Models = append(out.Models, m)
	}
	return out, nil
}

// TokenCatalogModelNames maps every public name (catalog id and aliases) of
// the models TOKEN sells to its catalog id, from the seeded catalog channels
// (token:<provider>:<id>, models "id,alias,..."). ids are the catalog ids with
// at least one enabled channel, sorted.
func TokenCatalogModelNames() ([]string, map[string]string, error) {
	var channels []Channel
	if err := DB.Select("name", "models", "status").Where("name LIKE ?", "token:%").Find(&channels).Error; err != nil {
		return nil, nil, err
	}
	names := map[string]string{}
	sold := map[string]bool{}
	for _, ch := range channels {
		parts := strings.SplitN(ch.Name, ":", 3)
		if len(parts) != 3 || parts[2] == "" {
			continue
		}
		id := parts[2]
		for name := range strings.SplitSeq(ch.Models, ",") {
			if name = strings.TrimSpace(name); name != "" {
				names[name] = id
			}
		}
		names[id] = id
		if ch.Status == common.ChannelStatusEnabled {
			sold[id] = true
		}
	}
	ids := make([]string, 0, len(sold))
	for id := range sold {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for name, id := range names {
		if !sold[id] {
			delete(names, name)
		}
	}
	return ids, names, nil
}
