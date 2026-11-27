package health

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
)

// A binary started without DATABASE_URL or REDIS_URL stays alive and reports them as not ready.
func TestUnconfiguredDependenciesAreNotReady(t *testing.T) {
	var pool *pgxpool.Pool
	var rdb *redis.Client
	res, ok := Ready(context.Background(), []Checker{Postgres(pool), Redis(rdb), SchemaAtLeast(pool, db.Sales, 1),
		CheckFunc(func(context.Context) api.Check { panic("boom") })}, false)
	if ok || len(res.Checks) != 4 {
		t.Fatalf("%+v", res)
	}
}
