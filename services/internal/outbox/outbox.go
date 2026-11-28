// Package outbox is the event backbone (P3, design D-12): business transactions write events into
// sales.outbox in the same transaction as the change (Write), the worker's relay publishes them to
// Redis Streams in order (Relay), and consumers read them at least once and apply each id once
// (Consumer).
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// Topics.
const (
	TopicOrders   = "orders"
	TopicPayments = "payments"
	TopicWallet   = "wallet"
)

// Event is what a business transaction records.
type Event struct {
	Topic       string
	Type        string // e.g. order.paid
	OrganiserID string
	EventID     string // the event (sale) the change belongs to, if any
	Payload     any
}

// Envelope is the common shape every consumer reads.
type Envelope struct {
	ID          string          `json:"id"`
	Type        string          `json:"type"`
	Version     int             `json:"version"`
	OrganiserID string          `json:"organiser_id"`
	EventID     string          `json:"event_id,omitempty"`
	SaleID      string          `json:"sale_id,omitempty"`
	OccurredAt  time.Time       `json:"occurred_at"`
	Payload     json.RawMessage `json:"payload"`
}

// Write records an event inside the caller's transaction (which runs under the organiser's scope,
// so row-level security refuses an event for another organiser). It returns the event id.
func Write(ctx context.Context, tx pgx.Tx, e Event) (string, error) {
	if e.Topic == "" || e.Type == "" || e.OrganiserID == "" {
		return "", errors.New("outbox: topic, type and organiser are required")
	}
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return "", err
	}
	var ev *string
	if e.EventID != "" {
		ev = &e.EventID
	}
	var id string
	err = tx.QueryRow(ctx,
		`insert into sales.outbox (organiser_id, event_id, topic, type, payload) values ($1, $2, $3, $4, $5) returning id::text`,
		e.OrganiserID, ev, e.Topic, e.Type, payload).Scan(&id)
	return id, err
}

// StreamKey is the Redis stream of a topic.
func StreamKey(topic string) string {
	k, _ := redisx.PlatformKey("stream", topic)
	return k
}

// List returns an organiser's most recent events (newest first) through row-level security: an
// organiser never sees another organiser's events.
func List(ctx context.Context, pool *pgxpool.Pool, organiserID string, limit int) ([]Envelope, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var out []Envelope
	err := db.WithTx(ctx, pool, db.RoleCore, organiserID, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select id::text, type, version, organiser_id::text, coalesce(event_id::text, ''), created_at, payload
			from sales.outbox order by seq desc limit $1`, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var e Envelope
			if err := rows.Scan(&e.ID, &e.Type, &e.Version, &e.OrganiserID, &e.EventID, &e.OccurredAt, &e.Payload); err != nil {
				return err
			}
			e.SaleID = e.EventID
			out = append(out, e)
		}
		return rows.Err()
	})
	return out, err
}
