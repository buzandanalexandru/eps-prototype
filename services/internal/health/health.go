// Package health implements liveness and readiness for every EPS binary. Liveness never touches a
// dependency; readiness fails (503) naming each failing check.
package health

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/api"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
)

// Checker is one readiness check.
type Checker interface {
	Check(ctx context.Context) api.Check
}

// CheckFunc adapts a function to Checker.
type CheckFunc func(ctx context.Context) api.Check

func (f CheckFunc) Check(ctx context.Context) api.Check { return f(ctx) }

func fail(name string, detail string) api.Check {
	return api.Check{Name: name, Ok: false, Detail: &detail}
}

// Postgres checks that the pool answers.
func Postgres(pool *pgxpool.Pool) Checker {
	return CheckFunc(func(ctx context.Context) api.Check {
		if pool == nil {
			return fail("postgres", "not configured")
		}
		if err := pool.Ping(ctx); err != nil {
			return fail("postgres", db.Redact(err.Error()))
		}
		return api.Check{Name: "postgres", Ok: true}
	})
}

// Redis checks that Redis answers PING.
func Redis(client *redis.Client) Checker {
	return CheckFunc(func(ctx context.Context) api.Check {
		if client == nil {
			return fail("redis", "not configured")
		}
		if err := client.Ping(ctx).Err(); err != nil {
			return fail("redis", err.Error())
		}
		return api.Check{Name: "redis", Ok: true}
	})
}

// SchemaAtLeast checks that a schema's applied version is at least want. A newer schema is ready:
// migrations are forward-only and backward compatible (expand, then contract in a later release),
// so the previous release keeps working while a new one rolls out, and a rollback never needs a
// down migration.
func SchemaAtLeast(pool *pgxpool.Pool, schema db.Schema, want int64) Checker {
	name := "schema:" + string(schema)
	return CheckFunc(func(ctx context.Context) api.Check {
		if pool == nil {
			return fail(name, "postgres not configured")
		}
		got, err := db.SchemaVersion(ctx, pool, schema)
		if err != nil {
			return fail(name, db.Redact(err.Error()))
		}
		c := api.Check{Name: name, Expected: &want, Actual: &got, Ok: got >= want}
		if !c.Ok {
			d := fmt.Sprintf("schema %s is at version %d, expected at least %d", schema, got, want)
			c.Detail = &d
		}
		return c
	})
}

var draining atomic.Bool

// SetDraining makes every readiness check fail from now on (the process is shutting down).
func SetDraining() { draining.Store(true) }

// ResetDrainingForTest clears the draining state (tests only).
func ResetDrainingForTest() { draining.Store(false) }

// Ready runs all checks with a deadline and reports whether all passed.
func Ready(ctx context.Context, checks []Checker, verbose bool) (api.Readiness, bool) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out := api.Readiness{Status: "ready", Checks: []api.Check{}}
	ok := true
	results := make([]api.Check, len(checks))
	done := make(chan struct{}, len(checks))
	for i, c := range checks {
		go func() {
			defer func() {
				if v := recover(); v != nil {
					results[i] = fail("check", fmt.Sprint("check panicked: ", v))
				}
				done <- struct{}{}
			}()
			results[i] = c.Check(ctx)
		}()
	}
	for range checks {
		<-done
	}
	for _, r := range results {
		if !r.Ok {
			ok = false
		}
		if !r.Ok || verbose {
			out.Checks = append(out.Checks, r)
		}
	}
	if draining.Load() {
		ok = false
		out.Checks = append(out.Checks, fail("draining", "shutting down"))
	}
	if !ok {
		out.Status = "not_ready"
	}
	return out, ok
}

// Handlers serves /healthz and /readyz for binaries that are not generated from the contract
// (the core serves them through its generated handler, with the same shapes).
func Handlers(mux *http.ServeMux, checks []Checker) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, api.Health{Status: "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		res, ok := Ready(r.Context(), checks, r.URL.Query().Get("verbose") == "true")
		status := http.StatusOK
		if !ok {
			status = http.StatusServiceUnavailable
		}
		httpx.WriteJSON(w, status, res)
	})
}
