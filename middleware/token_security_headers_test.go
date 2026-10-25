package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestTokenSecurityHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	saved := common.SessionCookieSecure
	t.Cleanup(func() { common.SessionCookieSecure = saved })

	serve := func() http.Header {
		r := gin.New()
		r.Use(TokenSecurityHeaders())
		r.GET("/", func(c *gin.Context) { c.Status(http.StatusOK) })
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
		return w.Header()
	}

	common.SessionCookieSecure = false
	h := serve()
	assert.Equal(t, "nosniff", h.Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", h.Get("X-Frame-Options"))
	assert.Contains(t, h.Get("Content-Security-Policy"), "script-src 'self' https://challenges.cloudflare.com;")
	assert.Contains(t, h.Get("Content-Security-Policy"), "frame-ancestors 'none'")
	assert.Empty(t, h.Get("Strict-Transport-Security"), "no HSTS on plain-HTTP local dev")

	common.SessionCookieSecure = true
	assert.Contains(t, serve().Get("Strict-Transport-Security"), "max-age=31536000")

	t.Setenv("TOKEN_CSP", "off")
	assert.Empty(t, serve().Get("Content-Security-Policy"))
}
