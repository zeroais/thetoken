package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

// counterValue and histogramCount read metrics without the testutil module.
func counterValue(c prometheus.Counter) float64 {
	var m dto.Metric
	_ = c.Write(&m)
	return m.GetCounter().GetValue()
}

func histogramCount(o prometheus.Observer) uint64 {
	var m dto.Metric
	_ = o.(prometheus.Histogram).Write(&m)
	return m.GetHistogram().GetSampleCount()
}

func TestMiddlewareLabelsRoutedAndUnroutedRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware)
	r.POST("/v1/chat/completions", func(c *gin.Context) {
		if c.Query("routed") == "1" {
			common.SetContextKey(c, constant.ContextKeyChannelId, 7)
			common.SetContextKey(c, constant.ContextKeyOriginalModel, "zai-org/glm-5.3")
		}
		c.String(http.StatusOK, "ok")
	})
	r.POST("/api/stripe/webhook", func(c *gin.Context) { c.Status(http.StatusInternalServerError) })

	for _, url := range []string{"/v1/chat/completions?routed=1", "/v1/chat/completions"} {
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, url, nil))
	}
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/stripe/webhook", nil))

	assert.Equal(t, 1.0, counterValue(requests.WithLabelValues("zai-org/glm-5.3", "2xx", "false")))
	assert.Equal(t, 1.0, counterValue(requests.WithLabelValues("unrouted", "2xx", "false")), "client-chosen names never become labels")
	assert.Equal(t, 1.0, counterValue(stripeWebhooks.WithLabelValues("500")))
}

func TestObserveConsume(t *testing.T) {
	other := model.NewLogOther()
	other.SetPublic("frt", float64(400))
	observeConsume(model.RecordConsumeLogParams{ModelName: "m", PromptTokens: 1000, CompletionTokens: 360,
		Quota: 500000, UseTimeSeconds: 4, IsStream: true, Other: other})

	assert.Equal(t, 1000.0, counterValue(tokens.WithLabelValues("m", "prompt")))
	assert.Equal(t, 360.0, counterValue(tokens.WithLabelValues("m", "completion")))
	assert.Equal(t, 1.0, counterValue(revenue.WithLabelValues("m")), "500000 quota = $1")
	assert.Equal(t, uint64(1), histogramCount(ttft.WithLabelValues("m")))
	assert.Equal(t, uint64(1), histogramCount(outputSpeed.WithLabelValues("m"))) // 360 tokens / 3.6 s = 100 tok/s
}
