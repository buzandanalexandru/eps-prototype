// Package checkout holds stock for an order, keeps the order's state machine and prices it from the
// sale snapshot (step 4: D2, D3, D7, D8, D9, D11; O2, O3). Stock lives behind the Inventory
// interface: one counter per pool instance in this step, the sharded engine in step 5.
package checkout

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// Hold is stock taken for one order: units per pool instance until a deadline.
type Hold struct {
	OrganiserID string
	EventID     string
	ID          string // the order id (or <order>_r<n> for a re-hold after a late payment)
	Draws       map[string]int64
	Deadline    time.Time
}

// Inventory takes, extends, commits and releases holds. Implementations must be all-or-nothing
// across every instance of a hold and must never take an instance above the capacity passed in.
type Inventory interface {
	// Reserve takes the hold's units; ErrShort names the first instance without room and
	// ErrNotReady means the counters must be rebuilt from Postgres first.
	Reserve(ctx context.Context, h Hold, capacity map[string]int64) error
	// Extend moves a live hold's deadline (a payment in progress).
	Extend(ctx context.Context, org, event, holdID string, deadline time.Time) error
	// Commit turns a live hold into sold units, never taking sold units above capacity.
	Commit(ctx context.Context, org, event, holdID string, capacity map[string]int64) error
	// Release gives a live hold's units back; it reports whether it released anything.
	Release(ctx context.Context, org, event, holdID string) (bool, error)
	// Used is units sold plus held per instance (for availability and the publish gate).
	Used(ctx context.Context, org, event string, instances []string) (map[string]int64, error)
	// Rebuild sets the counters and live holds of an event from the record of truth, unless they
	// are already present.
	Rebuild(ctx context.Context, org, event string, used, sold map[string]int64, holds []Hold) error
}

// ErrShort means an instance had no room (or a commit would sell above capacity).
type ErrShort struct{ Instance string }

func (e *ErrShort) Error() string { return "checkout: no room in " + e.Instance }

// ErrNotReady means the event's counters are not in Redis (first use, or Redis restarted).
var ErrNotReady = errors.New("checkout: inventory counters not loaded")

// ErrNoHold means the hold is not live (released, committed or unknown).
var ErrNoHold = errors.New("checkout: hold not live")

// Shard is the only shard of step 4 (step 5 adds more behind the same interface).
const Shard = "0"

// RedisInventory is the single-counter inventory: for each pool instance a "used" counter (sold +
// held) and a "sold" counter, a hash per hold and an expiry index, all under one hot-key hash tag
// per event, changed only by Lua scripts so every move is atomic.
type RedisInventory struct {
	Redis *redis.Client
}

// instance ids carry a colon in slot times (pool@2027-07-02T01:00); key parts cannot.
func instPart(instance string) string { return strings.ReplaceAll(instance, ":", "h") }

func (r *RedisInventory) key(org, event, kind string, rest ...string) (string, error) {
	return redisx.HotKey(org, event, Shard, kind, rest...)
}

func (r *RedisInventory) counterKeys(org, event string, instances []string) (used, sold []string, err error) {
	for _, in := range instances {
		u, e1 := r.key(org, event, "inv-used", instPart(in))
		s, e2 := r.key(org, event, "inv-sold", instPart(in))
		if e1 != nil || e2 != nil {
			return nil, nil, fmt.Errorf("checkout: bad instance %q", in)
		}
		used = append(used, u)
		sold = append(sold, s)
	}
	return used, sold, nil
}

