// Package metrics exposes TOKEN's Prometheus metrics on a separate,
// internal-only listener (TOKEN_METRICS_ADDR, e.g. ":9090"); never on the
// public API port. It observes the relay through a gin middleware and two
// upstream hooks (model.OnConsumeLog, service.OnChannelError), and probes
// database, Redis and channel health on every scrape.
package metrics

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	registry = prometheus.NewRegistry()

	requests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "token_relay_requests_total",
		Help: "Relay (API) requests by public model, final status class and stream mode.",
	}, []string{"model", "status", "stream"})

	duration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "token_relay_request_duration_seconds",
		Help:    "Total relay request duration as seen by the client.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2, 4, 8, 15, 30, 60, 120, 300},
	}, []string{"model"})

	overhead = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "token_relay_first_byte_seconds",
		Help:    "Time from request arrival to the first byte sent to the client (TTFB, stream or not).",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8, 15, 30},
	}, []string{"model", "stream"})

	ttft = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "token_ttft_seconds",
		Help:    "Time to first token reported by the relay (upstream first response).",
		Buckets: []float64{0.1, 0.2, 0.3, 0.5, 0.75, 1, 1.5, 2, 3, 5, 8, 15},
	}, []string{"model"})

	tokens = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "token_tokens_total",
		Help: "Billed tokens by model and kind (prompt, completion).",
	}, []string{"model", "kind"})

	outputSpeed = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "token_output_tokens_per_second",
		Help:    "Completion tokens per second after the first token, per request.",
		Buckets: []float64{5, 10, 20, 40, 60, 80, 120, 200, 400, 800},
	}, []string{"model"})

	revenue = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "token_revenue_usd_total",
		Help: "Customer charges in USD by model (from billed quota).",
	}, []string{"model"})

	channelErrors = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "token_channel_errors_total",
		Help: "Upstream channel errors by channel (provider/model) and HTTP status, including retried ones.",
	}, []string{"provider", "model", "status"})

	stripeWebhooks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "token_stripe_webhook_responses_total",
		Help: "Stripe webhook responses by status code.",
	}, []string{"status"})
)

// Install wires the hooks and returns the relay middleware. If addr is
// non-empty it also starts the internal metrics listener.
func Install(addr string) gin.HandlerFunc {
	registry.MustRegister(requests, duration, overhead, ttft, tokens, outputSpeed, revenue, channelErrors, stripeWebhooks,
		collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}), healthCollector{})

	model.OnConsumeLog = observeConsume
	service.OnChannelError = observeChannelError

	if addr != "" {
		mux := http.NewServeMux()
		mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
		go func() {
			srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
			if err := srv.ListenAndServe(); err != nil {
				common.SysError("metrics listener stopped: " + err.Error())
			}
		}()
		common.SysLog("TOKEN metrics listening on " + addr + "/metrics (internal only)")
	}
	return middleware
}

// firstByteWriter records when the first body byte is written.
type firstByteWriter struct {
	gin.ResponseWriter
	first time.Time
}

func (w *firstByteWriter) Write(b []byte) (int, error) {
	if w.first.IsZero() {
		w.first = time.Now()
	}
	return w.ResponseWriter.Write(b)
}

func (w *firstByteWriter) WriteString(s string) (int, error) {
	if w.first.IsZero() {
		w.first = time.Now()
	}
	return w.ResponseWriter.WriteString(s)
}

func middleware(c *gin.Context) {
	path := c.Request.URL.Path
	if path == "/api/stripe/webhook" {
		c.Next()
		stripeWebhooks.WithLabelValues(strconv.Itoa(c.Writer.Status())).Inc()
		return
	}
	if !strings.HasPrefix(path, "/v1") && !strings.HasPrefix(path, "/v1beta") {
		c.Next()
		return
	}
	start := time.Now()
	w := &firstByteWriter{ResponseWriter: c.Writer}
	c.Writer = w
	c.Next()

	name := "unrouted"
	if common.GetContextKeyInt(c, constant.ContextKeyChannelId) > 0 {
		// Only routed requests carry a catalog model name; this bounds the
		// label set to models TOKEN actually sells.
		name = common.GetContextKeyString(c, constant.ContextKeyOriginalModel)
	}
	stream := strconv.FormatBool(strings.Contains(c.Writer.Header().Get("Content-Type"), "event-stream"))
	requests.WithLabelValues(name, strconv.Itoa(c.Writer.Status()/100)+"xx", stream).Inc()
	duration.WithLabelValues(name).Observe(time.Since(start).Seconds())
	if !w.first.IsZero() {
		overhead.WithLabelValues(name, stream).Observe(w.first.Sub(start).Seconds())
	}
}

