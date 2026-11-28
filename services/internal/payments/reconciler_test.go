package payments_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
)

// Scenario "Dropped webhook": the provider never sends it; within 30 s the reconciler finds the
// success and the order is paid.
func TestDroppedWebhookReconciled(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	r.knobs(t, "a", fakepsp.Knobs{WebhookDropRate: 1})
	res, err := r.press(t, b, o, payments.HostedCard, "press-drop-1")
	if err != nil {
		t.Fatal(err)
	}
	r.pagePost(t, res.ActionURL, "approve")
	time.Sleep(100 * time.Millisecond)
	if got := r.get(t, o.ID); got.State != checkout.Paying {
		t.Fatalf("order %s before reconciling", got.State)
	}
	// The worker asks about attempts unresolved for 20 s; here the age is shortened.
	rec := &payments.Reconciler{Svc: r.svc, Age: time.Millisecond}
	n, err := rec.RunOnce(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("reconciled %d (%v)", n, err)
	}
	r.waitState(t, o.ID, checkout.Paid)
}

// An attempt still without an outcome after its 120 s limit is failed; the order keeps its hold
// as payment failed and can be paid again.
func TestAttemptPastItsLimitFails(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	res, err := r.press(t, b, o, payments.HostedCard, "press-limit-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.env.Pool.Exec(context.Background(), "update sales.payment_attempts set deadline_at = now() - interval '1 second' where id = $1", res.Attempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&payments.Reconciler{Svc: r.svc, Age: time.Millisecond}).RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	got := r.get(t, o.ID)
	if got.State != checkout.PaymentFailed || *got.LastErrorCode != "EPS-PAY-TIMEOUT" {
		t.Fatalf("order %s %v", got.State, got.LastErrorCode)
	}
}
