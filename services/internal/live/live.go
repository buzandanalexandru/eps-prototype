// Package live builds and streams the live on-sale dashboard (step 6, E8, R2): the worker's
// aggregator turns each watched sale's exact Redis and Postgres values into one frame a second
// (design D-6), and the gateway streams the frames to the organiser's browser as server-sent
// events (D-7).
package live

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// HistoryLen is how many compact points a sale keeps (15 minutes at one a second).
const HistoryLen = 900

// MaxWatched bounds the sales framed each second, MaxWatchedPerOrg one organiser's share of them,
// WatchedScan how many watched sales a tick considers (review D-g: slots shared fairly).
const (
	MaxWatched       = 20
	MaxWatchedPerOrg = 5
	WatchedScan      = 200
)

func watchKey() string {
	k, _ := redisx.PlatformKey("live-watch")
	return k
}

func historyKey(sale string) string {
	k, _ := redisx.PlatformKey("live-hist", sale)
	return k
}

func lastKey(sale string) string {
	k, _ := redisx.PlatformKey("live-last", sale)
	return k
}

func reportKey(sale string) string {
	k, _ := redisx.PlatformKey("live-sim", sale)
	return k
}

// Channel is the pub/sub channel of a sale's frames.
func Channel(sale string) string { return "eps:live:" + sale }

// Watch keeps a sale framed for ttl (a dashboard stream open, a run active).
func Watch(ctx context.Context, rdb *redis.Client, sale string, ttl time.Duration) error {
	if sale == "" {
		return nil
	}
	until := time.Now().Add(ttl).UnixMilli()
	// GT: never shortens a longer watch.
	return rdb.ZAddArgs(ctx, watchKey(), redis.ZAddArgs{GT: true, Members: []redis.Z{{Score: float64(until), Member: sale}}}).Err()
}

// Watched lists the sales to frame now (expired watches dropped), at most limit.
func Watched(ctx context.Context, rdb *redis.Client, now time.Time, limit int) ([]string, error) {
	if err := rdb.ZRemRangeByScore(ctx, watchKey(), "-inf", strconv.FormatInt(now.UnixMilli(), 10)).Err(); err != nil {
		return nil, err
	}
	return rdb.ZRevRange(ctx, watchKey(), 0, int64(limit-1)).Result()
}

// Unwatch drops a sale (deleted or unknown).
func Unwatch(ctx context.Context, rdb *redis.Client, sale string) {
	_ = rdb.ZRem(ctx, watchKey(), sale).Err()
}

// StoreReport keeps the active run's latest report for the aggregator (counts only).
func StoreReport(ctx context.Context, rdb *redis.Client, sale string, report []byte) error {
	return rdb.Set(ctx, reportKey(sale), report, time.Hour).Err()
}

// LoadReport reads it (nil when none).
func LoadReport(ctx context.Context, rdb *redis.Client, sale string) json.RawMessage {
	b, err := rdb.Get(ctx, reportKey(sale)).Bytes()
	if err != nil {
		return nil
	}
	return b
}

// Point is one second of the sparklines.
type Point struct {
	TS        int64   `json:"t"`
	Waiting   int64   `json:"q"`
	Admitted  float64 `json:"a"` // per minute
	Sold      float64 `json:"s"` // per minute
	Failovers int64   `json:"f"` // total
	// Totals (rates are worked out from them when the history is read back).
	AdmittedTotal int64 `json:"at"`
	SoldTotal     int64 `json:"st"`
}

// Publish stores a frame as the latest, appends its point and sends it to every gateway.
func Publish(ctx context.Context, rdb *redis.Client, sale string, frame []byte, p Point) error {
	pb, _ := json.Marshal(p)
	pipe := rdb.TxPipeline()
	pipe.Set(ctx, lastKey(sale), frame, time.Hour)
	pipe.LPush(ctx, historyKey(sale), pb)
	pipe.LTrim(ctx, historyKey(sale), 0, HistoryLen-1)
	pipe.Expire(ctx, historyKey(sale), time.Hour)
	pipe.Publish(ctx, Channel(sale), frame)
	_, err := pipe.Exec(ctx)
	return err
}

// Latest is a sale's latest frame (nil when none).
func Latest(ctx context.Context, rdb *redis.Client, sale string) []byte {
	b, err := rdb.Get(ctx, lastKey(sale)).Bytes()
	if err != nil {
		return nil
	}
	return b
}

// History returns up to n points, oldest first.
func History(ctx context.Context, rdb *redis.Client, sale string, n int) ([]Point, error) {
	raw, err := rdb.LRange(ctx, historyKey(sale), 0, int64(n-1)).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	out := make([]Point, 0, len(raw))
	for i := len(raw) - 1; i >= 0; i-- {
		var p Point
		if json.Unmarshal([]byte(raw[i]), &p) == nil {
			out = append(out, p)
		}
	}
	return out, nil
}
