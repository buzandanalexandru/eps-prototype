package checkout_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

func at(s string) func() time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return func() time.Time { return t }
}

// Scenario "Schedule adds up": 10 RON now plus the monthly parts of a 539 RON order add up to
// exactly 539 RON, the remainder on the last part.
func TestScheduleAddsUp(t *testing.T) {
	env := salestest.Setup(t)
	env.Svc.Now = at("2026-12-05T10:00:00Z")
	sale := env.NewSale(t, 100, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductPlus, Qty: 1})
	offer, err := env.Svc.Offer(context.Background(), o)
	if err != nil || offer == nil {
		t.Fatalf("offer: %v %v", offer, err)
	}
	sum := offer.DepositMinor
	for _, p := range offer.Payments {
		sum += p.AmountMinor
	}
	if offer.DepositMinor != 1000 || sum != 53900 {
		t.Fatalf("deposit %d, parts add up to %d; want 1000 and 53900", offer.DepositMinor, sum)
	}
	last := offer.Payments[len(offer.Payments)-1].AmountMinor
	if first := offer.Payments[0].AmountMinor; last < first || last-first >= int64(len(offer.Payments)) {
		t.Fatalf("remainder not on the last part: first %d last %d", first, last)
	}
}

// Scenario "Plan capped per buyer": up to 9 monthly payments for an event on 15 Jul 2027 gives a
// buyer on 5 Dec 2026 seven (1 Jan to 1 Jul 2027); a buyer in March gets fewer.
func TestPlanCappedPerBuyer(t *testing.T) {
	env := salestest.Setup(t)
	env.Svc.Now = at("2026-12-05T10:00:00Z")
	sale := env.NewSale(t, 100, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	offer, _ := env.Svc.Offer(context.Background(), o)
	if offer == nil || len(offer.Payments) != 7 || offer.Payments[0].DueOn != "2027-01-01" || offer.Payments[6].DueOn != "2027-07-01" {
		t.Fatalf("5 Dec buyer: %+v", offer)
	}
	env.Svc.Now = at("2027-03-10T10:00:00Z")
	later, _ := env.Svc.Offer(context.Background(), o)
	if later == nil || len(later.Payments) != 4 || later.Payments[0].DueOn != "2027-04-01" {
		t.Fatalf("March buyer: %+v", later)
	}
}

type decliningCharger struct{ env *salestest.Env }

func (c decliningCharger) ChargePart(ctx context.Context, p *checkout.Plan, n int) error {
	return db.WithTx(ctx, c.env.Pool, db.RoleCore, p.OrganiserID, func(tx pgx.Tx) error {
		return checkout.PartResultTx(ctx, tx, p.OrganiserID, p.ID, n, false, c.env.Svc.Clock())
	})
}

// Scenario "Later instalment fails": a declined part is overdue with a 7-day grace period.
func TestLaterInstalmentFails(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	env.Svc.Now = at("2026-12-05T10:00:00Z")
	sale := env.NewSale(t, 100, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductPlus, Qty: 1})
	offer, _ := env.Svc.Offer(ctx, o)
	err := db.WithTx(ctx, env.Pool, db.RoleCore, sale.Org, func(tx pgx.Tx) error {
		if _, err := checkout.CreatePlanTx(ctx, tx, o, offer); err != nil {
			return err
		}
		return checkout.ActivatePlanTx(ctx, tx, o.ID, "")
	})
	if err != nil {
		t.Fatal(err)
	}
	// Nothing is due on 5 Dec; "simulate next due date" moves the plan's clock to 1 Jan.
	runner := &checkout.InstalmentRunner{Svc: env.Svc, Charger: decliningCharger{env}}
	if n, _ := runner.RunOnce(ctx); n != 0 {
		t.Fatalf("%d parts charged before they are due", n)
	}
	if _, err := env.Svc.SimulateNextDue(ctx, sale.Org, o.ID); err != nil {
		t.Fatal(err)
	}
	if n, err := runner.RunOnce(ctx); n != 1 || err != nil {
		t.Fatalf("charged %d (%v), want the first part", n, err)
	}
	p, _ := env.Svc.PlanOf(ctx, sale.Org, o.ID)
	pt := p.Parts[0]
	if pt.State != "overdue" || pt.GraceUntil == nil || *pt.GraceUntil != "2027-01-08" {
		t.Fatalf("part %+v, want overdue with grace until 8 Jan 2027", pt)
	}
	if p.Parts[1].State != "scheduled" {
		t.Fatalf("second part %s", p.Parts[1].State)
	}
}

// Review D-c: when an overdue part's 7-day grace ends, the plan defaults: the order is cancelled,
// its stock goes back, the open parts are cancelled and what was paid is kept.
func TestGraceEndsPlanDefaults(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	env.Svc.Now = at("2026-12-05T10:00:00Z")
	sale := env.NewSale(t, 100, 100)
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductPlus, Qty: 1})
	offer, _ := env.Svc.Offer(ctx, o)
	err := db.WithTx(ctx, env.Pool, db.RoleCore, sale.Org, func(tx pgx.Tx) error {
		if _, err := checkout.CreatePlanTx(ctx, tx, o, offer); err != nil {
			return err
		}
		return checkout.ActivatePlanTx(ctx, tx, o.ID, "")
	})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := env.Svc.Complete(ctx, sale.Org, o.ID, "deposit", "deposit paid", nil); err != nil || !out.Paid {
		t.Fatalf("deposit: %v", err)
	}
	runner := &checkout.InstalmentRunner{Svc: env.Svc, Charger: decliningCharger{env}}
	if _, err := env.Svc.SimulateNextDue(ctx, sale.Org, o.ID); err != nil {
		t.Fatal(err)
	}
	if n, _ := runner.RunOnce(ctx); n != 1 {
		t.Fatal("no part charged")
	}
	// Eight days later on the plan's clock.
	env.Svc.Now = at("2026-12-13T10:00:00Z")
	if n, err := (&checkout.DefaultRunner{Svc: env.Svc}).RunOnce(ctx); err != nil || n != 1 {
		t.Fatalf("defaulted %d (%v)", n, err)
	}
	got, _ := env.Svc.GetIn(ctx, sale.Org, o.ID)
	if got.State != checkout.Cancelled {
		t.Fatalf("order %s", got.State)
	}
	p, _ := env.Svc.PlanOf(ctx, sale.Org, o.ID)
	if p.State != "defaulted" || p.Parts[0].State != "cancelled" || p.Parts[1].State != "cancelled" {
		t.Fatalf("plan %+v", p)
	}
	if used := sale.Used(t)[salestest.PoolPlus]; used != 0 {
		t.Fatalf("stock used %d", used)
	}
}
