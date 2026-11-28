package payments_test

import (
	"context"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
)

// A fails before accepting, B pays, in one press, and one failover event is recorded.
func TestAFailsBPays(t *testing.T) {
	r := newRig(t)
	r.knobs(t, "a", fakepsp.Knobs{FailureRate: 1})
	b, o := r.order(t)
	res, err := r.press(t, b, o, payments.HostedCard, "press-failover-1")
	if err != nil {
		t.Fatalf("the fan must see no error: %v", err)
	}
	if res.Attempt.Provider != "b" || res.ActionURL == "" {
		t.Fatalf("attempt %+v", res.Attempt)
	}
	r.pagePost(t, res.ActionURL, "approve")
	r.waitState(t, o.ID, checkout.Paid)
	as := r.attempts(t, o.ID)
	if len(as) != 2 || as[0].Provider != "a" || as[0].State != payments.Failed || as[1].Provider != "b" || as[1].State != payments.Succeeded {
		t.Fatalf("attempts %+v", as)
	}
	if n := r.events(t, "payment.failover"); n != 1 {
		t.Fatalf("%d failover events, want 1 per failover", n)
	}
}

// Scenario "Decline is not failed over".
func TestDeclineIsNotFailedOver(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	res, err := r.press(t, b, o, payments.HostedCard, "press-decline-1")
	if err != nil {
		t.Fatal(err)
	}
	r.pagePost(t, res.ActionURL, "decline_funds")
	got := r.waitState(t, o.ID, checkout.PaymentFailed)
	if got.LastErrorCode == nil || *got.LastErrorCode != apperr.PayDeclined {
		t.Fatalf("code %v", got.LastErrorCode)
	}
	as := r.attempts(t, o.ID)
	if len(as) != 1 || as[0].Provider != "a" || as[0].State != payments.Declined {
		t.Fatalf("attempts %+v: no attempt may go to B after a decline", as)
	}
	if !got.HoldExpiresAt.After(got.CreatedAt) {
		t.Fatal("hold not live")
	}
	if n := r.events(t, "payment.failover"); n != 0 {
		t.Fatalf("%d failover events", n)
	}
}

func openBreaker(t *testing.T, r *rig, provider string) {
	t.Helper()
	for i := 0; i < payments.MaxConsecutive; i++ {
		r.svc.Breaker.Record(context.Background(), r.sale.Org, provider, false)
	}
	if st := r.svc.Breaker.Read(context.Background(), r.sale.Org, provider); st.State != payments.Open {
		t.Fatalf("breaker %s is %s", provider, st.State)
	}
}

// Scenario "Both providers down": nothing is sent, PROVIDERS-BUSY, the hold keeps running.
func TestBothProvidersDown(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	openBreaker(t, r, "a")
	openBreaker(t, r, "b")
	_, err := r.press(t, b, o, payments.HostedCard, "press-busy-1")
	if code(err) != apperr.PayProvidersBusy {
		t.Fatalf("want %s, got %v", apperr.PayProvidersBusy, err)
	}
	if n := len(r.attempts(t, o.ID)); n != 0 {
		t.Fatalf("%d attempts sent", n)
	}
	got := r.get(t, o.ID)
	if got.State != checkout.Held {
		t.Fatalf("order %s, want still held", got.State)
	}
	if n := r.psp["a"].Stats()["charges_created"] + r.psp["b"].Stats()["charges_created"]; n != 0 {
		t.Fatalf("%d charges", n)
	}
}

// Scenario "Google Pay during failover": with A's breaker open, Google Pay completes on B.
func TestGooglePayDuringFailover(t *testing.T) {
	r := newRig(t)
	openBreaker(t, r, "a")
	b, o := r.order(t)
	res, err := r.press(t, b, o, payments.GooglePay, "press-gpay-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempt.Provider != "b" || res.Attempt.Method != payments.GooglePay {
		t.Fatalf("attempt %+v", res.Attempt)
	}
	r.pagePost(t, res.ActionURL, "approve")
	r.waitState(t, o.ID, checkout.Paid)
	if n := r.psp["a"].Stats()["charges_created"]; n != 0 {
		t.Fatalf("A got %d charges while its breaker was open", n)
	}
}
