package simulation_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simtest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simulation"
)

// Scenario "Create a rehearsal": same snapshot version, prices and settings, full stock of its
// own under an event id of its own, and its waiting room says it is a rehearsal.
func TestCreateRehearsal(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 100, 20)
	ctx := context.Background()
	rh, err := s.Sim.CreateRehearsal(ctx, ev.Org, real.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if rh.Rehearsal == nil || rh.Rehearsal.SourceSaleID != real.ID || rh.Rehearsal.SourceEventID != ev.Event || rh.Rehearsal.SnapshotVersion != ev.Version {
		t.Fatalf("rehearsal link: %+v", rh.Rehearsal)
	}
	if rh.EventID == ev.Event {
		t.Fatal("a rehearsal must have an event id of its own")
	}
	if rh.Settings.CapPerOrder != real.Settings.CapPerOrder || rh.Settings.RatePerMinute != real.Settings.RatePerMinute {
		t.Fatalf("settings not copied: %+v", rh.Settings)
	}
	snap, err := s.Store.Load(ctx, rh.EventID)
	if err != nil {
		t.Fatal(err)
	}
	src, _ := s.Store.Load(ctx, ev.Event)
	if !snap.Rehearsal || snap.Version != 1 || snap.EventID != rh.EventID || len(snap.Products) != len(src.Products) {
		t.Fatalf("rehearsal snapshot: rehearsal=%v v=%d event=%s", snap.Rehearsal, snap.Version, snap.EventID)
	}
	if snap.Products[0].PriceSteps[0].AllInMinor != src.Products[0].PriceSteps[0].AllInMinor || snap.Pools[0].Instances[0].Capacity != 100 {
		t.Fatal("prices or capacity differ from the source")
	}
	// Full stock: nothing used on the rehearsal event.
	used, err := checkout.Usage{Svc: s.Svc}.PoolUsage(ctx, ev.Org, rh.EventID)
	if err != nil {
		t.Fatal(err)
	}
	for inst, n := range used {
		if n != 0 {
			t.Fatalf("rehearsal stock used on %s: %d", inst, n)
		}
	}
	b := newBrowser(s.SimToken(t, rh.EventID, uuid.NewString(), "86.100.0.1"))
	code, out := b.join(t, s, rh.ID)
	if code != http.StatusOK || out["rehearsal"] != true {
		t.Fatalf("rehearsal waiting room: %d %v", code, out)
	}
	// A rehearsal of a rehearsal is refused.
	if _, err := s.Sim.CreateRehearsal(ctx, ev.Org, rh.ID, "test"); err == nil {
		t.Fatal("rehearsal of a rehearsal accepted")
	}
}

// Scenario "Kept out of the real sale": rehearsal orders never reach the real event's orders,
// stock counters or cap ledger, and each is marked rehearsal with its run.
func TestRehearsalOrdersKeptOut(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 100, 20)
	rh := openRehearsal(t, s, real)
	run := uuid.NewString()
	b := newBrowser(s.SimToken(t, rh.EventID, run, "86.100.1.1"))
	b.admit(t, s, rh.ID)
	o := b.buy(t, s, rh.EventID, 1, 2)
	if o["state"] != "paid" && o["state"] != "issued" {
		t.Fatalf("rehearsal order: %v", o["state"])
	}
	if o["rehearsal"] != true {
		t.Fatalf("order not marked rehearsal: %v", o["rehearsal"])
	}
	if n := count(t, s, `select count(*) from sales.orders where event_id = $1`, ev.Event); n != 0 {
		t.Fatalf("real event has %d orders", n)
	}
	if n := count(t, s, `select count(*) from sales.cap_ledger where event_id = $1`, ev.Event); n != 0 {
		t.Fatalf("real event cap ledger has %d rows", n)
	}
	if n := count(t, s, `select count(*) from sales.orders where event_id = $1 and rehearsal and sim_run_id = $2 and state in ('paid', 'issued')`, rh.EventID, run); n != 1 {
		t.Fatalf("rehearsal orders marked with the run: %d", n)
	}
	used := ev.Used(t)
	for inst, n := range used {
		if n != 0 {
			t.Fatalf("real stock used on %s: %d", inst, n)
		}
	}
	// A report that sums the organiser's orders leaves rehearsals out.
	if n := count(t, s, `select count(*) from sales.orders where organiser_id = $1 and not rehearsal`, ev.Org); n != 0 {
		t.Fatalf("organiser report counts %d", n)
	}
}

