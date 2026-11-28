// Package db opens Postgres pools and runs every application transaction under an application role
// with the organiser scope set (P2, U2). It is the only place where SET LOCAL ROLE happens.
package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Role is a database application role. Only the constants below exist, so the role name is never
// built from input.
type Role string

const (
	// RoleCore is the Go core's role: writes the sales schema, reads granted control tables.
	RoleCore Role = "eps_core"
	// RoleWeb is the web app's role (used by Go tests to prove the web side cannot write sales).
	RoleWeb Role = "eps_web"
)

func (r Role) valid() bool { return r == RoleCore || r == RoleWeb }

// Open connects a pool of at most maxConns connections (0 keeps pgx's default); errors never
// contain the password or the full connection string. The bound matters: every pod of every
// service holds its own pool against one Neon compute (design D0.2).
func Open(ctx context.Context, dsn string, maxConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, &SafeError{Host: HostOf(dsn), Err: errors.New("invalid connection string")}
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	cfg.MaxConnIdleTime = 2 * time.Minute
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, &SafeError{Host: HostOf(dsn), Err: err}
	}
	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, &SafeError{Host: HostOf(dsn), Err: err}
	}
	return pool, nil
}

// Beginner is satisfied by *pgxpool.Pool and *pgx.Conn.
type Beginner interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WithTx runs fn in a transaction under role, scoped to organiserID ("" means no organiser scope:
// organiser tables then show nothing and refuse writes). It commits when fn returns nil.
func WithTx(ctx context.Context, b Beginner, role Role, organiserID string, fn func(pgx.Tx) error) (err error) {
	if !role.valid() {
		return fmt.Errorf("db: unknown role %q", role)
	}
	tx, err := b.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		// A panic in fn rolls back before it continues up the stack, so the connection goes back
		// to the pool clean instead of carrying an open transaction under the application role.
		if p := recover(); p != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	// Role and organiser scope in one round trip (set_config('role', ..., true) is SET LOCAL ROLE);
	// the role is one of the constants above.
	if _, err = tx.Exec(ctx, "select set_config('role', $1, true), set_config('app.organiser_id', $2, true)", string(role), organiserID); err != nil {
		return fmt.Errorf("db: set role and organiser scope: %w", err)
	}
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
