package payments_test

import (
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
)

// Scenario "Normal success": created, submitted, succeeded; the order is paid on the webhook.
func TestNormalSuccess(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	res, err := r.press(t, b, o, payments.HostedCard, "press-normal-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Attempt.State != payments.Submitted || res.ActionURL == "" || res.Attempt.Provider != "a" {
		t.Fatalf("after Pay: %+v %q", res.Attempt, res.ActionURL)
	}
	if got := r.get(t, o.ID); got.State != checkout.Paying {
		t.Fatalf("order %s while the fan is on the provider page", got.State)
	}
	r.pagePost(t, res.ActionURL, "approve")
	r.waitState(t, o.ID, checkout.Paid)
	as := r.attempts(t, o.ID)
	if len(as) != 1 || as[0].State != payments.Succeeded {
		t.Fatalf("attempts %+v", as)
	}
}

// Scenario "Second attempt while one is open".
func TestSecondAttemptWhileOneIsOpen(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	first, err := r.press(t, b, o, payments.HostedCard, "press-open-1")
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.press(t, b, r.get(t, o.ID), payments.ApplePay, "press-open-2")
	if err != nil {
		t.Fatal(err)
	}
	if second.Attempt.ID != first.Attempt.ID {
		t.Fatalf("a second attempt %s was made while %s is open", second.Attempt.ID, first.Attempt.ID)
	}
	if n := len(r.attempts(t, o.ID)); n != 1 {
		t.Fatalf("%d attempts", n)
	}
}