// Scenario "Rehearsed outage stays in the rehearsal": an outage set for the rehearsal's merchant
// leaves the real sale paying at provider A with its breaker closed.
func TestRehearsedOutageStaysInRehearsal(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 100, 20)
	rh := openRehearsal(t, s, real)
	ctx := context.Background()
	body, _ := json.Marshal(map[string]any{"outage": true, "outage_mode": "503"})
	if _, err := s.Knobs["a"].Put(ctx, payments.MerchantOf(payments.Scope(ev.Org, rh.EventID)), body); err != nil {
		t.Fatal(err)
	}
	// Rehearsal payments fail over to B; enough failures open the rehearsal's breaker only.
	for i := 0; i < 25; i++ {
		s.Breaker.Record(ctx, payments.Scope(ev.Org, rh.EventID), "a", false)
	}
	sim := newBrowser(s.SimToken(t, rh.EventID, uuid.NewString(), "86.100.0.9"))
	sim.admit(t, s, rh.ID)
	o := sim.buy(t, s, rh.EventID, 1, 1)
	if o["state"] != "paid" && o["state"] != "issued" {
		t.Fatalf("rehearsal order: %v", o["state"])
	}
	if got := s.Breaker.Read(ctx, payments.Scope(ev.Org, rh.EventID), "a").State; got != payments.Open {
		t.Fatalf("rehearsal breaker: %s", got)
	}
	fan := newBrowser(s.FanToken(t, "192.0.2.50"))
	fan.admit(t, s, real.ID)
	ro := fan.buy(t, s, ev.Event, ev.Version, 1)
	if ro["state"] != "paid" && ro["state"] != "issued" {
		t.Fatalf("real order: %v", ro["state"])
	}
	if got := s.Breaker.Read(ctx, ev.Org, "a").State; got != payments.Closed {
		t.Fatalf("organiser breaker: %s", got)
	}
	if n := count(t, s, `select count(*) from sales.payment_attempts a join sales.orders o on o.id = a.order_id
		where o.event_id = $1 and a.provider = 'a' and a.state = 'succeeded'`, ev.Event); n != 1 {
		t.Fatalf("the real payment should succeed at A: %d", n)
	}
	if n := count(t, s, `select count(*) from sales.payment_attempts a join sales.orders o on o.id = a.order_id
		where o.event_id = $1 and a.provider = 'b' and a.state = 'succeeded' and a.merchant_event = $1`, rh.EventID); n != 1 {
		t.Fatalf("the rehearsal payment should succeed at B under its merchant: %d", n)
	}
}

// Scenario "Reset": a fresh rehearsal with full stock and an empty queue, the old one's Redis data
// deleted by the clean-up.
func TestResetRehearsal(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 100, 20)
	rh := openRehearsal(t, s, real)
	ctx := context.Background()
	b := newBrowser(s.SimToken(t, rh.EventID, uuid.NewString(), "86.100.0.7"))
	b.admit(t, s, rh.ID)
	if code, out := b.hold(t, s, rh.EventID, 1, 2); code != http.StatusCreated {
		t.Fatalf("hold: %d %v", code, out)
	}
	fresh, err := s.Sim.ResetRehearsal(ctx, ev.Org, rh.ID, "test")
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID == rh.ID || fresh.EventID == rh.EventID || fresh.Rehearsal.SourceSaleID != real.ID {
		t.Fatalf("not a fresh rehearsal: %+v", fresh)
	}
	list, _ := s.Sales.Rehearsals(ctx, ev.Org, real.ID)
	if len(list) != 1 || list[0].ID != fresh.ID {
		t.Fatalf("live rehearsals: %d", len(list))
	}
	before, _ := s.Redis.Keys(ctx, "eps:{"+ev.Org+":"+rh.EventID+":*").Result()
	if len(before) == 0 {
		t.Fatal("expected the old rehearsal to have Redis keys")
	}
	c := &simulation.Cleaner{Pool: s.Pool, Redis: s.Redis, Sales: s.Sales, Now: time.Now}
	// A reset rehearsal waits out its holds first (review D-e).
	if _, err := c.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if left, _ := s.Redis.Keys(ctx, "eps:{"+ev.Org+":"+rh.EventID+":*").Result(); len(left) == 0 {
		t.Fatal("cleaned before its holds could end")
	}
	if _, err := s.Pool.Exec(ctx, `update sales.sales set deleted_at = now() - interval '21 minutes' where id = $1`, rh.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := c.RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, `select count(*) from sales.orders where event_id = $1 and (buyer_email is not null or buyer_name is not null)`, rh.EventID); n != 0 {
		t.Fatalf("%d rehearsal orders keep contact details after the clean-up", n)
	}
	for _, pat := range []string{"eps:{" + ev.Org + ":" + rh.EventID + ":*", "eps:{" + ev.Org + "}:" + rh.EventID + ":*"} {
		if left, _ := s.Redis.Keys(ctx, pat).Result(); len(left) != 0 {
			t.Fatalf("keys left after the clean-up: %v", left[:min(3, len(left))])
		}
	}
	// Nothing of the real event was touched.
	if k, _ := s.Redis.Keys(ctx, "eps:{"+ev.Org+"}:"+ev.Event+":*").Result(); len(k) == 0 {
		t.Fatal("the real event's keys were deleted")
	}
	// The fresh rehearsal's waiting room is empty.
	b2 := newBrowser(s.SimToken(t, fresh.EventID, uuid.NewString(), "86.100.0.8"))
	if _, out := b2.join(t, s, fresh.ID); out["state"] != "before" {
		t.Fatalf("fresh rehearsal: %v", out["state"])
	}
}

