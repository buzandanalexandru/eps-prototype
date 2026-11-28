// Package inventory is the sharded stock engine of step 5 (O2, design D-9 and D-10): each pool
// instance's capacity is split over up to 32 shard counters, each on its own Redis hash tag
// (redisx.HotKey, eps:{<org>:<event>:<shard>}:...), so one big on-sale never sits on one hot key.
// A hold takes units shard by shard with one Lua script per shard and keeps its own record (state,
// deadline and where its units are) on a "home" tag chosen by its id; that record's state decides
// every race between a payment, a release and the sweepers, exactly like step 4's hold hash.
//
// It implements checkout.Inventory and checkout.HoldIndex, so step 4's checkout runs on it
// unchanged, plus Stock (held versus sold out) and the reconciler's reads and repairs.
package inventory

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// Shard rules (D-9).
const (
	MaxShards     = 32
	UnitsPerShard = 250
	// HomeTags is how many tags the hold records of one event spread over.
	HomeTags = 16
	// freezeFor bounds a resize: a crashed resize stops blocking holds after this long.
	freezeFor = 3 * time.Second
)

// ShardCount is how many shards a pool of this capacity gets: min(32, max(1, ceil(capacity/250))).
func ShardCount(capacity int64) int {
	if capacity <= 0 {
		return 1
	}
	n := int((capacity + UnitsPerShard - 1) / UnitsPerShard)
	return max(1, min(MaxShards, n))
}

// Split divides total over n shards as evenly as possible (the first shards get the remainder).
func Split(total int64, n int) []int64 {
	out := make([]int64, n)
	if n <= 0 {
		return out
	}
	base, rem := total/int64(n), total%int64(n)
	for i := range out {
		out[i] = base
		if int64(i) < rem {
			out[i]++
		}
	}
	return out
}

var (
	resizes      = metrics.NewCounter("eps_inventory_resizes_total", "Pool instances split (or re-split) over their shards.")
	shardRetries = metrics.NewCounter("eps_inventory_shard_retries_total", "Shard calls retried because a resize was running.")
)

// Sharded is the sharded engine.
type Sharded struct {
	Redis *redis.Client
	// Now is the clock (tests); nil means time.Now.
	Now func() time.Time
}

var (
	_ checkout.Inventory = (*Sharded)(nil)
	_ checkout.HoldIndex = (*Sharded)(nil)
)

func (s *Sharded) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// part makes an instance id usable inside a key (slot times carry a colon).
func part(instance string) string { return strings.ReplaceAll(instance, ":", "h") }

func shardTag(i int) string { return strconv.Itoa(i) }

func homeOf(holdID string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(holdID))
	return "h" + strconv.Itoa(int(h.Sum32()%HomeTags))
}

func startShard(holdID string, n int) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte("s" + holdID))
	return int(h.Sum32() % uint32(n))
}

type keys struct{ org, event string }

func (k keys) k(tag, kind string, rest ...string) string {
	key, err := redisx.HotKey(k.org, k.event, tag, kind, rest...)
	if err != nil {
		panic(fmt.Sprintf("inventory: bad key part (%s/%s): %v", k.org, k.event, err))
	}
	return key
}

func (k keys) ready() string                    { return k.k("0", "inv-ready") }
func (k keys) shards() string                   { return k.k("0", "inv-shards") }
func (k keys) insts() string                    { return k.k("0", "inv-insts") }
func (k keys) meta(p string) string             { return k.k("0", "inv-meta", p) }
func (k keys) lock(kind, p string) string       { return k.k("0", kind, p) }
func (k keys) ctr(i int, p string) string       { return k.k(shardTag(i), "inv", p) }
func (k keys) ctrPrefix(i int) string           { return strings.TrimSuffix(k.k(shardTag(i), "inv", "x"), "x") }
func (k keys) entry(i int, hold string) string  { return k.k(shardTag(i), "hs", hold) }
func (k keys) entryPrefix(i int) string         { return strings.TrimSuffix(k.k(shardTag(i), "hs", "x"), "x") }
func (k keys) shardExp(i int) string            { return k.k(shardTag(i), "hx") }
func (k keys) hold(hold string) string          { return k.k(homeOf(hold), "hold", hold) }
func (k keys) homeExp(home string) string       { return k.k(home, "hx") }
func (k keys) homeCommitted(home string) string { return k.k(home, "hc") }

