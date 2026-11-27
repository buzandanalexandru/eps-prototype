// Package testdb starts throwaway postgres:18 and redis:8 containers (testcontainers) for tests,
// one per test binary, and gives each test a fresh, migrated database.
package testdb

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	dbfiles "github.com/buzandanalexandru/eps-prototype/services/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
)

var (
	pgOnce    sync.Once
	pgAdmin   string // DSN of the container's postgres database
	pgErr     error
	pgC       *tcpostgres.PostgresContainer
	redisOnce sync.Once
	redisURL  string
	redisErr  error
	redisC    *tcredis.RedisContainer
)

// Main runs the package's tests and stops the containers afterwards.
func Main(m *testing.M) {
	code := m.Run()
	ctx := context.Background()
	if pgC != nil {
		_ = pgC.Terminate(ctx)
	}
	if redisC != nil {
		_ = redisC.Terminate(ctx)
	}
	os.Exit(code)
}

func adminDSN(t testing.TB) string {
	t.Helper()
	pgOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		pgC, pgErr = tcpostgres.Run(ctx, "postgres:18",
			tcpostgres.WithDatabase("postgres"),
			tcpostgres.WithUsername("postgres"),
			tcpostgres.WithPassword("test-password"),
			tcpostgres.BasicWaitStrategies(),
		)
		if pgErr == nil {
			pgAdmin, pgErr = pgC.ConnectionString(ctx, "sslmode=disable")
		}
	})
	if pgErr != nil {
		t.Fatalf("start postgres:18 container (Docker required): %v", pgErr)
	}
	return pgAdmin
}

func randName() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "t_" + hex.EncodeToString(b)
}

// NewDatabase creates an empty database and returns its DSN.
func NewDatabase(t testing.TB) string {
	t.Helper()
	admin := adminDSN(t)
	name := randName()
	sqlDB, err := sql.Open("pgx", admin)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if _, err := sqlDB.Exec("CREATE DATABASE " + name); err != nil {
		t.Fatal(err)
	}
	// Swap the database name in the DSN (postgres://u:p@h:port/postgres?...).
	return replaceDB(admin, name)
}

func replaceDB(dsn, name string) string {
	// The container DSN always ends in /postgres?sslmode=disable.
	const tail = "/postgres?sslmode=disable"
	return dsn[:len(dsn)-len(tail)] + "/" + name + "?sslmode=disable"
}

// ControlMigrations returns the control schema migrations from the web app's folder.
func ControlMigrations(t testing.TB) string {
	t.Helper()
	dir, _ := os.Getwd()
	for {
		p := filepath.Join(dir, "apps", "web", "db", "migrations")
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("apps/web/db/migrations not found above the working directory")
		}
		dir = parent
	}
}

// Migrated creates a fresh database with both schemas migrated and returns a pool to it
// (connected as the container superuser, i.e. the migrator; application code must use db.WithTx).
func Migrated(t testing.TB) (*pgxpool.Pool, string) {
	t.Helper()
	dsn := NewDatabase(t)
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	ctx := context.Background()
	if err := db.MigrateAll(ctx, sqlDB, os.DirFS(ControlMigrations(t)), dbfiles.Sales()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool, dsn
}

// RedisURL starts (once) a redis:8 container and returns its URL.
func RedisURL(t testing.TB) string {
	t.Helper()
	redisOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		redisC, redisErr = tcredis.Run(ctx, "redis:8")
		if redisErr == nil {
			redisURL, redisErr = redisC.ConnectionString(ctx)
		}
	})
	if redisErr != nil {
		t.Fatalf("start redis:8 container (Docker required): %v", redisErr)
	}
	return redisURL
}

