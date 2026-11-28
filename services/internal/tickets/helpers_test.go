package tickets_test

import (
	"context"
	"crypto/ed25519"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wallet"
)

func TestMain(m *testing.M) { testdb.Main(m) }

type env struct {
	*salestest.Env
	sale   *salestest.Sale
	svc    *tickets.Service
	wallet *wallet.Service
	now    time.Time
}

func setup(t *testing.T) *env {
	t.Helper()
	e := salestest.Setup(t)
	x := &env{Env: e, now: time.Now()}
	x.sale = e.NewSale(t, 100, 100)
	x.wallet = &wallet.Service{Pool: e.Pool, Redis: e.Redis, Checkout: e.Svc}
	x.svc = &tickets.Service{Pool: e.Pool, Snaps: e.Store, Keys: tickets.NewKeyRing(1, vecKey()), Links: e.Svc.Links, Redis: e.Redis, Log: e.Log,
		Pay: x.wallet.PayFeeTx, IdentitySecret: []byte("identity-secret"), Now: func() time.Time { return x.now }}
	return x
}

// paid holds and pays an order (without issuing it).
func (x *env) paid(t *testing.T, b checkout.Buyer, items ...checkout.Item) *checkout.Order {
	t.Helper()
	o := x.sale.Hold(t, b, items...)
	out, err := x.Svc.Complete(context.Background(), x.sale.Org, o.ID, "pay-"+o.ID[:8], "test payment", nil)
	if err != nil || !out.Paid {
		t.Fatalf("pay: %+v %v", out, err)
	}
	return o
}

func (x *env) issued(t *testing.T, b checkout.Buyer, items ...checkout.Item) (*checkout.Order, []*tickets.Ticket) {
	t.Helper()
	o := x.paid(t, b, items...)
	if _, err := x.svc.Issue(context.Background(), x.sale.Org, o.ID); err != nil {
		t.Fatal(err)
	}
	return o, x.ticketsOf(t, x.sale.Org, o.ID)
}

func (x *env) ticketsOf(t *testing.T, org, orderID string) []*tickets.Ticket {
	t.Helper()
	var out []*tickets.Ticket
	err := db.WithTx(context.Background(), x.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `select id::text, generation, state, holder_id, product_id::text, event_id::text, order_id::text from sales.tickets where order_id = $1 order by line_no, unit_no`, orderID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var tk tickets.Ticket
			if err := rows.Scan(&tk.ID, &tk.Generation, &tk.State, &tk.HolderID, &tk.ProductID, &tk.EventID, &tk.OrderID); err != nil {
				return err
			}
			out = append(out, &tk)
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (x *env) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := x.Pool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func holder(b checkout.Buyer) tickets.Holder { return tickets.Holder{ID: b.ID, Kind: b.Kind} }

func ga(n int) checkout.Item    { return checkout.Item{ProductID: salestest.ProductGA, Qty: n} }
func addon(n int) checkout.Item { return checkout.Item{ProductID: salestest.ProductAddOn, Qty: n} }

// feed is the event's door feed kinds in order.
func (x *env) feed(t *testing.T, eventID string) []string {
	t.Helper()
	rows, err := x.Pool.Query(context.Background(), "select kind from sales.door_feed where event_id = $1 order by seq", eventID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var k string
		_ = rows.Scan(&k)
		out = append(out, k)
	}
	return out
}

func pubKeys(r *tickets.KeyRing) map[byte]ed25519.PublicKey { return r.Keys() }

var _ = uuid.NewString
