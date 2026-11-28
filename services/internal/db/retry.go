package db

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// Lock conflicts a migration may hit against a live system (step 6's production deploy
// deadlocked against the worker): a lock wait past the migration's lock_timeout, or a deadlock.
const (
	SQLStateLockNotAvailable = "55P03"
	SQLStateDeadlock         = "40P01"
)

// IsLockConflict reports whether err is a lock timeout or a deadlock (possibly wrapped, as goose
// wraps a failed migration's error).
func IsLockConflict(err error) bool {
	if err == nil {
		return false
	}
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code == SQLStateLockNotAvailable || pe.Code == SQLStateDeadlock
	}
	s := err.Error()
	return strings.Contains(s, "SQLSTATE "+SQLStateLockNotAvailable) || strings.Contains(s, "SQLSTATE "+SQLStateDeadlock)
}

// Retry runs f up to attempts times while it fails with a lock conflict, waiting base, 2·base,
// 4·base ... (with up to 50% jitter) between tries. Any other error, or success, returns at once.
// A migration that failed rolled back as a whole (goose runs each one in a transaction), so it is
// safe to run again.
func Retry(ctx context.Context, attempts int, base time.Duration, onRetry func(try int, err error, wait time.Duration), f func() error) error {
	if attempts < 1 {
		attempts = 1
	}
	var err error
	for try := 1; ; try++ {
		if err = f(); err == nil || !IsLockConflict(err) || try >= attempts {
			return err
		}
		wait := base << (try - 1)
		wait += time.Duration(rand.Int64N(int64(wait/2) + 1))
		if onRetry != nil {
			onRetry(try, err, wait)
		}
		select {
		case <-ctx.Done():
			return err
		case <-time.After(wait):
		}
	}
}

// MigrateAllRetry is MigrateAll retried on lock conflicts (up to attempts tries, 2 s backoff
// doubling): what the deploy's migrate step runs against a live database.
func MigrateAllRetry(ctx context.Context, sqlDB *sql.DB, control, sales fs.FS, attempts int, onRetry func(try int, err error, wait time.Duration)) error {
	return Retry(ctx, attempts, 2*time.Second, onRetry, func() error { return MigrateAll(ctx, sqlDB, control, sales) })
}
