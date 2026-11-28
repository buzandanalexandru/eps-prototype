package inventory_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

func reconEnv(t *testing.T, arena int64) (*salestest.Env, *salestest.Sale, *inventory.Sharded, *inventory.Reconciler) {
	env := salestest.Setup(t)
	sale := env.NewSale(t, arena, 100)
	inv := env.Svc.Inv.(*inventory.Sharded)
	return env, sale, inv, &inventory.Reconciler{Inv: inv, Pool: env.Pool, Log: env.Log}
}

func pay(t *testing.T, env *salestest.Env, sale *salestest.Sale, o *checkout.Order) {
	t.Helper()
	out, err := env.Svc.Complete(context.Background(), sale.Org, o.ID, "pay-"+o.ID[:8], "test payment", nil)
	if err != nil || !out.Paid {
		t.Fatalf("pay: %+v %v", out, err)
	}
}

func adjustments(t *testing.T, env *salestest.Env, kind string) int {
	t.Helper()
	var n int
	if err := env.Pool.QueryRow(context.Background(), "select count(*) from sales.inventory_adjustments where kind = $1", kind).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func level(t *testing.T, inv *inventory.Sharded, sale *salestest.Sale) inventory.Level {
	t.Helper()
	st, err := inv.Stock(context.Background(), sale.Org, sale.Event, []string{salestest.PoolArena})
	if err != nil {
		t.Fatal(err)
	}
	return st[salestest.PoolArena]
}

// Scenario "Counter lost an update": the counters show 3 fewer free than capacity minus paid minus
// live holds in two rounds in a row; the reconciler adds 3 back and records it.
func TestReconcilerRepairsLostUpdate(t *testing.T) {
	env, sale, inv, rec := reconEnv(t, 1000)
	ctx := context.Background()
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	pay(t, env, sale, o)
	sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	// A lost release: one shard's used counter is 3 too high.
	k, _ := redisx.HotKey(sale.Org, sale.Event, "2", "inv", salestest.PoolArena)
	if err := env.Redis.HIncrBy(ctx, k, "used", 3).Err(); err != nil {
		t.Fatal(err)
	}
	if l := level(t, inv, sale); l.Free != 1000-3-3 {
		t.Fatalf("free %d before the repair", l.Free)
	}
	if c, _ := rec.RunOnce(ctx); len(c) != 0 {
		t.Fatalf("first round corrected %v (needs two rounds)", c)
	}
	c, _ := rec.RunOnce(ctx)
	if len(c) != 1 || c[0].Kind != "shard_invariant" || c[0].Delta != -3 {
		t.Fatalf("second round: %+v", c)
	}
	if l := level(t, inv, sale); l.Free != 1000-3 || l.Sold != 2 || l.Held != 1 {
		t.Fatalf("after the repair %+v", l)
	}
	if adjustments(t, env, "shard_invariant") != 1 {
		t.Fatal("no adjustment recorded")
	}
	if c, _ := rec.RunOnce(ctx); len(c) != 0 {
		t.Fatalf("a clean round corrected %v", c)
	}
}

// Scenario "Commit without a paid order": stock committed for an order still unpaid 2 minutes
// later goes back to free stock.
func TestReconcilerReturnsUnpaidCommit(t *testing.T) {
	env, sale, inv, rec := reconEnv(t, 100)
	ctx := context.Background()
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	paid := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	pay(t, env, sale, paid)
	// The core committed the stock and stopped before the order was marked paid.
	if err := inv.Commit(ctx, sale.Org, sale.Event, o.HoldRef, map[string]int64{salestest.PoolArena: 100}); err != nil {
		t.Fatal(err)
	}
	if l := level(t, inv, sale); l.Sold != 3 {
		t.Fatalf("sold %d", l.Sold)
	}
	rec.UnpaidAfter = time.Hour
	if c, _ := rec.RunOnce(ctx); len(c) != 0 {
		t.Fatalf("corrected within the grace: %v", c)
	}
	rec.UnpaidAfter = time.Millisecond
	time.Sleep(5 * time.Millisecond)
	c, _ := rec.RunOnce(ctx)
	if len(c) != 1 || c[0].Kind != "unpaid_commit" {
		t.Fatalf("corrections %+v", c)
	}
	if l := level(t, inv, sale); l.Sold != 1 || l.Free != 99 {
		t.Fatalf("after the repair %+v", l)
	}
}

// Scenario "Oversell risk": after a store restart the database has 10 more paid tickets than the
// counters allow; the pool stops taking new holds and an alert is raised.
func TestReconcilerOversellRisk(t *testing.T) {
	env, sale, inv, rec := reconEnv(t, 20)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		pay(t, env, sale, sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 3}))
	}
	// The store lost 10 of the 15 sold (a restart from an old file), and new holds took the room.
	k, _ := redisx.HotKey(sale.Org, sale.Event, "0", "inv", salestest.PoolArena)
	_ = env.Redis.HIncrBy(ctx, k, "sold", -10).Err()
	_ = env.Redis.HIncrBy(ctx, k, "used", -10).Err()
	dropRecords(t, env, sale) // the old file had none of the paid holds' records either
	for i := 0; i < 5; i++ {
		sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 3})
	}
	if l := level(t, inv, sale); l.Free != 0 {
		t.Fatalf("free %d", l.Free)
	}
	_, _ = rec.RunOnce(ctx)
	c, _ := rec.RunOnce(ctx)
	if len(c) != 1 || c[0].Kind != "oversell_risk" || c[0].Delta != 10 {
		t.Fatalf("corrections %+v", c)
	}
	if !level(t, inv, sale).Halted {
		t.Fatal("pool not stopped")
	}
	var n int
	_ = env.Pool.QueryRow(ctx, "select count(*) from sales.outbox where type = 'inventory.oversell_risk'").Scan(&n)
	if n != 1 || adjustments(t, env, "oversell_risk") != 1 {
		t.Fatalf("alert events %d", n)
	}
	// A hold released frees stock, but the stopped pool takes no new holds.
	if _, _, err := env.Svc.CreateOrder(ctx, salestest.Buyer(), sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 1}), ""); err == nil {
		t.Fatal("a stopped pool took a hold")
	}
}

