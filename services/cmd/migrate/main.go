// Command migrate applies the forward-only migrations of both schemas, control first, then sales
// (design D0.2). Only the pipeline runs it. It reads DATABASE_URL (use the unpooled URL on Neon)
// and never prints it.
//
//	go run ./cmd/migrate -control ../apps/web/db/migrations
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	dbfiles "github.com/buzandanalexandru/eps-prototype/services/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/service"
)

func main() {
	controlDir := flag.String("control", "../apps/web/db/migrations", "directory of control schema migrations")
	statusOnly := flag.Bool("status", false, "print the applied versions and exit")
	flag.Parse()
	log := service.Logger("migrate")

	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Error("DATABASE_URL is not set")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Error("open failed", "host", db.HostOf(dsn), "error", db.Redact(err.Error()))
		os.Exit(1)
	}
	defer sqlDB.Close()
	if err := sqlDB.PingContext(ctx); err != nil {
		log.Error("connect failed", "host", db.HostOf(dsn), "error", db.Redact(err.Error()))
		os.Exit(1)
	}
	if !*statusOnly {
		if err := db.MigrateAll(ctx, sqlDB, os.DirFS(*controlDir), dbfiles.Sales()); err != nil {
			log.Error("migration failed", "error", db.Redact(err.Error()))
			os.Exit(1)
		}
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Error("connect failed", "host", db.HostOf(dsn), "error", db.Redact(err.Error()))
		os.Exit(1)
	}
	defer pool.Close()
	c, _ := db.SchemaVersion(ctx, pool, db.Control)
	s, _ := db.SchemaVersion(ctx, pool, db.Sales)
	log.Info("schema versions", "control", c, "sales", s)
	fmt.Printf("control=%d sales=%d\n", c, s)
}