func (s *Sharded) keysFor(org, event string) (keys, error) {
	if _, err := redisx.HotKey(org, event, "0", "inv-ready"); err != nil {
		return keys{}, err
	}
	return keys{org, event}, nil
}

func validHold(id string) bool {
	_, err := redisx.HotKey("o", "e", "0", "hold", id)
	return err == nil
}

// draw is a hold's units of one instance on one shard.
type draw struct {
	Shard int
	Part  string
	Qty   int64
}

func encodeDraws(ds []draw) string {
	var b strings.Builder
	for i, d := range ds {
		if i > 0 {
			b.WriteByte(';')
		}
		fmt.Fprintf(&b, "%d,%s,%d", d.Shard, d.Part, d.Qty)
	}
	return b.String()
}

func decodeDraws(s string) []draw {
	var out []draw
	for _, f := range strings.Split(s, ";") {
		p := strings.Split(f, ",")
		if len(p) != 3 {
			continue
		}
		i, err1 := strconv.Atoi(p[0])
		q, err2 := strconv.ParseInt(p[2], 10, 64)
		if err1 == nil && err2 == nil {
			out = append(out, draw{i, p[1], q})
		}
	}
	return out
}

func shardsOf(ds []draw) []int {
	seen := map[int]bool{}
	var out []int
	for _, d := range ds {
		if !seen[d.Shard] {
			seen[d.Shard] = true
			out = append(out, d.Shard)
		}
	}
	sort.Ints(out)
	return out
}

// meta is what the event's meta tag knows of one instance.
type meta struct {
	Cap  int64
	N    int
	Over bool
	Ok   bool
}

func (s *Sharded) readMeta(ctx context.Context, k keys, parts []string) (bool, []meta, error) {
	pipe := s.Redis.Pipeline()
	ready := pipe.Exists(ctx, k.ready())
	cmds := make([]*redis.SliceCmd, len(parts))
	for i, p := range parts {
		cmds[i] = pipe.HMGet(ctx, k.meta(p), "cap", "n", "over")
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return false, nil, err
	}
	out := make([]meta, len(parts))
	for i, c := range cmds {
		v := c.Val()
		if len(v) == 3 && v[0] != nil && v[1] != nil {
			out[i].Cap, _ = strconv.ParseInt(fmt.Sprint(v[0]), 10, 64)
			out[i].N, _ = strconv.Atoi(fmt.Sprint(v[1]))
			out[i].Over = fmt.Sprint(v[2]) == "1"
			out[i].Ok = out[i].N > 0
		}
	}
	return ready.Val() == 1, out, nil
}

// errBusy means another core is re-splitting the instance; the caller retries.
var errBusy = errors.New("inventory: resize in progress")

