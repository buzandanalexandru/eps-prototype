package db_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

// The services connect as eps_core_login / eps_web_login: they log in, cannot bypass row-level
// security, own nothing, inherit only their application role's grants and can still read the
// schema versions on a raw pool connection (no owner rights needed).
func TestLoginRolesAreRLSBoundAndNeedNoOwnerRights(t *testing.T) {
	ctx := context.Background()
	owner, dsn := testdb.Migrated(t)
	for _, role := range []db.LoginRole{db.LoginWeb, db.LoginCore} {
		var canLogin, bypass, super, createRole bool
		if err := owner.QueryRow(ctx, "select rolcanlogin, rolbypassrls, rolsuper, rolcreaterole from pg_roles where rolname = $1",
			string(role)).Scan(&canLogin, &bypass, &super, &createRole); err != nil {
			t.Fatal(err)
		}
		if !canLogin || bypass || super || createRole {
			t.Fatalf("%s: login=%v bypassrls=%v super=%v createrole=%v", role, canLogin, bypass, super, createRole)
		}
	}

	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	pw, err := db.NewPassword()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetLoginPassword(ctx, sqlDB, db.LoginCore, pw); err != nil {
		t.Fatal(err)
	}
	if err := db.SetLoginPassword(ctx, sqlDB, db.LoginCore, "short'; drop role x; --"); err == nil {
		t.Fatal("unsafe password accepted")
	}
	url, err := db.LoginURL(dsn, db.LoginCore, pw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(url, "test-password") {
		t.Fatal("login URL still carries the owner's password")
	}
	core, err := db.Open(ctx, url, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()

	var user string
	if err := core.QueryRow(ctx, "select current_user").Scan(&user); err != nil || user != string(db.LoginCore) {
		t.Fatalf("current_user %q (%v)", user, err)
	}
	for _, s := range []db.Schema{db.Control, db.Sales} {
		if v, err := db.SchemaVersion(ctx, core, s); err != nil || v == 0 {
			t.Fatalf("schema version %s as login role: %d, %v", s, v, err)
		}
	}
	// Transactions switch to eps_core as usual.
	err = db.WithTx(ctx, core, db.RoleCore, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select current_user").Scan(&user)
	})
	if err != nil || user != "eps_core" {
		t.Fatalf("role in transaction %q (%v)", user, err)
	}
	// Cannot write control, cannot read private keys, cannot become the web role.
	if _, err := core.Exec(ctx, "insert into control.organisers (slug, name) values ('x', 'x')"); err == nil {
		t.Fatal("core login wrote control.organisers")
	}
	if _, err := core.Exec(ctx, `select "privateKey" from control."jwks"`); err == nil {
		t.Fatal("core login read private keys")
	}
	if _, err := core.Exec(ctx, "set role eps_web"); err == nil {
		t.Fatal("core login switched to eps_web")
	}
	// Owns no relation.
	var owned int
	if err := owner.QueryRow(ctx, `select count(*) from pg_class c join pg_roles r on r.oid = c.relowner
		where r.rolname in ('eps_web_login', 'eps_core_login')`).Scan(&owned); err != nil || owned != 0 {
		t.Fatalf("login roles own %d relations (%v)", owned, err)
	}
}

// A panic inside WithTx rolls the transaction back and continues up the stack; the pool stays
// usable and nothing from the panicking transaction was committed.
func TestWithTxRollsBackOnPanic(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	small, err := pgxpool.New(ctx, pool.Config().ConnString()+"&pool_max_conns=1")
	if err != nil {
		t.Fatal(err)
	}
	defer small.Close()
	var pid uint32
	func() {
		defer func() {
			if r := recover(); r != "boom" {
				t.Fatalf("panic not re-raised: %v", r)
			}
		}()
		_ = db.WithTx(ctx, small, db.RoleWeb, "", func(tx pgx.Tx) error {
			pid = tx.Conn().PgConn().PID()
			if _, err := tx.Exec(ctx, "insert into control.organisers (slug, name) values ('panic-org', 'P')"); err != nil {
				return err
			}
			panic("boom")
		})
	}()
	// The same connection is back in the pool (rolled back, not thrown away), idle and under its
	// login user again.
	c, err := small.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var user string
	if err := c.QueryRow(ctx, "select current_user").Scan(&user); err != nil {
		t.Fatal(err)
	}
	if c.Conn().PgConn().PID() != pid || c.Conn().PgConn().TxStatus() != 'I' || user == "eps_web" {
		t.Fatalf("connection after panic: pid %d (was %d), tx status %c, user %s",
			c.Conn().PgConn().PID(), pid, c.Conn().PgConn().TxStatus(), user)
	}
	c.Release()
	var n int
	if err := pool.QueryRow(ctx, "select count(*) from control.organisers where slug = 'panic-org'").Scan(&n); err != nil || n != 0 {
		t.Fatalf("row from the panicking transaction committed (%d, %v)", n, err)
	}
	// And a returned error still rolls back.
	sentinel := errors.New("no")
	if err := db.WithTx(ctx, small, db.RoleWeb, "", func(pgx.Tx) error { return sentinel }); !errors.Is(err, sentinel) {
		t.Fatalf("error not returned: %v", err)
	}
}
