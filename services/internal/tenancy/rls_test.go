package tenancy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func seedProbes(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	for _, org := range []string{orgA, orgB} {
		err := db.WithTx(ctx, pool, db.RoleCore, org, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, "insert into sales.probe (organiser_id, label) values ($1, $2)", org, "row of "+org)
			return err
		})
		if err != nil {
			t.Fatalf("seed %s: %v", org, err)
		}
	}
}

func pgCode(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

// Scenario "Query without a filter".
func TestFilterlessReadSeesOnlyOwnOrganiser(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	seedProbes(t, pool)
	err := db.WithTx(ctx, pool, db.RoleCore, orgA, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select organiser_id::text from sales.probe")
		if err != nil {
			return err
		}
		orgs, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return err
		}
		if len(orgs) != 1 || orgs[0] != orgA {
			t.Fatalf("scoped to A, saw %v", orgs)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Scenario "Insert for another organiser".
func TestInsertForAnotherOrganiserIsRefused(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	err := db.WithTx(ctx, pool, db.RoleCore, orgA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "insert into sales.probe (organiser_id, label) values ($1, 'smuggled')", orgB)
		return err
	})
	if pgCode(err) != "42501" { // insufficient_privilege: new row violates row-level security policy
		t.Fatalf("want RLS violation, got %v", err)
	}
}

// Scenario "Update moving a row to another organiser".
func TestUpdateToAnotherOrganiserIsRefused(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	seedProbes(t, pool)
	err := db.WithTx(ctx, pool, db.RoleCore, orgA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update sales.probe set organiser_id = $1", orgB)
		return err
	})
	if pgCode(err) != "42501" {
		t.Fatalf("want RLS violation, got %v", err)
	}
	// And A cannot touch B's row at all: the update matches nothing.
	err = db.WithTx(ctx, pool, db.RoleCore, orgA, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, "update sales.probe set label = 'hijacked' where organiser_id = $1", orgB)
		if err == nil && tag.RowsAffected() != 0 {
			t.Fatalf("updated %d rows of B", tag.RowsAffected())
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Scenario "Scope forgotten".
func TestNoScopeSeesNothingAndCannotWrite(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	seedProbes(t, pool)
	err := db.WithTx(ctx, pool, db.RoleCore, "", func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, "select count(*) from sales.probe").Scan(&n); err != nil {
			return err
		}
		if n != 0 {
			t.Fatalf("unscoped read saw %d rows", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.WithTx(ctx, pool, db.RoleCore, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "insert into sales.probe (organiser_id, label) values ($1, 'x')", orgA)
		return err
	})
	if pgCode(err) != "42501" {
		t.Fatalf("unscoped insert: want RLS violation, got %v", err)
	}
	// The scope is transaction-local: it does not leak into the next transaction on the connection.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	_ = db.WithTx(ctx, conn, db.RoleCore, orgA, func(pgx.Tx) error { return nil })
	var scope, role string
	if err := conn.QueryRow(ctx, "select coalesce(current_setting('app.organiser_id', true), ''), current_user").Scan(&scope, &role); err != nil {
		t.Fatal(err)
	}
	if scope != "" || role == string(db.RoleCore) {
		t.Fatalf("scope %q / role %q leaked out of the transaction", scope, role)
	}
}

// Scenario "Current role in a transaction" (both sides' roles).
func TestTransactionRunsAsApplicationRole(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	for _, role := range []db.Role{db.RoleCore, db.RoleWeb} {
		err := db.WithTx(ctx, pool, role, orgA, func(tx pgx.Tx) error {
			var current string
			var bypass, super bool
			var owned int
			err := tx.QueryRow(ctx, `select current_user::text, r.rolbypassrls, r.rolsuper,
				(select count(*) from pg_class c where c.relowner = r.oid)
				from pg_roles r where r.rolname = current_user`).Scan(&current, &bypass, &super, &owned)
			if err != nil {
				return err
			}
			if current != string(role) || bypass || super || owned != 0 {
				t.Fatalf("role %s: current=%s bypass=%v super=%v owns=%d", role, current, bypass, super, owned)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := db.WithTx(ctx, pool, db.Role("postgres"), "", func(pgx.Tx) error { return nil }); err == nil {
		t.Fatal("WithTx accepted a role outside the two application roles")
	}
}

// Scenarios "Core writes control" and "Web writes sales".
func TestEachSideWritesOnlyItsOwnSchema(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	seedProbes(t, pool)

	err := db.WithTx(ctx, pool, db.RoleCore, "", func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "insert into control.organisers (slug, name) values ('sneaky', 'Sneaky')")
		return err
	})
	if pgCode(err) != "42501" {
		t.Fatalf("core inserting into control: want permission error, got %v", err)
	}
	err = db.WithTx(ctx, pool, db.RoleWeb, orgA, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update sales.probe set label = 'web was here'")
		return err
	})
	if pgCode(err) != "42501" {
		t.Fatalf("web updating sales: want permission error, got %v", err)
	}
	// Granted reads still work: the web role reads sales (scoped), the core reads the registry.
	err = db.WithTx(ctx, pool, db.RoleWeb, orgA, func(tx pgx.Tx) error {
		var n int
		return tx.QueryRow(ctx, "select count(*) from sales.probe").Scan(&n)
	})
	if err != nil {
		t.Fatalf("web read of sales: %v", err)
	}
	err = db.WithTx(ctx, pool, db.RoleCore, "", func(tx pgx.Tx) error {
		var n int
		return tx.QueryRow(ctx, "select count(*) from control.organisers").Scan(&n)
	})
	if err != nil {
		t.Fatalf("core read of organisers: %v", err)
	}
	// The core reads only the public columns of the key table.
	err = db.WithTx(ctx, pool, db.RoleCore, "", func(tx pgx.Tx) error {
		var n int
		return tx.QueryRow(ctx, `select count("privateKey") from control."jwks"`).Scan(&n)
	})
	if pgCode(err) != "42501" {
		t.Fatalf("core reading private keys: want permission error, got %v", err)
	}
}

// Memberships: organiser-scoped, plus a read-only window on the caller's own rows.
func TestMembershipsIsolation(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	_, err := pool.Exec(ctx, `
		insert into control.organisers (id, slug, name) values ($1, 'org-a', 'A'), ($2, 'org-b', 'B');
		insert into control."user" (id, name, email, "emailVerified") values ('u1', 'U1', 'u1@x', true), ('u2', 'U2', 'u2@x', true);`,
		orgA, orgB)
	if err == nil {
		t.Fatal("multi-statement with args should not be allowed; use separate execs")
	}
	mustExec := func(sql string, args ...any) {
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	mustExec(`insert into control.organisers (id, slug, name) values ($1, 'org-a', 'A'), ($2, 'org-b', 'B')`, orgA, orgB)
	mustExec(`insert into control."user" (id, name, email, "emailVerified") values ('u1', 'U1', 'u1@x', true), ('u2', 'U2', 'u2@x', true)`)
	mustExec(`insert into control.memberships (organiser_id, user_id, role) values ($1, 'u1', 'owner'), ($2, 'u2', 'owner'), ($2, 'u1', 'staff')`, orgA, orgB)

	count := func(role db.Role, org, user string) int {
		var n int
		err := db.WithTx(ctx, pool, role, org, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, "select set_config('app.user_id', $1, true)", user); err != nil {
				return err
			}
			return tx.QueryRow(ctx, "select count(*) from control.memberships").Scan(&n)
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := count(db.RoleWeb, orgA, ""); n != 1 {
		t.Fatalf("scoped to A: %d memberships, want 1", n)
	}
	if n := count(db.RoleWeb, "", "u1"); n != 2 {
		t.Fatalf("user u1 own memberships: %d, want 2", n)
	}
	if n := count(db.RoleWeb, "", ""); n != 0 {
		t.Fatalf("no scope: %d memberships, want 0", n)
	}
	// The user window is read-only: u1 cannot add itself to B.
	err = db.WithTx(ctx, pool, db.RoleWeb, "", func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "select set_config('app.user_id', 'u2', true)"); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, "insert into control.memberships (organiser_id, user_id, role) values ($1, 'u2', 'owner')", orgA)
		return err
	})
	if pgCode(err) != "42501" {
		t.Fatalf("user-window insert: want RLS violation, got %v", err)
	}
}
