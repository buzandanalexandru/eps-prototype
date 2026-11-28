package outbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

const (
	orgA = "11111111-1111-4111-8111-111111111111"
	orgB = "22222222-2222-4222-8222-222222222222"
)

func setup(t *testing.T) (*pgxpool.Pool, *redis.Client) {
	t.Helper()
	pool, _ := testdb.Migrated(t)
	rdb, err := redisx.Open(testdb.RedisURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	_ = rdb.FlushAll(context.Background()).Err()
	return pool, rdb
}

func count(t *testing.T, pool *pgxpool.Pool, org, typ string) int {
	t.Helper()
	var n int
	err := db.WithTx(context.Background(), pool, db.RoleCore, org, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), "select count(*) from sales.outbox where type = $1", typ).Scan(&n)
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// Scenario "Rolled-back change has no event".
func TestRolledBackChangeHasNoEvent(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	boom := errors.New("payment transaction failed")
	err := db.WithTx(ctx, pool, db.RoleCore, orgA, func(tx pgx.Tx) error {
		if _, err := outbox.Write(ctx, tx, outbox.Event{Topic: outbox.TopicOrders, Type: "order.paid", OrganiserID: orgA, Payload: map[string]string{"orderId": "o1"}}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	if n := count(t, pool, orgA, "order.paid"); n != 0 {
		t.Fatalf("%d events after a rollback", n)
	}
}

// Scenario "Committed change has an event".
func TestCommittedChangeHasOneEvent(t *testing.T) {
	pool, _ := setup(t)
	ctx := context.Background()
	err := db.WithTx(ctx, pool, db.RoleCore, orgA, func(tx pgx.Tx) error {
		_, err := outbox.Write(ctx, tx, outbox.Event{Topic: outbox.TopicOrders, Type: "order.paid", OrganiserID: orgA, Payload: map[string]string{"orderId": "o1"}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if n := count(t, pool, orgA, "order.paid"); n != 1 {
		t.Fatalf("%d events, want 1", n)
	}
	// An event for another organiser cannot be written under A's scope.
	err = db.WithTx(ctx, pool, db.RoleCore, orgA, func(tx pgx.Tx) error {
		_, err := outbox.Write(ctx, tx, outbox.Event{Topic: outbox.TopicOrders, Type: "order.paid", OrganiserID: orgB, Payload: map[string]string{}})
		return err
	})
	if err == nil {
		t.Fatal("wrote an event for another organiser")
	}
}