// resize splits (or re-splits) an instance over its shards for a capacity (D-9): new holds stop on
// every shard for a moment, each shard keeps what it has taken, and the free units (capacity minus
// everything taken) are spread evenly. A capacity below what is taken leaves every shard short in
// proportion and marks the instance "over", so commits check the capacity one at a time.
func (s *Sharded) resize(ctx context.Context, k keys, instance string, capacity int64) error {
	p := part(instance)
	lk := k.lock("inv-resize", p)
	ok, err := s.Redis.SetNX(ctx, lk, "1", 5*time.Second).Result()
	if err != nil {
		return err
	}
	if !ok {
		return errBusy
	}
	defer s.Redis.Del(context.WithoutCancel(ctx), lk)
	m, err := s.Redis.HMGet(ctx, k.meta(p), "cap", "n").Result()
	if err != nil {
		return err
	}
	oldN := 0
	if m[1] != nil {
		oldN, _ = strconv.Atoi(fmt.Sprint(m[1]))
		if c, _ := strconv.ParseInt(fmt.Sprint(m[0]), 10, 64); c == capacity {
			return nil // another core did it
		}
	}
	n := max(oldN, ShardCount(capacity))
	until := s.now().Add(freezeFor).UnixMilli()
	used := make([]int64, n)
	var total int64
	for i := 0; i < n; i++ {
		v, err := shardFreeze.Run(ctx, s.Redis, []string{k.ctr(i, p)}, until).Int64Slice()
		if err != nil {
			return err
		}
		used[i] = v[0]
		total += v[0]
	}
	alloc := make([]int64, n)
	over := false
	if free := capacity - total; free >= 0 {
		for i, f := range Split(free, n) {
			alloc[i] = used[i] + f
		}
	} else {
		over = true
		alloc = shortfall(used, -free)
	}
	for i := 0; i < n; i++ {
		if err := shardAlloc.Run(ctx, s.Redis, []string{k.ctr(i, p)}, alloc[i]).Err(); err != nil {
			return err
		}
	}
	ov := 0
	if over {
		ov = 1
	}
	pipe := s.Redis.TxPipeline()
	pipe.HSet(ctx, k.meta(p), "cap", capacity, "n", n, "over", ov, "inst", instance)
	pipe.SAdd(ctx, k.insts(), p)
	pipe.Eval(ctx, `local c = tonumber(redis.call('GET', KEYS[1]) or '0') if tonumber(ARGV[1]) > c then redis.call('SET', KEYS[1], ARGV[1]) end return 1`, []string{k.shards()}, n)
	if _, err := pipe.Exec(ctx); err != nil {
		return err
	}
	resizes.Inc()
	return nil
}

// shortfall takes `deficit` units of allocation away from shards in proportion to what each has
// taken (largest remainders first), never below zero.
func shortfall(used []int64, deficit int64) []int64 {
	var total int64
	for _, u := range used {
		total += u
	}
	out := make([]int64, len(used))
	copy(out, used)
	if total == 0 {
		return out
	}
	type rem struct {
		i int
		r int64
	}
	var left int64 = deficit
	var rs []rem
	for i, u := range used {
		d := deficit * u / total
		out[i] -= d
		left -= d
		rs = append(rs, rem{i, deficit * u % total})
	}
	sort.Slice(rs, func(a, b int) bool { return rs[a].r > rs[b].r })
	for j := 0; left > 0 && j < len(rs)*2; j++ {
		i := rs[j%len(rs)].i
		if out[i] > 0 {
			out[i]--
			left--
		}
	}
	return out
}

// ensure makes every instance split for its capacity, waiting for another core's resize.
func (s *Sharded) ensure(ctx context.Context, k keys, insts []string, capacity map[string]int64) ([]meta, error) {
	parts := make([]string, len(insts))
	for i, in := range insts {
		parts[i] = part(in)
	}
	for try := 0; try < 200; try++ {
		ready, ms, err := s.readMeta(ctx, k, parts)
		if err != nil {
			return nil, err
		}
		if !ready {
			return nil, checkout.ErrNotReady
		}
		again := false
		for i, in := range insts {
			if !ms[i].Ok || ms[i].Cap != capacity[in] {
				if err := s.resize(ctx, k, in, capacity[in]); err != nil && !errors.Is(err, errBusy) {
					return nil, err
				}
				again = true
			}
		}
		if !again {
			return ms, nil
		}
		if try > 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}
	return nil, errors.New("inventory: instance split is taking too long")
}

// takeShard runs one shard reserve, waiting out a running resize.
func (s *Sharded) takeShard(ctx context.Context, k keys, i int, p, holdID string, need int64, deadline time.Time) (int64, error) {
	for try := 0; ; try++ {
		got, err := shardReserve.Run(ctx, s.Redis, []string{k.ctr(i, p), k.entry(i, holdID), k.shardExp(i)},
			need, deadline.UnixMilli(), holdID, p, s.now().UnixMilli()).Int64()
		if err != nil {
			return 0, err
		}
		switch got {
		case -1:
			return 0, checkout.ErrNotReady
		case -2:
			if try >= 300 {
				return 0, errors.New("inventory: shard frozen too long")
			}
			shardRetries.Inc()
			time.Sleep(10 * time.Millisecond)
			continue
		case -3:
			return 0, nil // halted by the reconciler: no new holds
		}
		return got, nil
	}
}

