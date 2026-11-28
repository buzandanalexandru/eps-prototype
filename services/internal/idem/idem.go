// Package idem keeps the client idempotency keys of the core (design D-3): a request carrying an
// Idempotency-Key is answered once; a repeat with the same key and body gets the stored answer, a
// repeat with another body is refused, and a repeat while the first is still running is told to
// wait. Records live in sales.idempotency_keys for 24 hours, per organiser and scope.
package idem

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
)

// Stored is a finished request's answer.
type Stored struct {
	Status   int
	Response json.RawMessage
}

var (
	// ErrMismatch: the key was used with another request body.
	ErrMismatch = errors.New("idem: key reused with a different request")
	// ErrInProgress: the first request with this key is still running.
	ErrInProgress = errors.New("idem: request in progress")
	// ErrBadKey: the key is missing or malformed.
	ErrBadKey = errors.New("idem: Idempotency-Key must be 8 to 100 letters, digits or dashes")
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{8,100}$`)

// ValidKey reports whether a client key is acceptable.
func ValidKey(k string) bool { return keyPattern.MatchString(k) }

// Hash is the request fingerprint stored with a key.
func Hash(body any) string {
	b, _ := json.Marshal(body)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Begin claims a key. It returns the stored answer of a finished request (replay), or nil when the
// caller should run the request and then call Finish (or Abort on a failure worth retrying).
func Begin(ctx context.Context, pool *pgxpool.Pool, org, scope, key, hash string) (*Stored, error) {
	if !ValidKey(key) {
		return nil, ErrBadKey
	}
	var out *Stored
	err := db.WithTx(ctx, pool, db.RoleCore, org, func(tx pgx.Tx) error {
		// Old keys are dropped as they are met (24 h life).
		if _, err := tx.Exec(ctx, `delete from sales.idempotency_keys where organiser_id = $1 and scope = $2 and key = $3 and created_at < now() - interval '24 hours'`, org, scope, key); err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `insert into sales.idempotency_keys (organiser_id, scope, key, request_hash) values ($1, $2, $3, $4) on conflict do nothing`, org, scope, key, hash)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			return nil
		}
		var h string
		var status *int
		var resp []byte
		if err := tx.QueryRow(ctx, `select request_hash, status, response from sales.idempotency_keys where organiser_id = $1 and scope = $2 and key = $3`, org, scope, key).Scan(&h, &status, &resp); err != nil {
			return err
		}
		if h != hash {
			return ErrMismatch
		}
		if status == nil {
			// A request that never finished (its process died) stops blocking after a minute
			// (review M3): the key is taken over by this request.
			tag, err := tx.Exec(ctx, `update sales.idempotency_keys set created_at = now() where organiser_id = $1 and scope = $2 and key = $3
				and status is null and created_at < now() - interval '60 seconds'`, org, scope, key)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 1 {
				return nil
			}
			return ErrInProgress
		}
		out = &Stored{Status: *status, Response: resp}
		return nil
	})
	return out, err
}

// Finish stores the answer of a request.
func Finish(ctx context.Context, pool *pgxpool.Pool, org, scope, key string, status int, response any) error {
	b, err := json.Marshal(response)
	if err != nil {
		return err
	}
	return db.WithTx(ctx, pool, db.RoleCore, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `update sales.idempotency_keys set status = $4, response = $5 where organiser_id = $1 and scope = $2 and key = $3`, org, scope, key, status, b)
		return err
	})
}

// Abort forgets a key whose request failed in a way worth retrying.
func Abort(ctx context.Context, pool *pgxpool.Pool, org, scope, key string) {
	ctx = context.WithoutCancel(ctx)
	_ = db.WithTx(ctx, pool, db.RoleCore, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `delete from sales.idempotency_keys where organiser_id = $1 and scope = $2 and key = $3 and status is null`, org, scope, key)
		return err
	})
}
