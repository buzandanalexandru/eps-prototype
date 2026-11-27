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
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/version"
)

// Logger returns the JSON logger for a binary.
func Logger(name string) *slog.Logger {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})).
		With("service", name, "commit", version.Commit)
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
	pool, err := db.Open(ctx, dsn)
	if err != nil {
		log.Error("postgres connect failed", "host", db.HostOf(dsn), "error", err.Error())
		// Keep a lazily connecting pool so readiness recovers when the database comes back.
		cfg, perr := pgxpool.ParseConfig(dsn)
		if perr != nil {
			return nil
		}
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

// Serve runs an HTTP server on PORT (default 8080) until SIGINT/SIGTERM.
func Serve(log *slog.Logger, h http.Handler) {
	addr := ":" + Env("PORT", "8080")
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
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
	<-ctx.Done()
	log.Info("shutting down")
	shCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = srv.Shutdown(shCtx)
}