func sortedInstances(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// KEYS: ready, hold, expiry index, used counters...  ARGV: deadline ms, n, then per instance: qty, capacity, used key name
var reserveScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 0 then return {'NOTREADY'} end
if redis.call('EXISTS', KEYS[2]) == 1 then return {'EXISTS'} end
local n = tonumber(ARGV[2])
for i = 1, n do
  local qty = tonumber(ARGV[3 + (i-1)*2])
  local cap = tonumber(ARGV[4 + (i-1)*2])
  local used = tonumber(redis.call('GET', KEYS[3 + i]) or '0')
  if used + qty > cap then return {'SHORT', tostring(i)} end
end
redis.call('HSET', KEYS[2], 'state', 'held', 'deadline', ARGV[1], 'n', ARGV[2])
for i = 1, n do
  local qty = tonumber(ARGV[3 + (i-1)*2])
  redis.call('INCRBY', KEYS[3 + i], qty)
  redis.call('HSET', KEYS[2], 'k' .. i, KEYS[3 + i], 'q' .. i, qty)
end
redis.call('ZADD', KEYS[3], ARGV[1], ARGV[3 + n*2])
return {'OK'}
`)

// KEYS: hold, expiry index. ARGV: hold id
var releaseScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'state') ~= 'held' then return 0 end
local n = tonumber(redis.call('HGET', KEYS[1], 'n'))
for i = 1, n do
  local k = redis.call('HGET', KEYS[1], 'k' .. i)
  local q = tonumber(redis.call('HGET', KEYS[1], 'q' .. i))
  redis.call('DECRBY', k, q)
end
redis.call('HSET', KEYS[1], 'state', 'released')
redis.call('EXPIRE', KEYS[1], 86400)
redis.call('ZREM', KEYS[2], ARGV[1])
return 1
`)

// KEYS: hold, expiry index, sold counters (same order as the hold's used counters). ARGV: hold id, then capacities
var commitScript = redis.NewScript(`
local st = redis.call('HGET', KEYS[1], 'state')
if st == 'committed' then return {'OK'} end
if st ~= 'held' then return {'NOHOLD'} end
local n = tonumber(redis.call('HGET', KEYS[1], 'n'))
for i = 1, n do
  local q = tonumber(redis.call('HGET', KEYS[1], 'q' .. i))
  local sold = tonumber(redis.call('GET', KEYS[2 + i]) or '0')
  if sold + q > tonumber(ARGV[1 + i]) then return {'SHORT', tostring(i)} end
end
for i = 1, n do
  redis.call('INCRBY', KEYS[2 + i], tonumber(redis.call('HGET', KEYS[1], 'q' .. i)))
end
redis.call('HSET', KEYS[1], 'state', 'committed')
redis.call('EXPIRE', KEYS[1], 86400)
redis.call('ZREM', KEYS[2], ARGV[1])
return {'OK'}
`)

// KEYS: hold, expiry index. ARGV: deadline ms, hold id
var extendScript = redis.NewScript(`
if redis.call('HGET', KEYS[1], 'state') ~= 'held' then return 0 end
redis.call('HSET', KEYS[1], 'deadline', ARGV[1])
redis.call('ZADD', KEYS[2], ARGV[1], ARGV[2])
return 1
`)

// KEYS: ready, then used counters, then sold counters, then hold keys, expiry index.
// ARGV: n instances, used values..., sold values..., m holds, then per hold: id, deadline, k, then k pairs (counter index, qty)
var rebuildScript = redis.NewScript(`
if redis.call('EXISTS', KEYS[1]) == 1 then return 0 end
local n = tonumber(ARGV[1])
for i = 1, n do
  redis.call('SET', KEYS[1 + i], ARGV[1 + i])
  redis.call('SET', KEYS[1 + n + i], ARGV[1 + n + i])
end
local m = tonumber(ARGV[2 + 2*n])
local a = 3 + 2*n
local exp = KEYS[2 + 2*n + m]
for j = 1, m do
  local hk = KEYS[1 + 2*n + j]
  local id = ARGV[a]; local deadline = ARGV[a+1]; local k = tonumber(ARGV[a+2])
  a = a + 3
  redis.call('DEL', hk)
  redis.call('HSET', hk, 'state', 'held', 'deadline', deadline, 'n', k)
  for i = 1, k do
    local ci = tonumber(ARGV[a]); local q = ARGV[a+1]
    a = a + 2
    redis.call('HSET', hk, 'k' .. i, KEYS[1 + ci], 'q' .. i, q)
  end
  redis.call('ZADD', exp, deadline, id)
end
redis.call('SET', KEYS[1], '1')
return 1
`)