// Reserve takes a hold's units (D-9): the hold's record first (state reserving, with its deadline,
// so a crash leaves something the sweeper can lapse), then each instance in a fixed order, shard
// by shard from a start chosen by the hold id. If an instance comes up short, everything taken is
// returned and the record dropped. A hold id seen before is not taken twice.
func (s *Sharded) Reserve(ctx context.Context, h checkout.Hold, capacity map[string]int64) error {
	if len(h.Draws) == 0 {
		return errors.New("inventory: empty hold")
	}
	if !validHold(h.ID) {
		return fmt.Errorf("inventory: bad hold id %q", h.ID)
	}
	k, err := s.keysFor(h.OrganiserID, h.EventID)
	if err != nil {
		return err
	}
	insts := sortedKeys(h.Draws)
	ms, err := s.ensure(ctx, k, insts, capacity)
	if err != nil {
		return err
	}
	home := homeOf(h.ID)
	began, err := masterBegin.Run(ctx, s.Redis, []string{k.hold(h.ID), k.homeExp(home)}, h.Deadline.UnixMilli(), h.ID).Int()
	if err != nil {
		return err
	}
	if began == 0 {
		return nil // the same hold again (a retried request): already taken once
	}
	var taken []draw
	undo := func() {
		c := context.WithoutCancel(ctx)
		s.settle(c, k, h.ID, taken, "release")
		_ = masterAbort.Run(c, s.Redis, []string{k.hold(h.ID), k.homeExp(home)}, h.ID).Err()
	}
	for idx, in := range insts {
		p := part(in)
		need := h.Draws[in]
		n := ms[idx].N
		start := startShard(h.ID, n)
		for j := 0; j < n && need > 0; j++ {
			i := (start + j) % n
			got, err := s.takeShard(ctx, k, i, p, h.ID, need, h.Deadline)
			if err != nil {
				undo()
				return err
			}
			if got > 0 {
				taken = append(taken, draw{i, p, got})
				need -= got
			}
		}
		if need > 0 {
			undo()
			return &checkout.ErrShort{Instance: in}
		}
	}
	st, err := masterFinish.Run(ctx, s.Redis, []string{k.hold(h.ID)}, encodeDraws(taken)).Text()
	if err != nil {
		undo()
		return err
	}
	if st != "held" {
		// The hold lapsed while it was being taken (a deadline already past): give it back.
		s.settle(context.WithoutCancel(ctx), k, h.ID, taken, "release")
		return &checkout.ErrShort{Instance: insts[0]}
	}
	s.index(ctx, h.OrganiserID, h.EventID)
	return nil
}

// settle commits or releases a hold's entries on each shard it touched.
func (s *Sharded) settle(ctx context.Context, k keys, holdID string, ds []draw, mode string) {
	for _, i := range shardsOf(ds) {
		_ = shardSettle.Run(ctx, s.Redis, []string{k.shardExp(i), k.entry(i, holdID)}, mode, k.ctrPrefix(i), holdID).Err()
	}
}

func (s *Sharded) index(ctx context.Context, org, event string) {
	key, _ := redisx.PlatformKey("hold-events")
	_ = s.Redis.SAdd(ctx, key, org+"/"+event).Err()
}

