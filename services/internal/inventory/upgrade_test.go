package inventory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// step4Keys writes step 4's single-counter state for an event (as Redis held it before the upgrade).
func step4Keys(t *testing.T, rdb *redis.Client, org, event, instance string, used, sold int64, live map[string]int64) []string {
	t.Helper()
	ctx := context.Background()
	k := func(kind string, rest ...string) string {
		s, err := redisx.HotKey(org, event, "0", kind, rest...)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	keys := []string{k("inv-ready"), k("inv-used", instance), k("inv-sold", instance), k("holds-exp")}
	pipe := rdb.Pipeline()
	pipe.Set(ctx, keys[0], "1", 0)
	pipe.Set(ctx, keys[1], used, 0)
	pipe.Set(ctx, keys[2], sold, 0)
	for id, q := range live {
		hk := k("hold", id)
		keys = append(keys, hk)
		pipe.HSet(ctx, hk, "state", "held", "deadline", time.Now().Add(time.Minute).UnixMilli(), "n", 1, "k1", keys[1], "q1", q)
		pipe.ZAdd(ctx, keys[3], redis.Z{Score: float64(time.Now().Add(time.Minute).UnixMilli()), Member: id})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		t.Fatal(err)
	}
	return keys
}

// Review D-a (corr H1, the reviewer's repro): Redis holds step 4's state after the upgrade. The
// sharded engine does not mistake step 4's "ready" for its own: the first use loads the counters
// from the orders, a live step-4 hold still commits, nothing sells above the capacity, and step
// 4's keys are deleted.
func TestStep4StateAfterUpgrade(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 10, 100)
	paid := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 6})
	if out, err := env.Svc.Complete(ctx, sale.Org, paid.ID, "pay-"+paid.ID[:8], "test", nil); err != nil || !out.Paid {
		t.Fatal(err)
	}
	live := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	more := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	// The deploy: Redis has only step 4's keys.
	if err := env.Redis.FlushAll(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	old := step4Keys(t, env.Redis, sale.Org, sale.Event, salestest.PoolArena, 10, 6, map[string]int64{live.HoldRef: 2, more.HoldRef: 2})
	inv := env.Svc.Inv.(*inventory.Sharded)
	// The raw engine refuses to act on state it has not loaded (the repro's "10 more on 10").
	if err := inv.Reserve(ctx, checkout.Hold{OrganiserID: sale.Org, EventID: sale.Event, ID: "new10", Draws: map[string]int64{salestest.PoolArena: 10}, Deadline: time.Now().Add(time.Minute)},
		map[string]int64{salestest.PoolArena: 10}); !errors.Is(err, checkout.ErrNotReady) {
		t.Fatalf("reserve on unloaded counters: %v", err)
	}
	if err := inv.Commit(ctx, sale.Org, sale.Event, live.HoldRef, map[string]int64{salestest.PoolArena: 10}); !errors.Is(err, checkout.ErrNotReady) {
		t.Fatalf("commit on unloaded counters: %v", err)
	}
	// The live step-4 hold is paid: the checkout loads the counters and commits it.
	out, err := env.Svc.Complete(ctx, sale.Org, live.ID, "pay-"+live.ID[:8], "test", nil)
	if err != nil || !out.Paid || out.Refund {
		t.Fatalf("paying the step-4 hold: %+v %v", out, err)
	}
	if _, _, err := env.Svc.CreateOrder(ctx, salestest.Buyer(), sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 1}), ""); !isSoldOut(err) {
		t.Fatalf("a hold above 6 sold + 2 paid + 2 held of 10: %v", err)
	}
	if n, _ := env.Redis.Exists(ctx, old...).Result(); n != 0 {
		t.Fatalf("%d of step 4's keys left", n)
	}
	st, _ := inv.Stock(ctx, sale.Org, sale.Event, []string{salestest.PoolArena})
	if l := st[salestest.PoolArena]; l.Sold != 8 || l.Held != 2 || l.Free != 0 {
		t.Fatalf("level %+v, want 8 sold and 2 held", l)
	}
}

func isSoldOut(err error) bool {
	var p *apperr.Problem
	return errors.As(err, &p) && p.Code == apperr.HoldSoldOut
}
