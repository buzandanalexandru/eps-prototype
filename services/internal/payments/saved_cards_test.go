package payments_test

import (
	"context"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// saveCard pays an order with "save card" and returns the saved card.
func saveCard(t *testing.T, r *rig) (checkout.Buyer, payments.SavedMethod) {
	t.Helper()
	b, o := r.order(t)
	res, err := r.svc.Pay(context.Background(), b, o, payments.PayRequest{Method: payments.HostedCard, SaveCard: true, ReturnURL: "https://shop.example.test/"}, "press-save-"+o.ID[:8])
	if err != nil {
		t.Fatal(err)
	}
	r.pagePost(t, res.ActionURL, "approve")
	r.waitState(t, o.ID, checkout.Paid)
	list, err := r.svc.SavedMethods(context.Background(), r.sale.Org, b.ID)
	if err != nil || len(list) != 1 || list[0].Label != "Test Visa •••• 4242" {
		t.Fatalf("saved cards %+v %v", list, err)
	}
	return b, list[0]
}

// One tap with a saved card, no card page.
func TestPayWithSavedCard(t *testing.T) {
	r := newRig(t)
	b, card := saveCard(t, r)
	o := r.sale.Details(t, r.sale.Hold(t, b, checkout.Item{ProductID: salestest.ProductPlus, Qty: 1}))
	res, err := r.svc.Pay(context.Background(), b, o, payments.PayRequest{Method: payments.SavedCard, SavedCardID: card.ID}, "press-onetap-1")
	if err != nil {
		t.Fatal(err)
	}
	if res.ActionURL != "" {
		t.Fatalf("a page opened: %s", res.ActionURL)
	}
	r.waitState(t, o.ID, checkout.Paid)
}

// Scenario "Token from another account": refused, nothing charged.
func TestSavedCardOfAnotherAccount(t *testing.T) {
	r := newRig(t)
	_, card := saveCard(t, r)
	before := r.psp["a"].Stats()["charges_created"]
	other, o := r.order(t)
	_, err := r.svc.Pay(context.Background(), other, o, payments.PayRequest{Method: payments.SavedCard, SavedCardID: card.ID}, "press-steal-1")
	if code(err) != apperr.SavedCardRefused {
		t.Fatalf("want %s, got %v", apperr.SavedCardRefused, err)
	}
	if after := r.psp["a"].Stats()["charges_created"]; after != before {
		t.Fatalf("%d charges made", after-before)
	}
	// Another organiser never sees the card either.
	if list, _ := r.svc.SavedMethods(context.Background(), r.env.NewSale(t, 10, 10).Org, other.ID); len(list) != 0 {
		t.Fatal("card visible at another organiser")
	}
}

// Scenario "Delete a saved card": gone from checkout, token revoked at the provider.
func TestDeleteSavedCard(t *testing.T) {
	r := newRig(t)
	b, card := saveCard(t, r)
	if err := r.svc.DeleteSavedMethod(context.Background(), r.sale.Org, b.ID, card.ID); err != nil {
		t.Fatal(err)
	}
	if list, _ := r.svc.SavedMethods(context.Background(), r.sale.Org, b.ID); len(list) != 0 {
		t.Fatalf("still listed: %+v", list)
	}
	// The provider refuses the revoked token even if someone still holds it.
	_, err := r.svc.Providers["a"].Create(context.Background(), payments.ChargeRequest{AmountMinor: 100, Currency: "RON", IdempotencyKey: "after-delete-1",
		Method: payments.SavedCard, Token: card.Token, CustomerRef: b.ID, Merchant: r.sale.Org})
	if err == nil {
		t.Fatal("revoked token charged")
	}
}