func sortedKeys(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Extend moves a live hold's deadline on its record and on every shard it touched.
func (s *Sharded) Extend(ctx context.Context, org, event, holdID string, deadline time.Time) error {
	k, err := s.keysFor(org, event)
	if err != nil || !validHold(holdID) {
		return checkout.ErrNoHold
	}
	d, err := masterExtend.Run(ctx, s.Redis, []string{k.hold(holdID), k.homeExp(homeOf(holdID))}, deadline.UnixMilli(), holdID).Text()
	if errors.Is(err, redis.Nil) {
		return checkout.ErrNoHold
	}
	if err != nil {
		return err
	}
	pipe := s.Redis.Pipeline()
	for _, i := range shardsOf(decodeDraws(d)) {
		pipe.ZAddXX(ctx, k.shardExp(i), redis.Z{Score: float64(deadline.UnixMilli()), Member: holdID})
	}
	_, err = pipe.Exec(ctx)
	return err
}

// Commit turns a live hold into sold units (idempotent). The record decides first (so a release
// racing it sees "committed"); then each shard moves the hold's units to sold. An instance whose
// capacity was lowered below what is taken ("over") is checked against the capacity, one commit
// at a time, so nothing ever sells above it.
func (s *Sharded) Commit(ctx context.Context, org, event, holdID string, capacity map[string]int64) error {
	k, err := s.keysFor(org, event)
	if err != nil || !validHold(holdID) {
		return checkout.ErrNoHold
	}
	home := homeOf(holdID)
	st, err := s.Redis.HMGet(ctx, k.hold(holdID), "state", "d").Result()
	if err != nil {
		return err
	}
	switch fmt.Sprint(st[0]) {
	case "committed":
		s.settle(ctx, k, holdID, decodeDraws(fmt.Sprint(st[1])), "commit") // finishes a crashed commit
		return nil
	case "held":
	default:
		return checkout.ErrNoHold
	}
	ds := decodeDraws(fmt.Sprint(st[1]))
	byInst := map[string]int64{}
	var insts []string
	for _, d := range ds {
		if _, ok := byInst[d.Part]; !ok {
			insts = append(insts, d.Part)
		}
		byInst[d.Part] += d.Qty
	}
	// Instance ids of the parts (capacity is keyed by instance id).
	ids := map[string]string{}
	for in := range capacity {
		ids[part(in)] = in
	}
	var checked []string
	for _, p := range insts {
		in, ok := ids[p]
		if !ok {
			continue // not capped (an instance the live version no longer has)
		}
		ms, err := s.ensure(ctx, k, []string{in}, capacity)
		if err != nil {
			return err
		}
		if ms[0].Over {
			checked = append(checked, in)
		}
	}
	var unlock []func()
	defer func() {
		for _, u := range unlock {
			u()
		}
	}()
	for _, in := range checked {
		u, err := s.mutex(ctx, k.lock("inv-commit", part(in)))
		if err != nil {
			return err
		}
		unlock = append(unlock, u)
		sold, err := s.sum(ctx, k, part(in), "sold")
		if err != nil {
			return err
		}
		if sold+byInst[part(in)] > capacity[in] {
			return &checkout.ErrShort{Instance: in}
		}
	}
	res, err := masterCommit.Run(ctx, s.Redis, []string{k.hold(holdID), k.homeExp(home), k.homeCommitted(home)}, holdID, s.now().UnixMilli()).StringSlice()
	if err != nil {
		return err
	}
	if res[0] == "NOHOLD" {
		return checkout.ErrNoHold
	}
	s.settle(ctx, k, holdID, decodeDraws(res[1]), "commit")
	return nil
}

// mutex takes a short Redis lock, waiting for it.
func (s *Sharded) mutex(ctx context.Context, key string) (func(), error) {
	for try := 0; try < 500; try++ {
		ok, err := s.Redis.SetNX(ctx, key, "1", 5*time.Second).Result()
		if err != nil {
			return nil, err
		}
		if ok {
			return func() { s.Redis.Del(context.WithoutCancel(ctx), key) }, nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return nil, errors.New("inventory: lock wait too long")
}

// sum adds a counter field over every shard of an instance.
func (s *Sharded) sum(ctx context.Context, k keys, p, field string) (int64, error) {
	n, err := s.Redis.HGet(ctx, k.meta(p), "n").Int()
	if err != nil && !errors.Is(err, redis.Nil) {
		return 0, err
	}
	pipe := s.Redis.Pipeline()
	cmds := make([]*redis.StringCmd, n)
	for i := 0; i < n; i++ {
		cmds[i] = pipe.HGet(ctx, k.ctr(i, p), field)
	}
	if n > 0 {
		if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
			return 0, err
		}
	}
	var t int64
	for _, c := range cmds {
		v, _ := c.Int64()
		t += v
	}
	return t, nil
}

// Release gives a live hold's units back and says what the hold was.
func (s *Sharded) Release(ctx context.Context, org, event, holdID string) (checkout.ReleaseResult, error) {
	k, err := s.keysFor(org, event)
	if err != nil || !validHold(holdID) {
		return checkout.ReleaseNotLive, err
	}
	out, err := s.releaseMany(ctx, k, []string{holdID})
	if err != nil {
		return checkout.ReleaseNotLive, err
	}
	return out[0], nil
}

// ReleaseMany releases several holds of one event: one script per home tag decides, then one
// script per shard gives the units back.
func (s *Sharded) ReleaseMany(ctx context.Context, org, event string, holdIDs []string) error {
	k, err := s.keysFor(org, event)
	if err != nil {
		return err
	}
	var ids []string
	for _, id := range holdIDs {
		if validHold(id) {
			ids = append(ids, id)
		}
	}
	_, err = s.releaseMany(ctx, k, ids)
	return err
}

func (s *Sharded) releaseMany(ctx context.Context, k keys, ids []string) ([]checkout.ReleaseResult, error) {
	out := make([]checkout.ReleaseResult, len(ids))
	byHome := map[string][]int{}
	for i, id := range ids {
		byHome[homeOf(id)] = append(byHome[homeOf(id)], i)
	}
	byShard := map[int][]string{}
	for home, idx := range byHome {
		keys := []string{k.homeExp(home)}
		args := []any{}
		for _, i := range idx {
			keys = append(keys, k.hold(ids[i]))
			args = append(args, ids[i])
		}
		res, err := masterRelease.Run(ctx, s.Redis, keys, args...).StringSlice()
		if err != nil {
			return nil, err
		}
		for j, r := range res {
			i := idx[j]
			switch {
			case r == "C":
				out[i] = checkout.ReleaseCommitted
			case strings.HasPrefix(r, "R:"):
				out[i] = checkout.ReleaseDone
				for _, sh := range shardsOf(decodeDraws(r[2:])) {
					byShard[sh] = append(byShard[sh], ids[i])
				}
			default:
				out[i] = checkout.ReleaseNotLive
			}
		}
	}
	for sh, hs := range byShard {
		for len(hs) > 0 {
			n := min(500, len(hs))
			keys := []string{k.shardExp(sh)}
			args := []any{"release", k.ctrPrefix(sh)}
			for _, id := range hs[:n] {
				keys = append(keys, k.entry(sh, id))
				args = append(args, id)
			}
			if err := shardSettle.Run(ctx, s.Redis, keys, args...).Err(); err != nil {
				return nil, err
			}
			hs = hs[n:]
		}
	}
	return out, nil
}

// HoldState is the hold record's state ("reserving" reads as "held").
func (s *Sharded) HoldState(ctx context.Context, org, event, holdID string) (string, error) {
	k, err := s.keysFor(org, event)
	if err != nil || !validHold(holdID) {
		return "", err
	}
	st, err := s.Redis.HGet(ctx, k.hold(holdID), "state").Result()
	if errors.Is(err, redis.Nil) {
		return "", nil
	}
	if st == "reserving" {
		st = "held"
	}
	return st, err
}

// Unsell gives a committed hold's sold units back.
func (s *Sharded) Unsell(ctx context.Context, org, event, holdID string) error {
	k, err := s.keysFor(org, event)
	if err != nil || !validHold(holdID) {
		return err
	}
	home := homeOf(holdID)
	d, err := masterUnsell.Run(ctx, s.Redis, []string{k.hold(holdID), k.homeCommitted(home)}, holdID).Text()
	if errors.Is(err, redis.Nil) {
		return nil
	}
	if err != nil {
		return err
	}
	byShard := map[int][]draw{}
	for _, x := range decodeDraws(d) {
		byShard[x.Shard] = append(byShard[x.Shard], x)
	}
	for sh, ds := range byShard {
		var keys []string
		var args []any
		for _, x := range ds {
			keys = append(keys, k.ctr(sh, x.Part))
			args = append(args, x.Qty)
		}
		if err := shardUnsell.Run(ctx, s.Redis, keys, args...).Err(); err != nil {
			return err
		}
	}
	return nil
}

// Used is units sold plus held per instance, over all its shards.
func (s *Sharded) Used(ctx context.Context, org, event string, instances []string) (map[string]int64, error) {
	st, err := s.Stock(ctx, org, event, instances)
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for in, v := range st {
		if u := v.Held + v.Sold; u > 0 {
			out[in] = u
		}
	}
	return out, nil
}

// Level is an instance's stock: free units, units held by live holds, and sold units.
type Level struct {
	Free, Held, Sold int64
	Halted           bool
}

// SoldOut is true only when nothing is free and no holds remain (D-9, the honest sold out).
func (l Level) SoldOut() bool { return l.Free <= 0 && l.Held <= 0 }

// AllHeld is true when nothing is free but holds remain: "all tickets are held right now; some may
// come back".
func (l Level) AllHeld() bool { return l.Free <= 0 && l.Held > 0 }

// Stock reads each instance's level over its shards. Instances never split read as empty.
func (s *Sharded) Stock(ctx context.Context, org, event string, instances []string) (map[string]Level, error) {
	k, err := s.keysFor(org, event)
	if err != nil {
		return nil, err
	}
	out := map[string]Level{}
	if len(instances) == 0 {
		return out, nil
	}
	parts := make([]string, len(instances))
	for i, in := range instances {
		parts[i] = part(in)
	}
	ready, ms, err := s.readMeta(ctx, k, parts)
	if err != nil {
		return nil, err
	}
	if !ready {
		return nil, checkout.ErrNotReady
	}
	pipe := s.Redis.Pipeline()
	cmds := make([][]*redis.SliceCmd, len(instances))
	for j, p := range parts {
		for i := 0; i < ms[j].N; i++ {
			cmds[j] = append(cmds[j], pipe.HMGet(ctx, k.ctr(i, p), "alloc", "used", "sold", "halt"))
		}
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	for j, in := range instances {
		var l Level
		for _, c := range cmds[j] {
			v := c.Val()
			a, u, sd := num(v[0]), num(v[1]), num(v[2])
			l.Free += max(0, a-u)
			l.Held += u - sd
			l.Sold += sd
			if fmt.Sprint(v[3]) == "1" {
				l.Halted = true
			}
		}
		if l.Halted {
			l.Free = 0
		}
		if ms[j].Ok && ms[j].Cap >= 0 {
			l.Free = max(0, min(l.Free, ms[j].Cap-l.Held-l.Sold))
		}
		out[in] = l
	}
	return out, nil
}

func num(v any) int64 {
	if v == nil {
		return 0
	}
	n, _ := strconv.ParseInt(fmt.Sprint(v), 10, 64)
	return n
}

// Rebuild loads an event's counters and live holds from the record of truth when Redis lost them
// (or on first use): everything goes onto shard 0 and the next hold re-splits each instance.
func (s *Sharded) Rebuild(ctx context.Context, org, event string, used, sold map[string]int64, holds []checkout.Hold) error {
	k, err := s.keysFor(org, event)
	if err != nil {
		return err
	}
	if n, err := s.Redis.Exists(ctx, k.ready()).Result(); err != nil || n == 1 {
		return err
	}
	set := map[string]bool{}
	for in := range used {
		set[in] = true
	}
	for in := range sold {
		set[in] = true
	}
	for _, h := range holds {
		for in := range h.Draws {
			set[in] = true
		}
	}
	insts := make([]string, 0, len(set))
	for in := range set {
		insts = append(insts, in)
	}
	sort.Strings(insts)
	// The hold records first (each on its home tag), so a commit right after the load finds them.
	for _, h := range holds {
		if !validHold(h.ID) {
			continue
		}
		var ds []draw
		for _, in := range sortedKeys(h.Draws) {
			ds = append(ds, draw{0, part(in), h.Draws[in]})
		}
		if err := masterRebuild.Run(ctx, s.Redis, []string{k.hold(h.ID), k.homeExp(homeOf(h.ID))}, h.Deadline.UnixMilli(), encodeDraws(ds), h.ID).Err(); err != nil {
			return err
		}
	}
	keys := []string{k.ready(), k.shards(), k.shardExp(0)}
	args := []any{len(insts), len(holds)}
	for _, in := range insts {
		keys = append(keys, k.meta(part(in)), k.ctr(0, part(in)))
		args = append(args, part(in), in, used[in], sold[in])
	}
	for _, h := range holds {
		keys = append(keys, k.entry(0, h.ID))
		hi := sortedKeys(h.Draws)
		args = append(args, h.ID, h.Deadline.UnixMilli(), len(hi))
		for _, in := range hi {
			args = append(args, part(in), h.Draws[in])
		}
	}
	if err := metaRebuild.Run(ctx, s.Redis, keys, args...).Err(); err != nil && !errors.Is(err, redis.Nil) {
		return err
	}
	if len(insts) > 0 {
		parts := make([]any, len(insts))
		for i, in := range insts {
			parts[i] = part(in)
		}
		_ = s.Redis.SAdd(ctx, k.insts(), parts...).Err()
	}
	s.index(ctx, org, event)
	return nil
}

// Lock serialises rebuilds of one event across cores.
func (s *Sharded) Lock(ctx context.Context, org, event string, ttl time.Duration) (bool, func(), error) {
	k, err := s.keysFor(org, event)
	if err != nil {
		return false, func() {}, err
	}
	key := k.lock("inv-rebuild-lock", "x")
	ok, err := s.Redis.SetNX(ctx, key, "1", ttl).Result()
	return ok, func() { _ = s.Redis.Del(context.WithoutCancel(ctx), key).Err() }, err
}

// Due returns hold ids of an event whose record's deadline passed before `before` (the orphan sweep).
func (s *Sharded) Due(ctx context.Context, org, event string, before time.Time, limit int64) ([]string, error) {
	k, err := s.keysFor(org, event)
	if err != nil {
		return nil, err
	}
	pipe := s.Redis.Pipeline()
	cmds := make([]*redis.StringSliceCmd, HomeTags)
	for h := 0; h < HomeTags; h++ {
		cmds[h] = pipe.ZRangeArgs(ctx, redis.ZRangeArgs{Key: k.homeExp("h" + strconv.Itoa(h)), Start: "-inf", Stop: strconv.FormatInt(before.UnixMilli(), 10), ByScore: true, Count: limit})
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	var out []string
	for _, c := range cmds {
		out = append(out, c.Val()...)
		if int64(len(out)) >= limit {
			return out[:limit], nil
		}
	}
	return out, nil
}

// Events lists the organiser/event pairs that ever held stock.
func (s *Sharded) Events(ctx context.Context) ([][2]string, error) {
	key, _ := redisx.PlatformKey("hold-events")
	ms, err := s.Redis.SMembers(ctx, key).Result()
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

// Forget drops a hold from the index.
func (s *Sharded) Forget(ctx context.Context, org, event, holdID string) error {
	k, err := s.keysFor(org, event)
	if err != nil || !validHold(holdID) {
		return err
	}
	return s.Redis.ZRem(ctx, k.homeExp(homeOf(holdID)), holdID).Err()
}

// Held is units held by live holds per instance (the shop's honest sold out).
func (s *Sharded) Held(ctx context.Context, org, event string, instances []string) (map[string]int64, error) {
	st, err := s.Stock(ctx, org, event, instances)
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for in, l := range st {
		if l.Held > 0 {
			out[in] = l.Held
		}
	}
	return out, nil
}

// HeldOf is the inventory's held reader when it is the sharded engine (nil otherwise).
func HeldOf(inv checkout.Inventory) checkout.HeldReader {
	if s, ok := inv.(*Sharded); ok {
		return s
	}
	return nil
}
