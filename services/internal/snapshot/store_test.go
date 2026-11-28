package snapshot_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func gaAllIn(t *testing.T, s *snapshot.Snapshot) int64 {
	t.Helper()
	return s.Products[0].PriceSteps[0].AllInMinor
}

func redisClient(t *testing.T) *redis.Client {
	t.Helper()
	c, err := redisx.Open(testdb.RedisURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// Scenario "Draft change during a sale": the core keeps the published price.
func TestDraftChangeIsIgnored(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	w := newWorld(t, pool)
	w.publish(t, 1, 53900)
	if _, err := pool.Exec(ctx, "update control.price_steps set price_minor = 99900 where product_id = $1", w.product); err != nil {
		t.Fatal(err)
	}
	st := snapshot.NewStore(snapshot.DBSource{Pool: pool}, redisClient(t), nil, snapshot.Options{})
	s, err := st.Load(ctx, w.event)
	if err != nil {
		t.Fatal(err)
	}
	if got := gaAllIn(t, s); got != 55248 {
		t.Fatalf("GA all-in = %d, want the published 55248", got)
	}
}

// The core role reads only through the published-only function: the snapshot table and the
// catalogue's draft tables are closed to it.
func TestCoreRoleCannotReadDraftsOrSnapshotTable(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	w := newWorld(t, pool)
	w.publish(t, 1, 53900)
	for _, q := range []string{"select count(*) from control.sale_snapshots", "select count(*) from control.price_steps", "select count(*) from control.products"} {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = tx.Exec(ctx, "set local role eps_core")
		var n int
		err = tx.QueryRow(ctx, q).Scan(&n)
		_ = tx.Rollback(ctx)
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("%s as eps_core: want permission denied, got n=%d err=%v", q, n, err)
		}
	}
}

// Scenario "Hash mismatch": a cached copy whose content does not match its hash is discarded,
// reloaded from the database and logged.
func TestHashMismatchReloadsFromDatabase(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	rdb := redisClient(t)
	w := newWorld(t, pool)
	w.publish(t, 1, 53900)
	first := snapshot.NewStore(snapshot.DBSource{Pool: pool}, rdb, nil, snapshot.Options{RedisTTL: time.Minute})
	if _, err := first.Load(ctx, w.event); err != nil {
		t.Fatal(err)
	}
	key, _ := redisx.Key(w.org, w.event, "snapshot", "live")
	doc, err := rdb.HGet(ctx, key, "doc").Result()
	if err != nil {
		t.Fatalf("live copy not in Redis: %v", err)
	}
	tampered := strings.Replace(doc, "55248", "100", 1)
	if err := rdb.HSet(ctx, key, "doc", tampered).Err(); err != nil {
		t.Fatal(err)
	}
	log, buf := newLog()
	second := snapshot.NewStore(snapshot.DBSource{Pool: pool}, rdb, log, snapshot.Options{RedisTTL: time.Minute})
	s, err := second.Load(ctx, w.event)
	if err != nil {
		t.Fatal(err)
	}
	if gaAllIn(t, s) != 55248 {
		t.Fatalf("served the tampered copy: %d", gaAllIn(t, s))
	}
	if !strings.Contains(buf.String(), "cached snapshot discarded") {
		t.Fatalf("mismatch not logged: %s", buf.String())
	}
	if again, _ := rdb.HGet(ctx, key, "doc").Result(); again != doc {
		t.Fatal("the cache was not refilled with the database copy")
	}
}

// Scenario "Earlier version for an order in flight".
func TestEarlierVersionLoadsUnchanged(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	w := newWorld(t, pool)
	id1 := w.publish(t, 1, 53900)
	w.publish(t, 2, 59900)
	st := snapshot.NewStore(snapshot.DBSource{Pool: pool}, redisClient(t), nil, snapshot.Options{})
	live, err := st.Load(ctx, w.event)
	if err != nil || live.Version != 2 {
		t.Fatalf("live = %+v, %v", live, err)
	}
	old, err := st.LoadVersion(ctx, w.event, 1)
	if err != nil {
		t.Fatal(err)
	}
	if old.Version != 1 || old.SnapshotID != id1 || gaAllIn(t, old) != 55248 {
		t.Fatalf("version 1 changed: %+v", old)
	}
	// From the Redis copy too (a second store, empty in-process cache).
	other := snapshot.NewStore(snapshot.DBSource{Pool: pool}, redisClient(t), nil, snapshot.Options{})
	if again, err := other.LoadVersion(ctx, w.event, 1); err != nil || again.SnapshotID != id1 {
		t.Fatalf("version 1 from cache: %v %v", again, err)
	}
	if _, err := st.LoadVersion(ctx, w.event, 3); !errors.Is(err, snapshot.ErrNotFound) {
		t.Fatalf("unknown version: %v", err)
	}
}

// Scenario "Activation message": the core serves the new version on its next request.
func TestActivationServesNewVersionAtOnce(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	w := newWorld(t, pool)
	w.publish(t, 1, 53900)
	st := snapshot.NewStore(snapshot.DBSource{Pool: pool}, redisClient(t), nil, snapshot.Options{MemTTL: time.Hour, RedisTTL: time.Hour})
	if s, _ := st.Load(ctx, w.event); s.Version != 1 {
		t.Fatal("version 1 not live")
	}
	w.publish(t, 2, 59900)
	if s, _ := st.Load(ctx, w.event); s.Version != 1 {
		t.Fatal("expected the cached version before activation")
	}
	sha, err := st.Activate(ctx, w.org, w.event, 2)
	if err != nil || len(sha) != 64 {
		t.Fatalf("activate: %q %v", sha, err)
	}
	if s, _ := st.Load(ctx, w.event); s.Version != 2 {
		t.Fatalf("served version %d after activation", s.Version)
	}
}

// Scenario "Activation lost": without the call, every core serves the new version once its
// in-process and Redis copies expire, which the defaults bound under 10 seconds.
func TestLostActivationServedWithinBound(t *testing.T) {
	if snapshot.DefaultMemTTL+snapshot.DefaultRedisTTL >= 10*time.Second {
		t.Fatalf("default staleness %v is not under 10 s", snapshot.DefaultMemTTL+snapshot.DefaultRedisTTL)
	}
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	w := newWorld(t, pool)
	w.publish(t, 1, 53900)
	opts := snapshot.Options{MemTTL: 400 * time.Millisecond, RedisTTL: 300 * time.Millisecond}
	a := snapshot.NewStore(snapshot.DBSource{Pool: pool}, redisClient(t), nil, opts)
	b := snapshot.NewStore(snapshot.DBSource{Pool: pool}, redisClient(t), nil, opts)
	_, _ = a.Load(ctx, w.event)
	_, _ = b.Load(ctx, w.event)
	w.publish(t, 2, 59900) // committed; the activation call is lost
	deadline := time.Now().Add(opts.MemTTL + opts.RedisTTL + 200*time.Millisecond)
	for _, st := range []*snapshot.Store{a, b} {
		for {
			s, err := st.Load(ctx, w.event)
			if err != nil {
				t.Fatal(err)
			}
			if s.Version == 2 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("version 2 not served within the staleness bound")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
}

// Scenario "Activation for another organiser's event": 404.
func TestActivationForeignEventNotFound(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	w := newWorld(t, pool)
	w.publish(t, 1, 53900)
	st := snapshot.NewStore(snapshot.DBSource{Pool: pool}, nil, nil, snapshot.Options{})
	if _, err := st.Activate(ctx, uuid(), w.event, 1); !errors.Is(err, snapshot.ErrNotFound) {
		t.Fatalf("foreign organiser: %v", err)
	}
	if _, err := st.Activate(ctx, w.org, uuid(), 1); !errors.Is(err, snapshot.ErrNotFound) {
		t.Fatalf("unknown event: %v", err)
	}
	if _, err := st.Activate(ctx, w.org, w.event, 9); !errors.Is(err, snapshot.ErrNotFound) {
		t.Fatalf("unknown version: %v", err)
	}
}
