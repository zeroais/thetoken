package service

import (
	"context"
	"fmt"
	"time"

	"github.com/QuantumNous/new-api/common"
)

// TOKEN: leaked-key detection. Each API request adds its client network (an
// IPv4 address or an IPv6 /64) to a Redis HyperLogLog per key and UTC hour,
// so the alert job (token_alerts.go) can see a key suddenly used from many
// networks, the usual sign of a key published or shared. Only a cardinality
// sketch is kept, never the addresses, and it expires after two hours.
// Without Redis nothing is recorded and the check stays silent.
const tokenKeyNetworksTTL = 2 * time.Hour

func tokenKeyNetworksKey(tokenID int, hour time.Time) string {
	return fmt.Sprintf("token:keynets:%d:%s", tokenID, hour.UTC().Format("2006010215"))
}

// TokenNoteKeyNetwork records that API key tokenID was used from network.
func TokenNoteKeyNetwork(ctx context.Context, tokenID int, network string) {
	if !common.RedisEnabled || common.RDB == nil || tokenID <= 0 || network == "" {
		return
	}
	key := tokenKeyNetworksKey(tokenID, time.Now())
	pipe := common.RDB.Pipeline()
	pipe.PFAdd(ctx, key, network)
	pipe.Expire(ctx, key, tokenKeyNetworksTTL)
	_, _ = pipe.Exec(ctx)
}

// TokenKeyNetworkCount estimates how many networks used key tokenID in the
// UTC hour containing hour.
func TokenKeyNetworkCount(ctx context.Context, tokenID int, hour time.Time) int64 {
	if !common.RedisEnabled || common.RDB == nil {
		return 0
	}
	n, err := common.RDB.PFCount(ctx, tokenKeyNetworksKey(tokenID, hour)).Result()
	if err != nil {
		return 0
	}
	return n
}
