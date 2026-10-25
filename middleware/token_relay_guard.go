package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// TOKEN: per-account guard on API requests, after the key is checked.
//
// Concurrency: an account may have at most token_security.max_concurrent_
// requests requests in flight (0: no cap). Rate limits count starts per
// minute; long streamed replies need a cap on how many run at once, so one
// account cannot hold a large share of upstream capacity or reserve against
// its whole balance at once. The count lives in Redis (shared by instances)
// as a sorted set of request start times; an entry older than
// tokenInflightTTL is dropped, so a crashed instance cannot block an account
// for long. Without Redis each instance counts on its own.
//
// It also records the client network per key for leaked-key detection
// (service/token_key_networks.go).
const tokenInflightTTL = 30 * time.Minute

var tokenInflightScript = redis.NewScript(`
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
if redis.call('ZCARD', KEYS[1]) >= tonumber(ARGV[3]) then return 0 end
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[4])
redis.call('EXPIRE', KEYS[1], ARGV[5])
return 1`)

var tokenInflightLocal = struct {
	sync.Mutex
	count map[int]int
}{count: map[int]int{}}

func TokenRelayGuard() gin.HandlerFunc {
	return func(c *gin.Context) {
		service.TokenNoteKeyNetwork(c.Request.Context(), c.GetInt("token_id"), tokenSignupNetwork(c.ClientIP()))

		limit := operation_setting.GetTokenSecuritySetting().MaxConcurrentRequests
		userID := c.GetInt("id")
		if limit <= 0 || userID == 0 {
			c.Next()
			return
		}
		release, ok := tokenAcquireInflight(c.Request.Context(), userID, limit)
		if !ok {
			abortWithOpenAiMessage(c, http.StatusTooManyRequests,
				fmt.Sprintf("Too many requests in progress for this account: at most %d at a time. Wait for one to finish, then try again.", limit),
				types.ErrorCode("concurrency_limit_exceeded"))
			return
		}
		defer release()
		c.Next()
	}
}

// tokenAcquireInflight admits one more in-flight request for userID if it
// has fewer than limit, and returns the function that ends it.
func tokenAcquireInflight(ctx context.Context, userID, limit int) (func(), bool) {
	if common.RedisEnabled && common.RDB != nil {
		key := "token:inflight:" + strconv.Itoa(userID)
		member := common.GetUUID()
		now := time.Now()
		admitted, err := tokenInflightScript.Run(ctx, common.RDB, []string{key},
			now.Add(-tokenInflightTTL).UnixMilli(), now.UnixMilli(), limit, member, int(tokenInflightTTL.Seconds())).Int()
		if err == nil {
			if admitted == 0 {
				return nil, false
			}
			return func() { common.RDB.ZRem(context.Background(), key, member) }, true
		}
		common.SysError("TOKEN: concurrency guard: Redis unavailable, counting on this instance: " + err.Error())
	}
	tokenInflightLocal.Lock()
	defer tokenInflightLocal.Unlock()
	if tokenInflightLocal.count[userID] >= limit {
		return nil, false
	}
	tokenInflightLocal.count[userID]++
	return func() {
		tokenInflightLocal.Lock()
		defer tokenInflightLocal.Unlock()
		if tokenInflightLocal.count[userID]--; tokenInflightLocal.count[userID] <= 0 {
			delete(tokenInflightLocal.count, userID)
		}
	}, true
}

// tokenRateLimitMessage is the English text of the per-account request limit
// (model-rate-limit.go; upstream's is Chinese). total: the limit counts
// failed requests too.
func tokenRateLimitMessage(limit int, total bool) string {
	what := "successful requests"
	if total {
		what = "requests (failed ones included)"
	}
	return fmt.Sprintf("Rate limit reached: at most %d %s per %d minute(s) for this account. Wait a moment, then try again.",
		limit, what, setting.ModelRequestRateLimitDurationMinutes)
}
