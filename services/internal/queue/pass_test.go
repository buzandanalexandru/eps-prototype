package queue_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
)

// Scenario "Pass issued": a fan at the front gets a pass that expires 10 minutes after admission,
// bound to its session, sale and event.
func TestPassIssued(t *testing.T) {
	env := onsaletest.Setup(t)
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), env.Open())
	f := env.Admitted(t, s, onsaletest.NewFan())
	c, err := env.Engine.Verifier().Verify(f.Pass, queue.KindPass, env.Clock.Now(), false)
	if err != nil {
		t.Fatal(err)
	}
	life := time.Unix(c.Exp, 0).Sub(time.Unix(c.Iat, 0))
	if life < 9*time.Minute+50*time.Second || life > 10*time.Minute {
		t.Fatalf("pass lasts %s, want 10 minutes", life)
	}
	if c.Sale != s.ID || c.Evt != s.EventID || c.Org != s.OrganiserID || c.Sid != queue.SessionHash(f.Session) || c.Sub != f.Buyer.ID {
		t.Fatalf("claims %+v", c)
	}
	// The same pass on every poll.
	if again := env.Status(t, s, f); again.Pass != f.Pass || again.State != queue.PlaceAdmitted {
		t.Fatalf("second poll %+v", again)
	}
	// Ten minutes later the pass has ended.
	env.Clock.Add(queue.PassLife)
	if got := env.Status(t, s, f); got.State != queue.PlaceEnded {
		t.Fatalf("after 10 minutes %+v", got)
	}
}

// Scenario "Pass ends after purchase": once the order is paid the pass can no longer be used and
// the shopping count drops by one.
func TestPassEndsAfterPurchase(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	sale := env.Env.NewSale(t, 100, 100)
	s := env.NewSale(t, sale, env.Open())
	f := env.Admitted(t, s, onsaletest.NewFan())
	o := sale.Hold(t, f.Buyer, salestestGA(2))
	before, _ := queue.CountsOf(ctx, env.Redis, s.OrganiserID, s.EventID, s.ID, env.Clock.Now())
	out, err := env.Svc.Complete(ctx, sale.Org, o.ID, "pay-"+o.ID[:8], "test payment", nil)
	if err != nil || !out.Paid {
		t.Fatalf("pay: %+v %v", out, err)
	}
	// The worker's consumer sees order.paid.
	if err := env.Life.End(ctx, out.Order, "used"); err != nil {
		t.Fatal(err)
	}
	after, _ := queue.CountsOf(ctx, env.Redis, s.OrganiserID, s.EventID, s.ID, env.Clock.Now())
	if before.Active != 1 || after.Active != 0 {
		t.Fatalf("shopping %d then %d, want 1 then 0", before.Active, after.Active)
	}
	if _, err := env.Gate.Passes.Check(ctx, f.Pass, f.Session, f.Buyer.ID, s.OrganiserID, s.EventID, s.ID, env.Clock.Now(), false); err == nil {
		t.Fatal("a used pass still works")
	}
	if _, _, err := env.Svc.CreateOrder(ctx, f.Buyer, sale.Request(checkout.Item{ProductID: salestestGA(1).ProductID, Qty: 1}), ""); code(err) != "EPS-PASS-REQUIRED" {
		t.Fatalf("a hold with the used pass: %v", err)
	}
}
