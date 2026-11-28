// Package checkout holds stock for an order, keeps the order's state machine and prices it from the
// sale snapshot (step 4: D2, D3, D7, D8, D9, D11; O2, O3). Stock lives behind the Inventory
// interface; step 5's sharded engine (internal/inventory) implements it (step 4's single-counter
// engine was removed in step 5's review).
package checkout

import (
	"context"
	"errors"
	"time"
)

// Hold is stock taken for one order: units per pool instance until a deadline.
type Hold struct {
	OrganiserID string
	EventID     string
	ID          string // the order id (or <order>_r<n> for a re-hold after a late payment)
	Draws       map[string]int64
	Deadline    time.Time
}

// Inventory takes, extends, commits and releases holds. Implementations must be all-or-nothing
// across every instance of a hold and must never take an instance above the capacity passed in.
type Inventory interface {
	// Reserve takes the hold's units; ErrShort names the first instance without room and
	// ErrNotReady means the counters must be rebuilt from Postgres first.
	Reserve(ctx context.Context, h Hold, capacity map[string]int64) error
	// Extend moves a live hold's deadline (a payment in progress).
	Extend(ctx context.Context, org, event, holdID string, deadline time.Time) error
	// Commit turns a live hold into sold units, never taking sold units above capacity.
	Commit(ctx context.Context, org, event, holdID string, capacity map[string]int64) error
	// Release gives a live hold's units back and says what the hold was: released now, already
	// committed (a payment won), or not live (released before, or unknown).
	Release(ctx context.Context, org, event, holdID string) (ReleaseResult, error)
	// ReleaseMany releases several holds of one event in one step (the sweeper's batches).
	ReleaseMany(ctx context.Context, org, event string, holdIDs []string) error
	// HoldState is "held", "committed", "released", "returned" or "" (unknown).
	HoldState(ctx context.Context, org, event, holdID string) (string, error)
	// Unsell gives a committed hold's sold units back (a defaulted instalment plan).
	Unsell(ctx context.Context, org, event, holdID string) error
	// Used is units sold plus held per instance (for availability and the publish gate).
	Used(ctx context.Context, org, event string, instances []string) (map[string]int64, error)
	// Rebuild sets the counters and live holds of an event from the record of truth, unless they
	// are already present.
	Rebuild(ctx context.Context, org, event string, used, sold map[string]int64, holds []Hold) error
	// Lock serialises rebuilds of one event across cores; it reports whether this caller got it.
	Lock(ctx context.Context, org, event string, ttl time.Duration) (bool, func(), error)
}

// HoldIndex finds holds the orders no longer own (the orphan sweep).
type HoldIndex interface {
	// Due returns hold ids of an event whose deadline passed before `before`.
	Due(ctx context.Context, org, event string, before time.Time, limit int64) ([]string, error)
	// Events lists the organiser/event pairs that ever held stock.
	Events(ctx context.Context) ([][2]string, error)
	// Forget drops a hold from the index.
	Forget(ctx context.Context, org, event, holdID string) error
}

// ReleaseResult is what Release found.
type ReleaseResult int

// Release results.
const (
	ReleaseNotLive ReleaseResult = iota
	ReleaseDone
	ReleaseCommitted
)

// ErrShort means an instance had no room (or a commit would sell above capacity).
type ErrShort struct{ Instance string }

func (e *ErrShort) Error() string { return "checkout: no room in " + e.Instance }

// ErrNotReady means the event's counters are not in Redis (first use, or Redis restarted).
var ErrNotReady = errors.New("checkout: inventory counters not loaded")

// ErrNoHold means the hold is not live (released, committed or unknown).
var ErrNoHold = errors.New("checkout: hold not live")

