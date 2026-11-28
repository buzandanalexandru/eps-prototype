package health

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
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

// Scenario "Redis unreachable" (step 6): readiness fails, so the instance gets no traffic, and the
// log shows the failing dependency with the request id.
func TestRedisUnreachableLogsWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, MaxRetries: -1})
	mux := http.NewServeMux()
	Handlers(mux, []Checker{Redis(rdb)})
	h := httpx.WithRequestID(mux)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz %d", rec.Code)
	}
	id := rec.Header().Get("X-Request-Id")
	var line map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &line); err != nil {
		t.Fatalf("log %q: %v", buf.String(), err)
	}
	if id == "" || line["requestId"] != id || !strings.Contains(fmt.Sprint(line["failing"]), "redis") {
		t.Fatalf("log line %v, request id %q", line, id)
	}
}
