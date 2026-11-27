// Command migrate applies the forward-only migrations of both schemas, control first, then sales
// (design D0.2). Only the pipeline runs it, as the branch owner. It reads DATABASE_URL (use the
// unpooled URL on Neon) and never prints it.
//
//	go run ./cmd/migrate -control ../apps/web/db/migrations            migrate both schemas
//	go run ./cmd/migrate -status                                        print the versions
//	go run ./cmd/migrate -assert-empty ...                              also fail if user data exists
//	go run ./cmd/migrate -login core -out FILE                          set a login role's password
//
// -assert-empty fails when control's user, session or account tables hold any row: a preview
// branch must start with no personal data (it is created schema-only, never from production).
//
// -login web|core sets the password of eps_web_login / eps_core_login (EPS_LOGIN_PASSWORD when set,
// else a new random one) and writes url=, pooled_url= (from DATABASE_URL_POOLED, when set) and
// password= lines to FILE for the pipeline to mask and store. It never prints them.
package main

import (
	"context"
	"database/sql"
	"errors"
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
	assertEmpty := flag.Bool("assert-empty", false, "fail if the branch holds any user, session or account row")
	login := flag.String("login", "", "web or core: set that login role's password instead of migrating")
	out := flag.String("out", "", "with -login: file to write url=, pooled_url= and password= to")
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
	if *login != "" {
		if err := setLogin(ctx, sqlDB, dsn, *login, *out); err != nil {
			log.Error("login role", "error", db.Redact(err.Error()))
			os.Exit(1)
		}
		log.Info("login role password set", "side", *login)
		return
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
	if *assertEmpty {
		if err := assertNoUserData(ctx, sqlDB); err != nil {
			log.Error("preview branch holds user data", "error", err.Error())
			os.Exit(1)
		}
		log.Info("no user, session or account rows")
	}
}

func assertNoUserData(ctx context.Context, sqlDB *sql.DB) error {
	for _, tbl := range []string{`control."user"`, `control."session"`, `control."account"`} {
		var n int64
		if err := sqlDB.QueryRowContext(ctx, "select count(*) from "+tbl).Scan(&n); err != nil {
			return fmt.Errorf("count %s: %s", tbl, db.Redact(err.Error()))
		}
		if n != 0 {
			return fmt.Errorf("%s has %d rows; a preview branch must start empty", tbl, n)
		}
	}
	return nil
}

func setLogin(ctx context.Context, sqlDB *sql.DB, dsn, side, out string) error {
	role, err := db.LoginRoleFor(side)
	if err != nil {
		return err
	}
	if out == "" {
		return errors.New("-login needs -out")
	}
	pw := os.Getenv("EPS_LOGIN_PASSWORD")
	if pw == "" {
		if pw, err = db.NewPassword(); err != nil {
			return err
		}
	}
	if err := db.SetLoginPassword(ctx, sqlDB, role, pw); err != nil {
		return err
	}
	direct, err := db.LoginURL(dsn, role, pw)
	if err != nil {
		return err
	}
	lines := "url=" + direct + "\npassword=" + pw + "\n"
	if pooled := os.Getenv("DATABASE_URL_POOLED"); pooled != "" {
		p, err := db.LoginURL(pooled, role, pw)
		if err != nil {
			return err
		}
		lines += "pooled_url=" + p + "\n"
	}
	return os.WriteFile(out, []byte(lines), 0o600)
}
