package payments_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
)

// Review D-b / C1 (reported by the correctness review): a declined instalment deposit followed by
// a payment in full never activates the instalment plan.
func TestFullPaymentAfterDeclinedDepositHasNoPlan(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	b, o := r.order(t)
	res, err := r.svc.Pay(ctx, b, o, payments.PayRequest{Method: payments.HostedCard, Plan: "instalments", ReturnURL: "https://shop.example.test/"}, "inst-key-1")
	if err != nil {
		t.Fatal(err)
	}
	r.pagePost(t, res.ActionURL, "decline_funds")
	o = r.waitState(t, o.ID, checkout.PaymentFailed)
	res, err = r.svc.Pay(ctx, b, o, payments.PayRequest{Method: payments.HostedCard, SaveCard: true, ReturnURL: "https://shop.example.test/"}, "full-key-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempt.AmountMinor != o.TotalMinor {
		t.Fatalf("full attempt of %d, total %d", res.Attempt.AmountMinor, o.TotalMinor)
	}
	r.pagePost(t, res.ActionURL, "approve")
	o = r.waitState(t, o.ID, checkout.Paid)
	if o.PayPlan != "full" {
		t.Fatalf("pay plan %s", o.PayPlan)
	}
	if p, _ := r.env.Svc.PlanOf(ctx, r.sale.Org, o.ID); p != nil && p.State == "active" {
		t.Fatalf("a full payment activated an instalment plan with %d parts", len(p.Parts))
	}
}

// Review D-b / C2 (reported by the correctness review): a deposit that succeeds after its order
// expired pays the order only with its plan rebuilt and active.
func TestLateDepositOnExpiredOrderRebuildsThePlan(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	b, o := r.order(t)
	res, err := r.svc.Pay(ctx, b, o, payments.PayRequest{Method: payments.HostedCard, Plan: "instalments", ReturnURL: "https://shop.example.test/"}, "inst-key-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.env.Pool.Exec(ctx, "update sales.payment_attempts set deadline_at = now() - interval '1 second', updated_at = now() - interval '1 minute' where id = $1", res.Attempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&payments.Reconciler{Svc: r.svc, Age: time.Millisecond}).RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	r.waitState(t, o.ID, checkout.PaymentFailed)
	r.sale.SetHoldDeadline(t, o.ID, time.Now().Add(-time.Second))
	if _, err := r.env.Svc.Expire(ctx, r.sale.Org, o.ID); err != nil {
		t.Fatal(err)
	}
	r.waitState(t, o.ID, checkout.Expired)
	r.pagePost(t, res.ActionURL, "approve")
	o = r.waitState(t, o.ID, checkout.Paid)
	p, _ := r.env.Svc.PlanOf(ctx, r.sale.Org, o.ID)
	if p == nil || p.State != "active" || len(p.Parts) == 0 {
		t.Fatalf("order paid with only the deposit; plan %+v", p)
	}
}

// Review D-c: a card an active plan charges cannot be deleted.
func TestCardOfAnActivePlanCannotBeDeleted(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	b, o := r.order(t)
	res, err := r.svc.Pay(ctx, b, o, payments.PayRequest{Method: payments.HostedCard, Plan: "instalments", ReturnURL: "https://shop.example.test/"}, "inst-key-3")
	if err != nil {
		t.Fatal(err)
	}
	r.pagePost(t, res.ActionURL, "approve")
	r.waitState(t, o.ID, checkout.Paid)
	cards, _ := r.svc.SavedMethods(ctx, r.sale.Org, b.ID)
	if len(cards) != 1 {
		t.Fatalf("cards %v", cards)
	}
	if err := r.svc.DeleteSavedMethod(ctx, r.sale.Org, b.ID, cards[0].ID); code(err) != "EPS-SAVED-CARD-IN-USE" {
		t.Fatalf("delete: %v", err)
	}
}

// Review H3: a provider that hangs (outage_mode hang) leaves the attempt unknown; B is not tried
// while A may still have charged, and the reconciler fails it at its limit.
func TestHangingProviderIsUnknownNotFailedOver(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.knobs(t, "a", fakepsp.Knobs{Outage: true, OutageMode: "hang"})
	b, o := r.order(t)
	res, err := r.press(t, b, o, payments.HostedCard, "press-hang-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempt.State != payments.Unknown || res.Attempt.Provider != "a" {
		t.Fatalf("attempt %+v", res.Attempt)
	}
	if n := r.psp["b"].Stats()["charges_created"]; n != 0 {
		t.Fatalf("B got %d charges", n)
	}
	if _, err := r.env.Pool.Exec(ctx, "update sales.payment_attempts set deadline_at = now() - interval '1 second', updated_at = now() - interval '1 minute' where id = $1", res.Attempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := (&payments.Reconciler{Svc: r.svc, Age: time.Millisecond}).RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if got := r.get(t, o.ID); got.State != checkout.PaymentFailed {
		t.Fatalf("order %s", got.State)
	}
}

// Review sec L1: a webhook whose charge does not match the attempt (amount) changes nothing.
func TestChargeOfAnotherAmountIsRefused(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	res, err := r.press(t, b, o, payments.HostedCard, "press-amount-1")
	if err != nil {
		t.Fatal(err)
	}
	err = r.svc.Apply(context.Background(), r.sale.Org, res.Attempt.ID, payments.Charge{ID: "ch_x", AmountMinor: 1, Currency: "RON", Status: "succeeded", IdempotencyKey: res.Attempt.ID})
	if err == nil {
		t.Fatal("a charge of another amount was applied")
	}
	if got := r.get(t, o.ID); got.State != checkout.Paying {
		t.Fatalf("order %s", got.State)
	}
}
