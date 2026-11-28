package inventory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
)

var (
	shardSwept   = metrics.NewCounter("eps_inventory_shard_entries_settled_total", "Shard entries settled by the shard sweeper (lapsed or left behind).")
	sweepBatches = metrics.NewCounter("eps_inventory_shard_sweeps_total", "Shard sweeper rounds.")
)

// SweepShards settles shard entries whose own deadline passed (D-9 expiry), so stock from a lapsed
// hold comes back within a second or two even when the core that took it stopped:
//   - the hold's record says committed: the entry is committed (a commit cut short);
//   - released, returned or no record: the entry is released;
//   - still reserving after its deadline (a core stopped mid-reserve): the record is lapsed and the
//     entry released;
//   - held with a later deadline (an extension): the entry follows the record;
//   - held past its deadline: left to the order sweeper, which ends the order in Postgres first.
//
// Work per call is bounded: at most `limit` entries per shard.
func (s *Sharded) SweepShards(ctx context.Context, org, event string, limit int64) (int, error) {
	k, err := s.keysFor(org, event)
	if err != nil {
		return 0, err
	}
	n, err := s.Redis.Get(ctx, k.shards()).Int()
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, err
	}
	settled := 0
	for i := 0; i < n; i++ {
		c, err := s.sweepShard(ctx, k, i, limit)
		if err != nil {
			return settled, err
		}
		settled += c
	}
	shardSwept.Add(int64(settled))
	return settled, nil
}

// ShardSweeper runs SweepShards over every event with holds, every interval.
type ShardSweeper struct {
	Inv *Sharded
	Log *slog.Logger
}

// RunOnce sweeps every event once.
func (w *ShardSweeper) RunOnce(ctx context.Context) int {
	evs, err := w.Inv.Events(ctx)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range evs {
		c, err := w.Inv.SweepShards(ctx, e[0], e[1], 500)
		if err != nil && w.Log != nil && ctx.Err() == nil {
			w.Log.Warn("shard sweep failed", "event", e[1], "error", err.Error())
		}
		n += c
	}
	sweepBatches.Inc()
	return n
}

// Run sweeps until ctx ends.
func (w *ShardSweeper) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.RunOnce(ctx)
		}
	}
}

// sweepShard settles one shard's due entries (at most limit).
func (s *Sharded) sweepShard(ctx context.Context, k keys, i int, limit int64) (int, error) {
	now := s.now().UnixMilli()
	ids, err := s.Redis.ZRangeArgs(ctx, redis.ZRangeArgs{Key: k.shardExp(i), Start: "-inf", Stop: strconv.FormatInt(now, 10), ByScore: true, Count: limit}).Result()
	if err != nil || len(ids) == 0 {
		return 0, err
	}
	pipe := s.Redis.Pipeline()
	cmds := make([]*redis.SliceCmd, len(ids))
	for j, id := range ids {
		cmds[j] = pipe.HMGet(ctx, k.hold(id), "state", "deadline")
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return 0, err
	}
	var commit, release, stale []string
	later := []redis.Z{}
	for j, id := range ids {
		v := cmds[j].Val()
		st := ""
		if v[0] != nil {
			st = fmt.Sprint(v[0])
		}
		dl := num(v[1])
		switch st {
		case "committed":
			commit = append(commit, id)
		case "", "released", "returned":
			release = append(release, id)
		case "reserving":
			if dl <= now {
				if _, err := masterRelease.Run(ctx, s.Redis, []string{k.homeExp(homeOf(id)), k.hold(id)}, id).StringSlice(); err == nil {
					release = append(release, id)
				}
			}
		case "held":
			switch {
			case dl > now:
				later = append(later, redis.Z{Score: float64(dl), Member: id})
			case dl+orphanAfter.Milliseconds() <= now && s.LiveOrders != nil:
				stale = append(stale, id)
			default:
				// The order sweeper's turn (Postgres first); look again in 5 s.
				later = append(later, redis.Z{Score: float64(now + 5000), Member: id})
			}
		}
	}
	// A held record well past its deadline whose order is not live (review corr L3): no order
	// will ever release it, so it lapses here.
	if len(stale) > 0 {
		live, err := s.LiveOrders(ctx, k.org, k.event, stale)
		if err == nil {
			for _, id := range stale {
				if live[id] {
					later = append(later, redis.Z{Score: float64(now + 5000), Member: id})
					continue
				}
				if _, err := masterRelease.Run(ctx, s.Redis, []string{k.homeExp(homeOf(id)), k.hold(id)}, id).StringSlice(); err == nil {
					release = append(release, id)
				}
			}
		}
	}
	settled := 0
	for mode, hs := range map[string][]string{"commit": commit, "release": release} {
		if len(hs) == 0 {
			continue
		}
		keys := []string{k.shardExp(i)}
		args := []any{mode, k.ctrPrefix(i)}
		for _, id := range hs {
			keys = append(keys, k.entry(i, id))
			args = append(args, id)
		}
		c, err := shardSettle.Run(ctx, s.Redis, keys, args...).Int()
		if err != nil {
			return settled, err
		}
		settled += c
	}
	if len(later) > 0 {
		_ = s.Redis.ZAddXX(ctx, k.shardExp(i), later...).Err()
	}
	return settled, nil
}

// orphanAfter is how long past its deadline a held record waits before the shard sweeper asks
// whether its order still lives.
const orphanAfter = 5 * time.Second