// Postgres ahead with free stock to cover it: sold is raised from free stock.
func TestReconcilerRaisesSold(t *testing.T) {
	env, sale, inv, rec := reconEnv(t, 100)
	ctx := context.Background()
	pay(t, env, sale, sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 4}))
	k, _ := redisx.HotKey(sale.Org, sale.Event, "0", "inv", salestest.PoolArena)
	_ = env.Redis.HIncrBy(ctx, k, "sold", -4).Err()
	_ = env.Redis.HIncrBy(ctx, k, "used", -4).Err()
	dropRecords(t, env, sale)
	_, _ = rec.RunOnce(ctx)
	c, _ := rec.RunOnce(ctx)
	if len(c) != 1 || c[0].Kind != "sold_raised" || c[0].Delta != 4 {
		t.Fatalf("corrections %+v", c)
	}
	if l := level(t, inv, sale); l.Sold != 4 || l.Free != 96 || l.Halted {
		t.Fatalf("level %+v", l)
	}
}

// dropRecords deletes the event's committed hold records (what a store restarted from an old file
// has lost along with the counters).
func dropRecords(t *testing.T, env *salestest.Env, sale *salestest.Sale) {
	t.Helper()
	ctx := context.Background()
	keys, err := env.Redis.Keys(ctx, "eps:{"+sale.Org+":"+sale.Event+":h*}:hold:*").Result()
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range keys {
		if st, _ := env.Redis.HGet(ctx, k, "state").Result(); st == "committed" {
			_ = env.Redis.Del(ctx, k).Err()
		}
	}
}

// Review corr L2: a paid order whose hold the store still has as held (its commit not applied yet)
// is not absorbed as "Postgres ahead": the orphan sweep commits it.
func TestReconcilerLeavesPendingCommits(t *testing.T) {
	env, sale, inv, rec := reconEnv(t, 100)
	ctx := context.Background()
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 3})
	// Marked paid in Postgres, stock not committed yet (the wallet path commits after its transaction).
	if _, err := env.Pool.Exec(ctx, "update sales.orders set state = 'paid', paid_at = now() where id = $1", o.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if c, _ := rec.RunOnce(ctx); len(c) != 0 {
			t.Fatalf("round %d corrected %+v", i, c)
		}
	}
	if l := level(t, inv, sale); l.Sold != 0 || l.Held != 3 {
		t.Fatalf("level %+v", l)
	}
}
