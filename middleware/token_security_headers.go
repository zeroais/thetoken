package middleware

import (
	"os"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/gin-gonic/gin"
)

// TOKEN fork: browser security headers (upstream sends none).
//
// The console is a same-origin SPA with no inline scripts, so scripts are
// limited to 'self', plus Cloudflare's Turnstile widget (human verification
// on sign-up and sign-in; it runs in a frame from the same origin). Styles allow inline attributes (React style props).
// Images and the About-page iframe may point to admin-configured https URLs.
// Override the policy with TOKEN_CSP (e.g. to add an analytics origin), or
// set TOKEN_CSP=off to disable it.
const tokenDefaultCSP = "default-src 'self'; " +
	"script-src 'self' https://challenges.cloudflare.com; " + // Turnstile on sign-up and sign-in
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob: https:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"frame-src 'self' https:; " +
	"frame-ancestors 'none'; " +
	"form-action 'self' https://checkout.stripe.com; " +
	"base-uri 'self'; " +
	"object-src 'none'"

// TokenSecurityHeaders sets security headers on every response.
func TokenSecurityHeaders() gin.HandlerFunc {
	csp := strings.TrimSpace(os.Getenv("TOKEN_CSP"))
	if csp == "" {
		csp = tokenDefaultCSP
	}
	if strings.EqualFold(csp, "off") {
		csp = ""
	}
	return func(c *gin.Context) {
		h := c.Writer.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		if csp != "" {
			h.Set("Content-Security-Policy", csp)
		}
		// HSTS only over HTTPS: production sets SESSION_COOKIE_SECURE=true
		// behind Cloudflare/ALB TLS termination.
		if common.SessionCookieSecure || c.Request.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}