func (r *RedisInventory) Reserve(ctx context.Context, h Hold, capacity map[string]int64) error {
	if len(h.Draws) == 0 {
		return errors.New("checkout: empty hold")
	}
	insts := sortedInstances(h.Draws)
	used, _, err := r.counterKeys(h.OrganiserID, h.EventID, insts)
	if err != nil {
		return err
	}
	ready, _ := r.key(h.OrganiserID, h.EventID, "inv-ready")
	hold, err := r.key(h.OrganiserID, h.EventID, "hold", h.ID)
	if err != nil {
		return err
	}
	exp, _ := r.key(h.OrganiserID, h.EventID, "holds-exp")
	keys := append([]string{ready, hold, exp}, used...)
	args := []any{h.Deadline.UnixMilli(), len(insts)}
	for _, in := range insts {
		args = append(args, h.Draws[in], capacity[in])
	}
	args = append(args, h.ID)
	res, err := reserveScript.Run(ctx, r.Redis, keys, args...).StringSlice()
	if err != nil {
		return err
	}
	switch res[0] {
	case "OK":
		r.index(ctx, h.OrganiserID, h.EventID)
		return nil
	case "NOTREADY":
		return ErrNotReady
	case "EXISTS":
		return nil // the same hold again (a retried request): already taken once
	case "SHORT":
		i, _ := strconv.Atoi(res[1])
		return &ErrShort{Instance: insts[i-1]}
	}
	return fmt.Errorf("checkout: reserve: %v", res)
}

// index remembers which events have holds, for the orphan sweep.
func (r *RedisInventory) index(ctx context.Context, org, event string) {
	k, _ := redisx.PlatformKey("hold-events")
	_ = r.Redis.SAdd(ctx, k, org+"/"+event).Err()
}

func (r *RedisInventory) Extend(ctx context.Context, org, event, holdID string, deadline time.Time) error {
	hold, err := r.key(org, event, "hold", holdID)
	if err != nil {
		return err
	}
	exp, _ := r.key(org, event, "holds-exp")
	n, err := extendScript.Run(ctx, r.Redis, []string{hold, exp}, deadline.UnixMilli(), holdID).Int()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNoHold
	}
	return nil
}

func (r *RedisInventory) Commit(ctx context.Context, org, event, holdID string, capacity map[string]int64) error {
	hold, err := r.key(org, event, "hold", holdID)
	if err != nil {
		return err
	}
	exp, _ := r.key(org, event, "holds-exp")
	// The hold's counters, in its own order, to find the matching sold counters and capacities.
	vals, err := r.Redis.HGetAll(ctx, hold).Result()
	if err != nil {
		return err
	}
	if vals["state"] == "committed" {
		return nil
	}
	if vals["state"] != "held" {
		return ErrNoHold
	}
	n, _ := strconv.Atoi(vals["n"])
	keys := []string{hold, exp}
	args := []any{holdID}
	insts := make([]string, n)
	for i := 1; i <= n; i++ {
		usedKey := vals["k"+strconv.Itoa(i)]
		soldKey := strings.Replace(usedKey, ":inv-used:", ":inv-sold:", 1)
		keys = append(keys, soldKey)
		in := usedKey[strings.LastIndex(usedKey, ":inv-used:")+len(":inv-used:"):]
		insts[i-1] = in
		args = append(args, capacityFor(capacity, in))
	}
	res, err := commitScript.Run(ctx, r.Redis, keys, args...).StringSlice()
	if err != nil {
		return err
	}
	switch res[0] {
	case "OK":
		return nil
	case "NOHOLD":
		return ErrNoHold
	case "SHORT":
		i, _ := strconv.Atoi(res[1])
		return &ErrShort{Instance: strings.ReplaceAll(insts[i-1], "h", ":")}
	}
	return fmt.Errorf("checkout: commit: %v", res)
}

// capacityFor finds an instance's capacity by its key part (colons were replaced in keys).
func capacityFor(capacity map[string]int64, part string) int64 {
	for in, c := range capacity {
		if instPart(in) == part {
			return c
		}
	}
	return 0
}

