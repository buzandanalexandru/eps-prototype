package outbox_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
)

func writeN(t *testing.T, ctx context.Context, pool interface {
	db.Beginner
}, org string, n int) {
	t.Helper()
	err := db.WithTx(ctx, pool, db.RoleCore, org, func(tx pgx.Tx) error {
		for i := 0; i < n; i++ {
			if _, err := outbox.Write(ctx, tx, outbox.Event{Topic: outbox.TopicOrders, Type: "order.paid", OrganiserID: org, Payload: map[string]int{"n": i}}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Scenario "Relay crash": published but not marked sent → published again after restart, none lost.
func TestRelayCrashResendsAndLosesNothing(t *testing.T) {
	pool, rdb := setup(t)
	ctx := context.Background()
	writeN(t, ctx, pool, orgA, 3)
	crash := errors.New("relay killed")
	r := &outbox.Relay{Pool: pool, Redis: rdb, AfterPublish: func() error { return crash }}
	if _, err := r.RunOnce(ctx); !errors.Is(err, crash) {
		t.Fatalf("want the simulated crash, got %v", err)
	}
	if n := rdb.XLen(ctx, outbox.StreamKey(outbox.TopicOrders)).Val(); n != 3 {
		t.Fatalf("stream holds %d entries after the crash, want 3", n)
	}
	r.AfterPublish = nil
	n, err := r.RunOnce(ctx)
	if err != nil || n != 3 {
		t.Fatalf("restart relayed %d (%v), want 3", n, err)
	}
	if n := rdb.XLen(ctx, outbox.StreamKey(outbox.TopicOrders)).Val(); n != 6 {
		t.Fatalf("stream holds %d entries, want 6 (each event twice)", n)
	}
	if n, _ := r.RunOnce(ctx); n != 0 {
		t.Fatalf("a third round relayed %d", n)
	}
	// A consumer applies each id once.
	applied := map[string]int{}
	c := &outbox.Consumer{Redis: rdb, Topic: outbox.TopicOrders, Group: "test", Name: "c1", Handle: func(_ context.Context, e outbox.Envelope) error {
		applied[e.ID]++
		return nil
	}}
	if err := c.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReadOnce(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if len(applied) != 3 {
		t.Fatalf("applied %d distinct events, want 3", len(applied))
	}
	for id, k := range applied {
		if k != 1 {
			t.Fatalf("event %s applied %d times", id, k)
		}
	}
}

// Scenario "Latency": 1,000 events recorded in one second reach the stream within 2 seconds.
func TestRelayThousandEventsWithinTwoSeconds(t *testing.T) {
	pool, rdb := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := &outbox.Relay{Pool: pool, Redis: rdb}
	go r.Run(ctx, 200*time.Millisecond)
	start := time.Now()
	for i := 0; i < 10; i++ { // ten transactions of 100 events, like 1,000 paid orders in a second
		writeN(t, ctx, pool, orgA, 100)
	}
	deadline := start.Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rdb.XLen(ctx, outbox.StreamKey(outbox.TopicOrders)).Val() >= 1000 {
			t.Logf("1,000 events in the stream after %s", time.Since(start))
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("only %d of 1,000 events reached the stream within 2 s", rdb.XLen(ctx, outbox.StreamKey(outbox.TopicOrders)).Val())
}

// Events of one topic keep their recorded order in the stream.
func TestRelayKeepsOrder(t *testing.T) {
	pool, rdb := setup(t)
	ctx := context.Background()
	writeN(t, ctx, pool, orgA, 20)
	if _, err := (&outbox.Relay{Pool: pool, Redis: rdb, Batch: 7}).RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	msgs := rdb.XRange(ctx, outbox.StreamKey(outbox.TopicOrders), "-", "+").Val()
	if len(msgs) != 20 {
		t.Fatalf("%d entries", len(msgs))
	}
	for i, m := range msgs {
		if want := fmt.Sprintf(`"payload":{"n":%d}`, i); !contains(m.Values["env"].(string), want) {
			t.Fatalf("entry %d out of order: %v", i, m.Values["env"])
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
