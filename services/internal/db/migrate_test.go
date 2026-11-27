package db_test

import (
	"context"
	"database/sql"
	"os"
	"testing"

	dbfiles "github.com/buzandanalexandru/eps-prototype/services/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

// Scenario "Migration order": both schemas are applied, control first, each with its own version
// record at the newest migration.
func TestMigrateAppliesBothSchemasInOrder(t *testing.T) {
	ctx := context.Background()
	pool, dsn := testdb.Migrated(t)

	wantControl, err := db.LatestVersion(os.DirFS(testdb.ControlMigrations(t)))
	if err != nil {
		t.Fatal(err)
	}
	wantSales, err := db.LatestVersion(dbfiles.Sales())
	if err != nil {
		t.Fatal(err)
	}
	gotControl, err := db.SchemaVersion(ctx, pool, db.Control)
	if err != nil {
		t.Fatal(err)
	}
	gotSales, err := db.SchemaVersion(ctx, pool, db.Sales)
	if err != nil {
		t.Fatal(err)
	}
	if gotControl != wantControl || gotSales != wantSales {
		t.Fatalf("versions control=%d sales=%d, want %d and %d", gotControl, gotSales, wantControl, wantSales)
	}
	if wantControl < db.MinControlVersion {
		t.Fatalf("MinControlVersion %d is newer than the newest control migration %d", db.MinControlVersion, wantControl)
	}

	// Separate version tables, one per schema.
	for _, tbl := range []string{"control.goose_db_version", "sales.goose_db_version"} {
		var reg *string
		if err := pool.QueryRow(ctx, "select to_regclass($1)::text", tbl).Scan(&reg); err != nil || reg == nil {
			t.Fatalf("version table %s missing (err %v)", tbl, err)
		}
	}
	if reg := ""; pool.QueryRow(ctx, "select coalesce(to_regclass('public.goose_db_version')::text, '')").Scan(&reg) == nil && reg != "" {
		t.Fatalf("goose wrote a version table to public")
	}

	// Control ran first: the sales migration depends on the roles it creates. Prove the order by
	// the applied timestamps.
	var controlFirst bool
	err = pool.QueryRow(ctx, `select (select min(tstamp) from control.goose_db_version where version_id > 0)
		<= (select min(tstamp) from sales.goose_db_version where version_id > 0)`).Scan(&controlFirst)
	if err != nil || !controlFirst {
		t.Fatalf("control migrations must run before sales (err %v)", err)
	}

	// Re-running is a no-op (forward-only, idempotent pipeline step).
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	if err := db.MigrateAll(ctx, sqlDB, os.DirFS(testdb.ControlMigrations(t)), dbfiles.Sales()); err != nil {
		t.Fatalf("second run: %v", err)
	}
}

// Migration 0001 creates NOLOGIN, NOBYPASSRLS roles that own nothing.
func TestRolesCannotLogInOrBypassRLS(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	for _, role := range []string{"eps_web", "eps_core"} {
		var canLogin, bypass, super bool
		err := pool.QueryRow(ctx, "select rolcanlogin, rolbypassrls, rolsuper from pg_roles where rolname = $1", role).
			Scan(&canLogin, &bypass, &super)
		if err != nil {
			t.Fatalf("%s: %v", role, err)
		}
		if canLogin || bypass || super {
			t.Fatalf("%s: login=%v bypassrls=%v super=%v", role, canLogin, bypass, super)
		}
		var owned int
		if err := pool.QueryRow(ctx, `select count(*) from pg_class c join pg_roles r on r.oid = c.relowner
			where r.rolname = $1`, role).Scan(&owned); err != nil || owned != 0 {
			t.Fatalf("%s owns %d relations (err %v)", role, owned, err)
		}
	}
}
