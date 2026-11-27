package health_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"


	dbfiles "github.com/buzandanalexandru/eps-prototype/services/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/auth"
	"github.com/buzandanalexandru/eps-prototype/services/internal/coreapi"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }


func coreHandler(t *testing.T, checks []health.Checker) http.Handler {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	h, err := coreapi.NewHandler(&coreapi.Server{Service: "core", Checks: checks}, coreapi.Options{
		Log: log, Verifier: auth.NewVerifier(nil), CORS: httpx.NewCORS(""),
	})
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func get(t *testing.T, h http.Handler, path string) (int, api.Readiness) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var r api.Readiness
	_ = json.Unmarshal(rec.Body.Bytes(), &r)
	return rec.Code, r
}

func find(r api.Readiness, name string) *api.Check {
	for i := range r.Checks {
		if r.Checks[i].Name == name {
			return &r.Checks[i]
		}
	}
	return nil
}

func checksFor(t *testing.T, redisURL string) ([]health.Checker, func(int64)) {
	t.Helper()
	pool, _ := testdb.Migrated(t)
	rdb, err := redisx.Open(redisURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	want, _ := db.LatestVersion(dbfiles.Sales())
	checks := []health.Checker{
		health.Postgres(pool),
		health.Redis(rdb),
		health.SchemaAtLeast(pool, db.Sales, want, true),
		health.SchemaAtLeast(pool, db.Control, db.MinControlVersion, false),
	}
	rollback := func(v int64) {
		if _, err := pool.Exec(context.Background(), "delete from sales.goose_db_version where version_id > $1", v); err != nil {
			t.Fatal(err)
		}
	}
	return checks, rollback
}

func TestReadyWhenEverythingAnswers(t *testing.T) {
	checks, _ := checksFor(t, testdb.RedisURL(t))
	for _, h := range []http.Handler{coreHandler(t, checks), simpleHandler(checks)} {
		code, r := get(t, h, "/readyz?verbose=true")
		if code != http.StatusOK || r.Status != "ready" || len(r.Checks) != 4 {
			t.Fatalf("readyz %d %+v", code, r)
		}
	}
}

// Scenario "Redis down".
func TestRedisDownFailsReadinessButNotLiveness(t *testing.T) {
	checks, _ := checksFor(t, "redis://127.0.0.1:1/0")
	for _, h := range []http.Handler{coreHandler(t, checks), simpleHandler(checks)} {
		code, r := get(t, h, "/readyz")
		if code != http.StatusServiceUnavailable || r.Status != "not_ready" {
			t.Fatalf("readyz %d %+v", code, r)
		}
		if c := find(r, "redis"); c == nil || c.Ok {
			t.Fatalf("redis not named as failing: %+v", r.Checks)
		}
		if c := find(r, "postgres"); c != nil {
			t.Fatalf("healthy postgres listed without verbose: %+v", c)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("healthz %d", rec.Code)
		}
	}
}

// Scenario "Service started against an older schema".
func TestSchemaBehindFailsWithVersions(t *testing.T) {
	checks, rollback := checksFor(t, testdb.RedisURL(t))
	want, _ := db.LatestVersion(dbfiles.Sales())
	rollback(want - 1)
	code, r := get(t, coreHandler(t, checks), "/readyz")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("readyz %d", code)
	}
	c := find(r, "schema:sales")
	if c == nil || c.Ok || c.Expected == nil || c.Actual == nil || *c.Expected != want || *c.Actual != want-1 {
		t.Fatalf("schema check %+v", c)
	}
}

func simpleHandler(checks []health.Checker) http.Handler {
	mux := http.NewServeMux()
	health.Handlers(mux, checks)
	return mux
}

