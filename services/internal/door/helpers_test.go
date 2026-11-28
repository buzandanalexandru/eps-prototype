package door_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/door"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

func TestMain(m *testing.M) { testdb.Main(m) }

type env struct {
	*salestest.Env
	sale *salestest.Sale
	tix  *tickets.Service
	door *door.Service
}

func setup(t *testing.T) *env {
	t.Helper()
	e := salestest.Setup(t)
	x := &env{Env: e, sale: e.NewSale(t, 100, 100)}
	x.tix = &tickets.Service{Pool: e.Pool, Snaps: e.Store, Links: e.Svc.Links, Log: e.Log}
	x.door = &door.Service{Pool: e.Pool, Snaps: e.Store}
	return x
}

func (x *env) staff(role string) door.Staff {
	return door.Staff{Org: x.sale.Org, Role: role, Sub: "staff-1"}
}

// ticketsFor issues orders of at most 5 GA passes until n tickets exist and returns their ids.
func (x *env) ticketsFor(t *testing.T, n int) []string {
	t.Helper()
	ctx := context.Background()
	var ids []string
	for len(ids) < n {
		q := min(5, n-len(ids))
		o := x.sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: q})
		if out, err := x.Svc.Complete(ctx, x.sale.Org, o.ID, "p-"+o.ID[:8], "test", nil); err != nil || !out.Paid {
			t.Fatal(err)
		}
		if _, err := x.tix.Issue(ctx, x.sale.Org, o.ID); err != nil {
			t.Fatal(err)
		}
		rows, err := x.Pool.Query(ctx, "select id::text from sales.tickets where order_id = $1 order by unit_no", o.ID)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var id string
			_ = rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
	}
	return ids
}

func (x *env) device(t *testing.T, gate string) string {
	t.Helper()
	d, err := x.door.Register(context.Background(), x.staff("box_office"), x.sale.Event, gate)
	if err != nil {
		t.Fatal(err)
	}
	return d.ID
}

var t0 = time.Now().Add(-30 * time.Minute).UTC().Truncate(time.Second)
