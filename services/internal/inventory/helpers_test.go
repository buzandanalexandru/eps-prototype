package inventory_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

type fixture struct {
	rdb        *redis.Client
	inv        *inventory.Sharded
	org, event string
}

// newFixture gives a flushed Redis and a loaded (empty) event.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	rdb, err := redisx.Open(testdb.RedisURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()
	if err := rdb.FlushAll(ctx).Err(); err != nil {
		t.Fatal(err)
	}
	f := &fixture{rdb: rdb, inv: &inventory.Sharded{Redis: rdb}, org: uuid.NewString(), event: uuid.NewString()}
	if err := f.inv.Rebuild(ctx, f.org, f.event, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) hold(id string, draws map[string]int64, d time.Duration) checkout.Hold {
	return checkout.Hold{OrganiserID: f.org, EventID: f.event, ID: id, Draws: draws, Deadline: time.Now().Add(d)}
}

func (f *fixture) level(t *testing.T, in string) inventory.Level {
	t.Helper()
	st, err := f.inv.Stock(context.Background(), f.org, f.event, []string{in})
	if err != nil {
		t.Fatal(err)
	}
	return st[in]
}

func (f *fixture) shards(t *testing.T, in string) []inventory.ShardState {
	t.Helper()
	s, err := f.inv.Shards(context.Background(), f.org, f.event, in)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func holdKey(f *fixture, id string) string { return inventory.HoldKey(f.org, f.event, id) }