func (r *RedisInventory) Release(ctx context.Context, org, event, holdID string) (bool, error) {
	hold, err := r.key(org, event, "hold", holdID)
	if err != nil {
		return false, err
	}
	exp, _ := r.key(org, event, "holds-exp")
	n, err := releaseScript.Run(ctx, r.Redis, []string{hold, exp}, holdID).Int()
	return n == 1, err
}

func (r *RedisInventory) Used(ctx context.Context, org, event string, instances []string) (map[string]int64, error) {
	out := map[string]int64{}
	if len(instances) == 0 {
		return out, nil
	}
	ready, _ := r.key(org, event, "inv-ready")
	used, _, err := r.counterKeys(org, event, instances)
	if err != nil {
		return nil, err
	}
	vals, err := r.Redis.MGet(ctx, append([]string{ready}, used...)...).Result()
	if err != nil {
		return nil, err
	}
	if vals[0] == nil {
		return nil, ErrNotReady
	}
	for i, in := range instances {
		if s, ok := vals[i+1].(string); ok {
			if n, err := strconv.ParseInt(s, 10, 64); err == nil && n > 0 {
				out[in] = n
			}
		}
	}
	return out, nil
}

func (r *RedisInventory) Rebuild(ctx context.Context, org, event string, used, sold map[string]int64, holds []Hold) error {
	set := map[string]bool{}
	for k := range used {
		set[k] = true
	}
	for k := range sold {
		set[k] = true
	}
	for _, h := range holds {
		for k := range h.Draws {
			set[k] = true
		}
	}
	insts := make([]string, 0, len(set))
	for k := range set {
		insts = append(insts, k)
	}
	sort.Strings(insts)
	idx := map[string]int{}
	for i, in := range insts {
		idx[in] = i + 1
	}
	usedKeys, soldKeys, err := r.counterKeys(org, event, insts)
	if err != nil {
		return err
	}
	ready, _ := r.key(org, event, "inv-ready")
	exp, _ := r.key(org, event, "holds-exp")
	keys := append([]string{ready}, usedKeys...)
	keys = append(keys, soldKeys...)
	args := []any{len(insts)}
	for _, in := range insts {
		args = append(args, used[in])
	}
	for _, in := range insts {
		args = append(args, sold[in])
	}
	args = append(args, len(holds))
	for _, h := range holds {
		hk, err := r.key(org, event, "hold", h.ID)
		if err != nil {
			return err
		}
		keys = append(keys, hk)
		hi := sortedInstances(h.Draws)
		args = append(args, h.ID, h.Deadline.UnixMilli(), len(hi))
		for _, in := range hi {
			args = append(args, idx[in], h.Draws[in])
		}
	}
	keys = append(keys, exp)
	if err := rebuildScript.Run(ctx, r.Redis, keys, args...).Err(); err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	r.index(ctx, org, event)
	return nil
}

// Due returns hold ids of an event whose Redis deadline passed before `before` (the orphan sweep).
func (r *RedisInventory) Due(ctx context.Context, org, event string, before time.Time, limit int64) ([]string, error) {
	exp, err := r.key(org, event, "holds-exp")
	if err != nil {
		return nil, err
	}
	return r.Redis.ZRangeArgs(ctx, redis.ZRangeArgs{Key: exp, Start: "-inf", Stop: strconv.FormatInt(before.UnixMilli(), 10), ByScore: true, Count: limit}).Result()
}

// Events lists the organiser/event pairs that ever held stock (for the orphan sweep).
func (r *RedisInventory) Events(ctx context.Context) ([][2]string, error) {
	k, _ := redisx.PlatformKey("hold-events")
	ms, err := r.Redis.SMembers(ctx, k).Result()
	if err != nil {
		return nil, err
	}
	out := make([][2]string, 0, len(ms))
	for _, m := range ms {
		if a, b, ok := strings.Cut(m, "/"); ok {
			out = append(out, [2]string{a, b})
		}
	}
	return out, nil
}
