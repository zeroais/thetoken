package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func signupRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	require.NoError(t, router.SetTrustedProxies(nil))
	router.GET("/verification", tokenSignupIPLimit(2, false), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	router.POST("/register", tokenSignupIPLimit(2, true), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": c.Query("ok") == "1", "message": ""})
	})
	return router
}

func signup(router http.Handler, method, path, remoteAddr string) int {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, nil)
	r.RemoteAddr = remoteAddr
	router.ServeHTTP(w, r)
	return w.Code
}

func testSignupLimit(t *testing.T) {
	router := signupRouter(t)
	ip := "203.0.113.7:4000"

	// Failed attempts (wrong code, taken username) do not count.
	for range 5 {
		assert.Equal(t, http.StatusOK, signup(router, http.MethodPost, "/register?ok=0", ip))
	}
	assert.Equal(t, http.StatusOK, signup(router, http.MethodPost, "/register?ok=1", ip))
	assert.Equal(t, http.StatusOK, signup(router, http.MethodGet, "/verification", ip))
	assert.Equal(t, http.StatusOK, signup(router, http.MethodPost, "/register?ok=1", ip))

	// At the limit: no more sign-ups and no more verification emails.
	assert.Equal(t, http.StatusTooManyRequests, signup(router, http.MethodPost, "/register?ok=1", ip))
	assert.Equal(t, http.StatusTooManyRequests, signup(router, http.MethodGet, "/verification", ip))

	// Another network is unaffected; one IPv6 /64 is one network.
	assert.Equal(t, http.StatusOK, signup(router, http.MethodPost, "/register?ok=1", "198.51.100.9:4000"))
	assert.Equal(t, http.StatusOK, signup(router, http.MethodPost, "/register?ok=1", "[2001:db8:1:2::1]:4000"))
	assert.Equal(t, http.StatusOK, signup(router, http.MethodPost, "/register?ok=1", "[2001:db8:1:2::ffff]:4000"))
	assert.Equal(t, http.StatusTooManyRequests, signup(router, http.MethodPost, "/register?ok=1", "[2001:db8:1:2:abcd::9]:4000"))
	assert.Equal(t, http.StatusOK, signup(router, http.MethodPost, "/register?ok=1", "[2001:db8:1:3::1]:4000"))
}

func TestSignupLimitCountsSuccessfulSignupsPerNetwork(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		previous := common.RedisEnabled
		common.RedisEnabled = false
		t.Cleanup(func() {
			common.RedisEnabled = previous
			tokenSignupLocal.Lock()
			clear(tokenSignupLocal.at)
			tokenSignupLocal.Unlock()
		})
		testSignupLimit(t)
	})
	t.Run("redis", func(t *testing.T) {
		server, _ := useRateLimitMiniRedis(t)
		testSignupLimit(t)
		assert.Greater(t, server.TTL("token:signup:203.0.113.7").Hours(), 23.0, "the window is 24 hours")
	})
}

func TestSignupLimitZeroTurnsItOff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/register", tokenSignupIPLimit(0, true), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	for range 5 {
		assert.Equal(t, http.StatusOK, signup(router, http.MethodPost, "/register", "203.0.113.8:1"))
	}
}

func TestSignupNetwork(t *testing.T) {
	assert.Equal(t, "203.0.113.7", tokenSignupNetwork("203.0.113.7"))
	assert.Equal(t, "203.0.113.7", tokenSignupNetwork("::ffff:203.0.113.7"))
	assert.Equal(t, "2001:db8:1:2::/64", tokenSignupNetwork("2001:db8:1:2:3:4:5:6"))
	assert.Equal(t, "garbage", tokenSignupNetwork("garbage"))
}
