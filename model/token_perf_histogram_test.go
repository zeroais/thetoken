package model

import (
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenPerfPercentileFromBins(t *testing.T) {
	bins := make([]int64, len(tokenTTFTBinsMs)+1)
	bins[3] = 10 // ten samples in (300, 400] ms
	p50 := tokenPerfPercentile(tokenTTFTBinsMs, bins, 0.5)
	require.NotNil(t, p50)
	assert.InDelta(t, 350, *p50, 0.01, "interpolated inside the bin")

	bins = make([]int64, len(tokenTTFTBinsMs)+1)
	bins[0], bins[len(bins)-1] = 5, 5 // half fast, half slower than the last bound
	assert.InDelta(t, 100, *tokenPerfPercentile(tokenTTFTBinsMs, bins, 0.5), 0.01)
	assert.InDelta(t, 60000, *tokenPerfPercentile(tokenTTFTBinsMs, bins, 0.9), 0.01, "the open last bin reports its lower bound")

	bins = make([]int64, len(tokenTTFTBinsMs)+1)
	bins[2] = tokenPerfMinSamples - 1
	assert.Nil(t, tokenPerfPercentile(tokenTTFTBinsMs, bins, 0.5), "too few samples")
}

func TestTokenPublicMetricsPerCatalogModel(t *testing.T) {
	tokenDialects(t, []any{&Channel{}, &PerfMetric{}, &TokenPerfHistogram{}}, func(t *testing.T) {
		tokenPerfHot.Lock()
		clear(tokenPerfHot.buckets)
		tokenPerfHot.Unlock()
		now := time.Date(2026, 10, 24, 10, 30, 0, 0, time.UTC)
		recent := now.Truncate(time.Hour).Add(-2 * time.Hour).Unix() // inside 24 h
		older := now.Truncate(time.Hour).Add(-72 * time.Hour).Unix() // inside 30 days only

		require.NoError(t, DB.Create(&[]Channel{
			{Name: "token:provider-a:moonshotai/kimi-k3", Models: "moonshotai/kimi-k3,kimi-k3", Key: "a", Status: common.ChannelStatusEnabled},
			{Name: "token:provider-b:moonshotai/kimi-k3", Models: "moonshotai/kimi-k3,kimi-k3", Key: "b", Status: common.ChannelStatusEnabled},
			{Name: "token:provider-b:retired/model", Models: "retired/model", Key: "c", Status: common.ChannelStatusManuallyDisabled},
			{Name: "someone else's channel", Models: "other", Key: "d", Status: common.ChannelStatusEnabled},
		}).Error)
		require.NoError(t, DB.Create(&[]PerfMetric{
			{ModelName: "moonshotai/kimi-k3", Group: "default", BucketTs: recent, RequestCount: 90, SuccessCount: 88},
			{ModelName: "kimi-k3", Group: "us-eu-hosted", BucketTs: recent, RequestCount: 10, SuccessCount: 10},
			{ModelName: "moonshotai/kimi-k3", Group: "default", BucketTs: older, RequestCount: 100, SuccessCount: 90},
			{ModelName: "retired/model", Group: "default", BucketTs: recent, RequestCount: 5, SuccessCount: 5},
		}).Error)

		for range 12 { // streamed: 350 ms to first token, 1000 tokens in 20 s = 50 tokens/s
			TokenRecordPerfSample("kimi-k3", recent, true, 350, 1000, 20000)
		}
		TokenRecordPerfSample("moonshotai/kimi-k3", recent, false, 0, 500, 9000) // not streamed: not timed
		TokenFlushPerfHistograms(recent + 3600)
		for range 8 { // a second instance adds to the same hour
			TokenRecordPerfSample("moonshotai/kimi-k3", recent, true, 350, 1000, 20000)
		}
		TokenFlushPerfHistograms(recent + 3600)

		ids, names, err := TokenCatalogModelNames()
		require.NoError(t, err)
		assert.Equal(t, []string{"moonshotai/kimi-k3"}, ids, "only models with an enabled catalog channel")
		assert.Equal(t, map[string]string{"moonshotai/kimi-k3": "moonshotai/kimi-k3", "kimi-k3": "moonshotai/kimi-k3"}, names)

		got, err := TokenBuildPublicMetrics(now, ids, names)
		require.NoError(t, err)
		assert.Equal(t, "2026-10-24T10:00:00Z", got.AsOf)
		require.Len(t, got.Models, 1)
		m := got.Models[0]
		day := m.Windows["24h"]
		assert.EqualValues(t, 100, day.Requests, "alias traffic and every group count")
		require.NotNil(t, day.SuccessRate)
		assert.InDelta(t, 0.98, *day.SuccessRate, 1e-9)
		assert.EqualValues(t, 20, day.StreamedSamples)
		require.NotNil(t, day.TTFTMs.P50)
		assert.InDelta(t, 350, *day.TTFTMs.P50, 0.01)
		require.NotNil(t, day.ThroughputTPS.P50)
		assert.True(t, *day.ThroughputTPS.P50 > 40 && *day.ThroughputTPS.P50 <= 50, *day.ThroughputTPS.P50)

		week := m.Windows["7d"]
		assert.EqualValues(t, 200, week.Requests, "72 hours ago is inside 7 days")
		month := m.Windows["30d"]
		assert.EqualValues(t, 200, month.Requests)
		assert.InDelta(t, 0.94, *month.SuccessRate, 1e-9)

		require.Len(t, m.Daily, 30)
		assert.Equal(t, "2026-10-24", m.Daily[29].Date)
		byDate := map[string]TokenPublicDay{}
		for _, d := range m.Daily {
			byDate[d.Date] = d
		}
		assert.EqualValues(t, 100, byDate["2026-10-24"].Requests)
		assert.EqualValues(t, 100, byDate["2026-10-21"].Requests)
		assert.Nil(t, byDate["2026-10-01"].SuccessRate, "no traffic is no figure, not 0%")
	})
}
