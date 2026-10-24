package controller

import (
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

// TOKEN: GET /public/metrics, per catalog model: request success, time to
// first token and output speed over 24 hours, 7 and 30 days, and 30 daily
// success figures, for the website (model/token_perf_histogram.go). Only
// aggregates; computed at most every 5 minutes and cacheable for as long.
const tokenPublicMetricsTTL = 5 * time.Minute

var tokenPublicMetricsCache struct {
	sync.Mutex
	at   time.Time
	body []byte
}

func TokenPublicMetrics(c *gin.Context) {
	tokenPublicMetricsCache.Lock()
	defer tokenPublicMetricsCache.Unlock()
	if tokenPublicMetricsCache.body == nil || time.Since(tokenPublicMetricsCache.at) >= tokenPublicMetricsTTL {
		ids, names, err := model.TokenCatalogModelNames()
		var metrics model.TokenPublicMetrics
		if err == nil {
			metrics, err = model.TokenBuildPublicMetrics(time.Now(), ids, names)
		}
		var body []byte
		if err == nil {
			body, err = common.Marshal(metrics)
		}
		if err != nil {
			common.SysError("TOKEN: public metrics: " + err.Error())
			c.Header("Cache-Control", "no-store")
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "metrics unavailable"})
			return
		}
		tokenPublicMetricsCache.at, tokenPublicMetricsCache.body = time.Now(), body
	}
	c.Header("Cache-Control", "public, max-age=300")
	c.Header("Access-Control-Allow-Origin", "*")
	c.Data(http.StatusOK, "application/json; charset=utf-8", tokenPublicMetricsCache.body)
}