func observeConsume(p model.RecordConsumeLogParams) {
	name := p.ModelName
	tokens.WithLabelValues(name, "prompt").Add(float64(max(p.PromptTokens, 0)))
	tokens.WithLabelValues(name, "completion").Add(float64(max(p.CompletionTokens, 0)))
	revenue.WithLabelValues(name).Add(float64(max(p.Quota, 0)) / common.QuotaPerUnit)
	frtMs := 0.0
	if p.Other != nil {
		if v, ok := p.Other.Snapshot()["frt"].(float64); ok && v > 0 {
			frtMs = v
			ttft.WithLabelValues(name).Observe(v / 1000)
		}
	}
	gen := float64(p.UseTimeSeconds) - frtMs/1000
	if p.IsStream && p.CompletionTokens > 0 && gen > 0.2 {
		outputSpeed.WithLabelValues(name).Observe(float64(p.CompletionTokens) / gen)
	}
}

func observeChannelError(channelID, status int) {
	provider, name := channelLabels(channelID)
	channelErrors.WithLabelValues(provider, name, strconv.Itoa(status)).Inc()
}

// channelLabels turns a TOKEN channel ("token:<provider>:<model>") into
// bounded labels; other channels are grouped as "other".
func channelLabels(channelID int) (provider, modelName string) {
	ch, err := model.CacheGetChannel(channelID)
	if err != nil || ch == nil {
		return "other", "other"
	}
	if rest, ok := strings.CutPrefix(ch.Name, "token:"); ok {
		if p, m, ok := strings.Cut(rest, ":"); ok {
			return p, m
		}
	}
	return "other", "other"
}

// healthCollector probes dependencies and channel state at scrape time.
type healthCollector struct{}

var (
	dbUp           = prometheus.NewDesc("token_db_up", "1 if the primary database answers a ping.", nil, nil)
	redisUp        = prometheus.NewDesc("token_redis_up", "1 if Redis answers a ping (absent when Redis is not configured).", nil, nil)
	modelChannels  = prometheus.NewDesc("token_model_enabled_channels", "Enabled TOKEN channels per public model.", []string{"model"}, nil)
	channelBalance = prometheus.NewDesc("token_provider_balance_usd", "Last fetched upstream prepaid balance per provider (New API channel balance).", []string{"provider"}, nil)
)

func (healthCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- dbUp
	ch <- redisUp
	ch <- modelChannels
	ch <- channelBalance
}

func (healthCollector) Collect(ch chan<- prometheus.Metric) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	up := 0.0
	if sqlDB, err := model.DB.DB(); err == nil && sqlDB.PingContext(ctx) == nil {
		up = 1
	}
	ch <- prometheus.MustNewConstMetric(dbUp, prometheus.GaugeValue, up)

	if common.RedisEnabled && common.RDB != nil {
		r := 0.0
		if common.RDB.Ping(ctx).Err() == nil {
			r = 1
		}
		ch <- prometheus.MustNewConstMetric(redisUp, prometheus.GaugeValue, r)
	}

	var channels []model.Channel
	if err := model.DB.WithContext(ctx).Select("id", "name", "status", "balance").
		Where("name LIKE ?", "token:%").Find(&channels).Error; err != nil {
		return
	}
	enabled := map[string]float64{}
	balances := map[string]float64{}
	for _, c := range channels {
		rest := strings.TrimPrefix(c.Name, "token:")
		provider, name, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		if _, seen := enabled[name]; !seen {
			enabled[name] = 0
		}
		if c.Status == common.ChannelStatusEnabled {
			enabled[name]++
		}
		if c.Balance > 0 {
			balances[provider] = max(balances[provider], c.Balance)
		}
	}
	for name, n := range enabled {
		ch <- prometheus.MustNewConstMetric(modelChannels, prometheus.GaugeValue, n, name)
	}
	for provider, b := range balances {
		ch <- prometheus.MustNewConstMetric(channelBalance, prometheus.GaugeValue, b, provider)
	}
}
