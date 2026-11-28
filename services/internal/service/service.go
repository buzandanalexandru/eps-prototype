// Package service holds the start-up code shared by every binary: JSON logging, configuration from
// the environment, dependency connections and an HTTP server with graceful shutdown.
package service

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/health"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/version"
)

// Logger returns the JSON logger for a binary.
func Logger(name string) *slog.Logger {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	l := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).
		With("service", name, "commit", version.Commit)
	slog.SetDefault(l) // packages without a logger of their own (health) log the same JSON lines
	return l
}

// Env returns an environment variable or a default.
func Env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Postgres connects to DATABASE_URL when set. A failure is logged (without secrets) and returns
// nil, so the process stays alive and /readyz reports the problem.
func Postgres(ctx context.Context, log *slog.Logger) *pgxpool.Pool {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Warn("DATABASE_URL not set")
		return nil
	}
	maxConns := int32(4)
	if n, err := strconv.Atoi(os.Getenv("DB_MAX_CONNS")); err == nil && n > 0 && n <= 100 {
		maxConns = int32(n)
	}
	pool, err := db.Open(ctx, dsn, maxConns)
	if err != nil {
		log.Error("postgres connect failed", "host", db.HostOf(dsn), "error", err.Error())
		// Keep a lazily connecting pool so readiness recovers when the database comes back.
		cfg, perr := pgxpool.ParseConfig(dsn)
		if perr != nil {
			return nil
		}
		cfg.MaxConns = maxConns
		p, _ := pgxpool.NewWithConfig(ctx, cfg)
		return p
	}
	return pool
}

// Redis connects to REDIS_URL when set.
func Redis(log *slog.Logger) *redis.Client {
	url := os.Getenv("REDIS_URL")
	if url == "" {
		log.Warn("REDIS_URL not set")
		return nil
	}
	c, err := redisx.Open(url)
	if err != nil {
		log.Error("redis config invalid", "error", err.Error())
		return nil
	}
	return c
}

// Serve runs an HTTP server on PORT (default 8080) and the metrics listener on METRICS_PORT
// (default 9090) until SIGINT/SIGTERM. There is no server-wide WriteTimeout (it would cut
// server-sent events); every non-stream request gets a 30 s write deadline instead. On SIGTERM
// readiness turns false at once, the server keeps serving for DRAIN_SECONDS (default 3) so the
// load balancer stops sending new requests, then shuts down gracefully.
func Serve(log *slog.Logger, h http.Handler) {
	addr := ":" + Env("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           httpx.WithWriteDeadline(30*time.Second, httpx.IsEventStream)(h),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		log.Info("listening", "addr", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server failed", "error", err.Error())
			os.Exit(1)
		}
	}()
	if mp := Env("METRICS_PORT", "9090"); mp != "off" {
		mux := http.NewServeMux()
		mux.Handle("GET /metrics", metrics.Handler())
		msrv := &http.Server{Addr: ":" + mp, Handler: mux, ReadHeaderTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second}
		go func() {
			if err := msrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Warn("metrics listener failed", "error", err.Error())
			}
		}()
	}
	<-ctx.Done()
	health.SetDraining()
	drain := 3 * time.Second
	if n, err := strconv.Atoi(os.Getenv("DRAIN_SECONDS")); err == nil && n >= 0 && n <= 60 {
		drain = time.Duration(n) * time.Second
	}
	log.Info("draining", "seconds", drain.Seconds())
	time.Sleep(drain)
	log.Info("shutting down")
	shCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = srv.Shutdown(shCtx)
}
