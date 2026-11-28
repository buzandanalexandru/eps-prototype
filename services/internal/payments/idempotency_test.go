package payments_test

import (
	"context"
	"sync"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// Scenario "Double click on Pay": the provider records one charge and both answers show the same attempt.
func TestDoubleClickOnPay(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	var wg sync.WaitGroup
	ids := make([]string, 2)
	for i := range ids {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for {
				res, err := r.press(t, b, o, payments.HostedCard, "press-double-1")
				if code(err) == apperr.InProgress {
					continue // the first press is still running: the browser retries
				}
				if err != nil {
					t.Error(err)
					return
				}
				ids[i] = res.Attempt.ID
				return
			}
		}(i)
	}
	wg.Wait()
	if ids[0] == "" || ids[0] != ids[1] {
		t.Fatalf("attempts %v", ids)
	}
	if n := r.psp["a"].Stats()["charges_created"]; n != 1 {
		t.Fatalf("provider recorded %d charges", n)
	}
}

// Scenario "Retry after a lost response": the create call times out; the core asks by the same
// key and the provider returns the charge it already made (one charge in all).
func TestRetryAfterALostResponse(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	r.knobs(t, "a", fakepsp.Knobs{TimeoutRate: 1})
	res, err := r.press(t, b, o, payments.HostedCard, "press-lost-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempt.State != payments.Submitted || res.Attempt.Provider != "a" {
		t.Fatalf("after the lost answer: %+v", res.Attempt)
	}
	// The same attempt key sent again gets the same charge.
	again, err := r.svc.Providers["a"].Create(context.Background(), payments.ChargeRequest{AmountMinor: o.TotalMinor, Currency: "RON", IdempotencyKey: res.Attempt.ID,
		Method: payments.HostedCard, Merchant: r.sale.Org, ReturnURL: "https://shop.example.test/"})
	if err != nil && err != payments.ErrTimeout {
		t.Fatal(err)
	}
	if err == nil && again.Status == "" {
		t.Fatal("no charge returned")
	}
	if n := r.psp["a"].Stats()["charges_created"]; n != 1 {
		t.Fatalf("provider recorded %d charges, want 1", n)
	}
	if n := len(r.attempts(t, o.ID)); n != 1 {
		t.Fatalf("%d attempts; B must not be tried while A's outcome is known", n)
	}
}

// Scenario "Reused key, different amount".
func TestReusedKeyDifferentAmount(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	if _, err := r.press(t, b, o, payments.HostedCard, "press-reuse-1"); err != nil {
		t.Fatal(err)
	}
	// The same buyer's key on another order is a mismatch too (keys are per buyer).
	other := r.sale.Details(t, r.env.NewSaleFor(t, r.sale.Org, r.sale.Brand, 100, 100).Hold(t, b, checkout.Item{ProductID: salestest.ProductGA, Qty: 1}))
	if _, err := r.press(t, b, other, payments.HostedCard, "press-reuse-1"); code(err) != apperr.IdempotencyMismatch {
		t.Fatalf("same key, another order: %v", err)
	}
	changed := *o
	changed.TotalMinor = o.TotalMinor + 100
	if _, err := r.press(t, b, &changed, payments.HostedCard, "press-reuse-1"); code(err) != apperr.IdempotencyMismatch {
		t.Fatalf("want %s, got %v", apperr.IdempotencyMismatch, err)
	}
	if _, err := r.svc.Pay(context.Background(), b, o, payments.PayRequest{Method: payments.ApplePay, ReturnURL: "https://shop.example.test/"}, "press-reuse-1"); code(err) != apperr.IdempotencyMismatch {
		t.Fatalf("same key, another method: %v", err)
	}
	if n := r.psp["a"].Stats()["charges_created"]; n != 1 {
		t.Fatalf("provider recorded %d charges, want 1", n)
	}
}
