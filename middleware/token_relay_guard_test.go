package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TOKEN: an account has at most max_concurrent_requests requests in flight,
// across instances (Redis) or per instance without Redis; a finished request
// frees its place. Every request records its key's client network.
func TestRelayGuardCapsConcurrentRequestsPerAccount(t *testing.T) {
	gin.SetMode(gin.TestMode)
	security := operation_setting.GetTokenSecuritySetting()
	saved := security.MaxConcurrentRequests
	security.MaxConcurrentRequests = 2
	t.Cleanup(func() { security.MaxConcurrentRequests = saved })

	for _, withRedis := range []bool{true, false} {
		t.Run(map[bool]string{true: "redis", false: "per instance"}[withRedis], func(t *testing.T) {
			if withRedis {
				useRateLimitMiniRedis(t)
			} else {
				previous := common.RedisEnabled
				common.RedisEnabled = false
				t.Cleanup(func() { common.RedisEnabled = previous })
			}
			release := make(chan struct{})
			started := make(chan struct{}, 4)
			router := gin.New()
			router.Use(func(c *gin.Context) {
				c.Set("id", 7)
				c.Set("token_id", 70)
				c.Next()
			}, TokenRelayGuard())
			router.POST("/v1/chat/completions", func(c *gin.Context) {
				started <- struct{}{}
				<-release
				c.Status(http.StatusOK)
			})
			send := func() *httptest.ResponseRecorder {
				recorder := httptest.NewRecorder()
				request := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				request.RemoteAddr = "203.0.113.9:4000"
				router.ServeHTTP(recorder, request)
				return recorder
			}

			var wg sync.WaitGroup
			results := make([]*httptest.ResponseRecorder, 2)
			for i := range 2 {
				wg.Go(func() { results[i] = send() })
			}
			<-started
			<-started
			third := send()
			assert.Equal(t, http.StatusTooManyRequests, third.Code, "a third request while two run")
			assert.Contains(t, third.Body.String(), "at most 2 at a time")

			close(release)
			wg.Wait()
			for _, r := range results {
				assert.Equal(t, http.StatusOK, r.Code)
			}
			release = make(chan struct{})
			close(release)
			assert.Equal(t, http.StatusOK, send().Code, "places are freed when requests finish")
			if withRedis {
				assert.Equal(t, int64(1), service.TokenKeyNetworkCount(t.Context(), 70, time.Now()))
			}
		})
	}
}

// TOKEN: the per-account request limit answers in English.
func TestRateLimitMessageIsEnglish(t *testing.T) {
	assert.Contains(t, tokenRateLimitMessage(60, true), "Rate limit reached: at most 60 requests (failed ones included) per 1 minute(s)")
	require.NotContains(t, tokenRateLimitMessage(60, false), "您")
}
