package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// DedupTTL is how long a consumer remembers the event ids it applied (longer than any resend).
const DedupTTL = 8 * 24 * time.Hour

// Consumer reads one topic in a consumer group (issuer, dashboard, webhooks), applies each event
// id once and acknowledges it afterwards: a crash before the acknowledgement re-delivers the
// event, and the id mark makes the repeat a no-op.
type Consumer struct {
	Redis  *redis.Client
	Topic  string
	Group  string
	Name   string
	Handle func(ctx context.Context, e Envelope) error
	// Organiser, when set, limits the consumer to one organiser's events (others are acknowledged
	// without being handled).
	Organiser string
	Log       *slog.Logger
	Block     time.Duration
	// Parallel handles up to this many entries of one read at once (default 1, in order). Only for
	// handlers that are safe in any order (step 7's issuer recomputes from the order's truth under
	// its row lock).
	Parallel int
}

func (c *Consumer) dedupKey(id string) string {
	k, _ := redisx.PlatformKey("dedup", c.Group, id)
	return k
}

// Ensure creates the group (from the start of the stream) if it does not exist.
func (c *Consumer) Ensure(ctx context.Context) error {
	err := c.Redis.XGroupCreateMkStream(ctx, StreamKey(c.Topic), c.Group, "0").Err()
	if err != nil && !strings.Contains(err.Error(), "BUSYGROUP") {
		return err
	}
	return nil
}

// Seen reports whether the group already applied an event id.
func (c *Consumer) Seen(ctx context.Context, id string) (bool, error) {
	n, err := c.Redis.Exists(ctx, c.dedupKey(id)).Result()
	return n > 0, err
}

// Apply handles one envelope unless its id was applied before; it returns whether it ran.
func (c *Consumer) Apply(ctx context.Context, e Envelope) (bool, error) {
	if c.Organiser != "" && e.OrganiserID != c.Organiser {
		return false, nil
	}
	seen, err := c.Seen(ctx, e.ID)
	if err != nil {
		return false, err
	}
	if seen {
		return false, nil
	}
	if err := c.Handle(ctx, e); err != nil {
		return false, err
	}
	return true, c.Redis.Set(ctx, c.dedupKey(e.ID), "1", DedupTTL).Err()
}

// ReadOnce reads up to count entries (first any this consumer left pending, then new ones),
// applies and acknowledges them. It returns how many were handled.
func (c *Consumer) ReadOnce(ctx context.Context, count int64) (int, error) {
	if count <= 0 {
		count = 100
	}
	handled := 0
	// Entries another consumer took and left unacknowledged for 30 s (it died) are taken over
	// (review D-j).
	if msgs, _, err := c.Redis.XAutoClaim(ctx, &redis.XAutoClaimArgs{Stream: StreamKey(c.Topic), Group: c.Group, Consumer: c.Name, MinIdle: ClaimIdle, Start: "0-0", Count: count}).Result(); err == nil {
		handled += c.handleAll(ctx, StreamKey(c.Topic), msgs)
	}
	for _, start := range []string{"0", ">"} {
		block := time.Duration(-1)
		if start == ">" {
			block = c.Block
			if block <= 0 {
				block = -1
			}
		}
		res, err := c.Redis.XReadGroup(ctx, &redis.XReadGroupArgs{Group: c.Group, Consumer: c.Name, Streams: []string{StreamKey(c.Topic), start}, Count: count, Block: block}).Result()
		if errors.Is(err, redis.Nil) {
			continue
		}
		if err != nil {
			return handled, err
		}
		for _, st := range res {
			handled += c.handleAll(ctx, st.Stream, st.Messages)
		}
	}
	return handled, nil
}

// handleAll handles entries in order, or Parallel at a time.
func (c *Consumer) handleAll(ctx context.Context, stream string, msgs []redis.XMessage) int {
	if c.Parallel <= 1 || len(msgs) <= 1 {
		n := 0
		for _, m := range msgs {
			if c.handle(ctx, stream, m) {
				n++
			}
		}
		return n
	}
	var n atomic.Int64
	var wg sync.WaitGroup
	sem := make(chan struct{}, c.Parallel)
	for _, m := range msgs {
		sem <- struct{}{}
		wg.Add(1)
		go func(m redis.XMessage) {
			defer func() { <-sem; wg.Done() }()
			if c.handle(ctx, stream, m) {
				n.Add(1)
			}
		}(m)
	}
	wg.Wait()
	return int(n.Load())
}

// ClaimIdle is how long an unacknowledged entry waits before another consumer takes it.
const ClaimIdle = 30 * time.Second

func (c *Consumer) handle(ctx context.Context, stream string, m redis.XMessage) bool {
	raw, _ := m.Values["env"].(string)
	var e Envelope
	if json.Unmarshal([]byte(raw), &e) != nil {
		_ = c.Redis.XAck(ctx, stream, c.Group, m.ID).Err()
		return false
	}
	ran, err := c.Apply(ctx, e)
	if err != nil {
		if c.Log != nil {
			c.Log.Warn("consumer: handler failed", "group", c.Group, "type", e.Type, "error", err.Error())
		}
		return false // stays pending: retried later
	}
	_ = c.Redis.XAck(ctx, stream, c.Group, m.ID).Err()
	return ran
}

// Run consumes until ctx ends.
func (c *Consumer) Run(ctx context.Context) {
	if c.Block <= 0 {
		c.Block = time.Second
	}
	for ctx.Err() == nil {
		if err := c.Ensure(ctx); err != nil {
			time.Sleep(time.Second)
			continue
		}
		if _, err := c.ReadOnce(ctx, 100); err != nil && ctx.Err() == nil {
			if c.Log != nil {
				c.Log.Warn("consumer: read failed", "group", c.Group, "error", err.Error())
			}
			time.Sleep(time.Second)
		}
	}
}
