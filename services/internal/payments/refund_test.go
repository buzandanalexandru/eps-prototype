package payments_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
)

// Scenario "Duplicate success refunded": two attempts on one order both succeed; the second is
// refunded, the order is paid once and an alert event is recorded.
func TestDuplicateSuccessRefunded(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	b, o := r.order(t)
	first, err := r.press(t, b, o, payments.HostedCard, "press-dupe-1")
	if err != nil {
		t.Fatal(err)
	}
	// The first attempt passes its limit without an outcome: failed, order payment failed.
	if _, err := r.env.Pool.Exec(ctx, "update sales.payment_attempts set deadline_at = now() - interval '1 second' where id = $1", first.Attempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&payments.Reconciler{Svc: r.svc, Age: time.Millisecond}).RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	second, err := r.press(t, b, r.get(t, o.ID), payments.HostedCard, "press-dupe-2")
	if err != nil {
		t.Fatal(err)
	}
	r.pagePost(t, second.ActionURL, "approve")
	r.waitState(t, o.ID, checkout.Paid)
	// The fan still had the first page open and approved it too.
	r.pagePost(t, first.ActionURL, "approve")
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a, _ := r.svc.GetAttempt(ctx, r.sale.Org, first.Attempt.ID)
		if a.State == payments.Refunded {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	as := r.attempts(t, o.ID)
	if as[0].State != payments.Refunded || as[1].State != payments.Succeeded {
		t.Fatalf("attempts %+v", as)
	}
	if n := r.events(t, "payment.duplicate_success"); n != 1 {
		t.Fatalf("%d alert events", n)
	}
	if n := r.events(t, "order.paid"); n != 1 {
		t.Fatalf("%d order.paid events", n)
	}
	if r.get(t, o.ID).State != checkout.Paid {
		t.Fatal("order not paid")
	}
}