// Review D-a: a signed-in person on a rehearsal is offered and allowed neither the wallet, a saved
// card, saving a card nor instalments, and no ledger line is written; the order is never issued.
func TestRehearsalNeverTouchesRealMoney(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 100, 20)
	rh := openRehearsal(t, s, real)
	b := newBrowser(s.AccountToken(t, "192.0.2.80"))
	b.admit(t, s, rh.ID)
	code, o := b.hold(t, s, rh.EventID, 1, 1)
	if code != http.StatusCreated {
		t.Fatalf("hold: %d %v", code, o)
	}
	id := o["id"].(string)
	if o["wallet"] != nil || o["instalmentOffer"] != nil || len(o["savedCards"].([]any)) != 0 {
		t.Fatalf("a rehearsal offers wallet, cards or instalments: %v %v %v", o["wallet"], o["instalmentOffer"], o["savedCards"])
	}
	call(t, http.MethodPut, s.Core.URL+"/v1/shop/orders/"+id+"/details", b.shop(), map[string]any{"name": "Ana", "email": "ana@example.test"})
	for _, body := range []map[string]any{
		{"method": "wallet"},
		{"method": "saved_card", "savedCardId": uuid.NewString(), "returnUrl": "http://localhost:3000/back"},
		{"method": "hosted_card", "saveCard": true, "returnUrl": "http://localhost:3000/back"},
		{"method": "hosted_card", "plan": "instalments", "returnUrl": "http://localhost:3000/back"},
	} {
		code, out := call(t, http.MethodPost, s.Core.URL+"/v1/shop/orders/"+id+"/payments", b.shop(), body)
		if code != http.StatusUnprocessableEntity || out["code"] != "EPS-PAY-METHOD" {
			t.Fatalf("%v: %d %v", body, code, out)
		}
	}
	if n := count(t, s, `select count(*) from sales.journal_lines where organiser_id = $1`, ev.Org); n != 0 {
		t.Fatalf("%d ledger lines for a rehearsal", n)
	}
	if n := count(t, s, `select count(*) from sales.payment_attempts where order_id = $1`, id); n != 0 {
		t.Fatalf("%d attempts were made", n)
	}
	// Card is fine; the paid order stays paid (never issued) and the real-orders view leaves it out.
	if final := b.buy(t, s, rh.EventID, 1, 1); final["state"] != "paid" {
		t.Fatalf("rehearsal order: %v", final["state"])
	}
	if err := s.Svc.Issue(context.Background(), ev.Org, id); err != nil {
		t.Fatal(err)
	}
	if n := count(t, s, `select count(*) from sales.orders where id = $1 and state = 'issued'`, id); n != 0 {
		t.Fatal("a rehearsal order was issued")
	}
	if n := count(t, s, `select count(*) from sales.real_orders where event_id = $1`, rh.EventID); n != 0 {
		t.Fatalf("real_orders shows %d rehearsal orders", n)
	}
}
