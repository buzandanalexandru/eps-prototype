package payments_test

import (
	"context"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
)

// lossy wraps a provider: Create times out (optionally after the provider made the charge).
type lossy struct {
	payments.Provider
	charge bool
	calls  int
}

func (l *lossy) Create(ctx context.Context, r payments.ChargeRequest) (payments.Charge, error) {
	l.calls++
	if l.charge {
		_, _ = l.Provider.Create(ctx, r)
	}
	return payments.Charge{}, payments.ErrTimeout
}

type counting struct {
	payments.Provider
	calls int
}

func (c *counting) Create(ctx context.Context, r payments.ChargeRequest) (payments.Charge, error) {
	c.calls++
	return c.Provider.Create(ctx, r)
}

// Scenario "Timeout, charge did happen": the status check finds a success; B is never called.
func TestTimeoutButCharged(t *testing.T) {
	r := newRig(t)
	a := &lossy{Provider: r.svc.Providers["a"], charge: true}
	bp := &counting{Provider: r.svc.Providers["b"]}
	r.svc.Providers["a"], r.svc.Providers["b"] = a, bp
	buyer, o := r.order(t)
	res, err := r.press(t, buyer, o, payments.HostedCard, "press-unknown-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempt.Provider != "a" || res.Attempt.State != payments.Submitted {
		t.Fatalf("attempt %+v", res.Attempt)
	}
	r.pagePost(t, res.ActionURL, "approve")
	r.waitState(t, o.ID, checkout.Paid)
	if bp.calls != 0 {
		t.Fatalf("B was called %d times", bp.calls)
	}
}

// Scenario "Timeout, no charge": the provider has no such charge, so the attempt fails and a new
// attempt goes to B.
func TestTimeoutNoCharge(t *testing.T) {
	r := newRig(t)
	r.svc.Providers["a"] = &lossy{Provider: r.svc.Providers["a"], charge: false}
	buyer, o := r.order(t)
	res, err := r.press(t, buyer, o, payments.HostedCard, "press-unknown-2")
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempt.Provider != "b" {
		t.Fatalf("attempt on %s", res.Attempt.Provider)
	}
	as := r.attempts(t, o.ID)
	if len(as) != 2 || as[0].State != payments.Failed || as[1].Provider != "b" {
		t.Fatalf("attempts %+v", as)
	}
}
