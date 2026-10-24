package middleware

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// TOKEN: at most TOKEN_SIGNUP_IP_DAILY_LIMIT accounts (default 3, 0 turns the
// limit off) are created from one client network in 24 hours: an IPv4
// address, or an IPv6 /64 so rotating addresses inside one allocation does not
// help. Only successful sign-ups count. The verification-email endpoint is
// refused too once a network is at its limit, so nobody gets a code they
// cannot use. Counts live in Redis (shared by all instances) and fall back to
// this instance's memory when Redis is unavailable.

const tokenSignupWindow = 24 * time.Hour

const tokenSignupLimitMessage = "Too many accounts were created from your network today. Please try again tomorrow, or contact support@thetoken.dev."

// TokenSignupIPLimit guards sign-up: countSuccess on the register route, off
// on routes that only check the limit (sending the verification email).
func TokenSignupIPLimit(countSuccess bool) gin.HandlerFunc {
	return tokenSignupIPLimit(common.GetEnvOrDefault("TOKEN_SIGNUP_IP_DAILY_LIMIT", 3), countSuccess)
}

func tokenSignupIPLimit(limit int, countSuccess bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		if limit <= 0 {
			c.Next()
			return
		}
		key := "token:signup:" + tokenSignupNetwork(c.ClientIP())
		ctx := c.Request.Context()
		if tokenSignupCount(ctx, key) >= limit {
			c.Header("Retry-After", "86400")
			c.JSON(http.StatusTooManyRequests, gin.H{"success": false, "message": tokenSignupLimitMessage})
			c.Abort()
			return
		}
		if !countSuccess {
			c.Next()
			return
		}
		w := &tokenSignupWriter{ResponseWriter: c.Writer}
		c.Writer = w
		c.Next()
		if bytes.Contains(w.head, []byte(`"success":true`)) {
			tokenSignupRecord(ctx, key)
		}
	}
}

// tokenSignupNetwork is the IPv4 address or the IPv6 /64 of ip.
func tokenSignupNetwork(ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return ip
	}
	if v4 := parsed.To4(); v4 != nil {
		return v4.String()
	}
	return parsed.Mask(net.CIDRMask(64, 128)).String() + "/64"
}

// tokenSignupWriter keeps the start of the response to see whether the
// sign-up succeeded (New API answers 200 with "success" true or false).
type tokenSignupWriter struct {
	gin.ResponseWriter
	head []byte
}

func (w *tokenSignupWriter) keep(b []byte) {
	if room := 4096 - len(w.head); room > 0 {
		w.head = append(w.head, b[:min(room, len(b))]...)
	}
}

func (w *tokenSignupWriter) Write(b []byte) (int, error) {
	w.keep(b)
	return w.ResponseWriter.Write(b)
}

func (w *tokenSignupWriter) WriteString(s string) (int, error) {
	w.keep([]byte(s))
	return w.ResponseWriter.WriteString(s)
}

var tokenSignupLocal = struct {
	sync.Mutex
	at map[string][]time.Time
}{at: map[string][]time.Time{}}

func tokenSignupLocalCount(key string, record bool) int {
	tokenSignupLocal.Lock()
	defer tokenSignupLocal.Unlock()
	now := time.Now()
	kept := tokenSignupLocal.at[key][:0]
	for _, t := range tokenSignupLocal.at[key] {
		if now.Sub(t) < tokenSignupWindow {
			kept = append(kept, t)
		}
	}
	if record {
		kept = append(kept, now)
	}
	if len(kept) == 0 {
		delete(tokenSignupLocal.at, key)
	} else {
		tokenSignupLocal.at[key] = kept
	}
	return len(kept)
}

func tokenSignupCount(ctx context.Context, key string) int {
	if common.RedisEnabled && common.RDB != nil {
		n, err := common.RDB.Get(ctx, key).Int()
		if err == nil {
			return n
		}
		if errors.Is(err, redis.Nil) {
			return 0
		}
	}
	return tokenSignupLocalCount(key, false)
}

func tokenSignupRecord(ctx context.Context, key string) {
	if common.RedisEnabled && common.RDB != nil {
		n, err := common.RDB.Incr(ctx, key).Result()
		if err == nil {
			if n == 1 {
				common.RDB.Expire(ctx, key, tokenSignupWindow)
			}
			return
		}
	}
	tokenSignupLocalCount(key, true)
}
