package snapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
)

// RehearsalSource answers for rehearsal events (step 6, design D-1): a rehearsal has an event id of
// its own, so the core loads the source event's pinned version and serves it as version 1 of the
// rehearsal event (eventId rewritten, hash recomputed). Any other event goes to Inner unchanged.
// The mapping comes from sales.rehearsal_source(), a SECURITY DEFINER function of eps_sales_index.
type RehearsalSource struct {
	Inner Source
	Pool  *pgxpool.Pool
}

// RehearsalVersion is the only version a rehearsal event has.
const RehearsalVersion = 1

func (r RehearsalSource) Fetch(ctx context.Context, eventID string, version int) (Stored, error) {
	if r.Pool == nil {
		return r.Inner.Fetch(ctx, eventID, version)
	}
	// One transaction (one connection) for the mapping and the document: a snapshot load can run
	// while its caller holds a connection of the same pool (the checkout's paid transaction).
	var out Stored
	err := db.WithTx(ctx, r.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		var org, src string
		var pinned int
		err := tx.QueryRow(ctx, "select organiser_id::text, source_event::text, version from sales.rehearsal_source($1::uuid)", eventID).Scan(&org, &src, &pinned)
		if errors.Is(err, pgx.ErrNoRows) {
			out, err = fetchTx(ctx, tx, eventID, version)
			return err
		}
		if err != nil {
			return err
		}
		if version != 0 && version != RehearsalVersion {
			return ErrNotFound
		}
		st, err := fetchTx(ctx, tx, src, pinned)
		if err != nil {
			return err
		}
		if st.OrganiserID != org {
			return ErrNotFound
		}
		doc, err := Rehearse(st.Doc, eventID)
		if err != nil {
			return err
		}
		out = Stored{OrganiserID: org, Version: RehearsalVersion, Doc: doc, SHA256: hashOf(doc)}
		return nil
	})
	return out, err
}

// Rehearse rewrites a snapshot document as version 1 of a rehearsal event, keeping every other
// field (numbers kept exactly) and marking it a rehearsal.
func Rehearse(doc []byte, eventID string) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, err
	}
	m["eventId"] = eventID
	m["version"] = RehearsalVersion
	m["rehearsal"] = true
	return json.Marshal(m)
}
