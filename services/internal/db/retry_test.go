package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsLockConflict(t *testing.T) {
	for _, c := range []struct {
		err  error
		want bool
	}{
		{&pgconn.PgError{Code: "55P03"}, true},
		{&pgconn.PgError{Code: "40P01"}, true},
		{fmt.Errorf("migrate sales: failed to run SQL migration: %w", &pgconn.PgError{Code: "55P03"}), true},
		{errors.New("ERROR: deadlock detected (SQLSTATE 40P01)"), true},
		{&pgconn.PgError{Code: "23505"}, false},
		{errors.New("syntax error"), false},
		{nil, false},
	} {
		if got := IsLockConflict(c.err); got != c.want {
			t.Errorf("IsLockConflict(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

// A migration that meets a lock timeout or a deadlock is run again with backoff; another error is
// returned at once; the number of tries is bounded.
func TestRetryOnLockConflict(t *testing.T) {
	ctx := context.Background()
	calls := 0
	var waits []time.Duration
	err := Retry(ctx, 5, time.Millisecond, func(_ int, _ error, w time.Duration) { waits = append(waits, w) }, func() error {
		calls++
		switch calls {
		case 1:
			return fmt.Errorf("wrapped: %w", &pgconn.PgError{Code: SQLStateLockNotAvailable})
		case 2:
			return &pgconn.PgError{Code: SQLStateDeadlock}
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Fatalf("err %v after %d calls, want success after 3", err, calls)
	}
	if len(waits) != 2 || waits[1] < 2*time.Millisecond {
		t.Fatalf("backoff %v, want two growing waits", waits)
	}

	calls = 0
	other := errors.New("relation does not exist")
	if err := Retry(ctx, 5, time.Millisecond, nil, func() error { calls++; return other }); !errors.Is(err, other) || calls != 1 {
		t.Fatalf("non-lock error: %v after %d calls, want it at once", err, calls)
	}

	calls = 0
	if err := Retry(ctx, 3, time.Millisecond, nil, func() error { calls++; return &pgconn.PgError{Code: SQLStateLockNotAvailable} }); !IsLockConflict(err) || calls != 3 {
		t.Fatalf("persistent conflict: %v after %d calls, want 3 tries then the error", err, calls)
	}
}
