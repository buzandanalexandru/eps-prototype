package checkout_test

import (
	"context"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

func guestOrder(t *testing.T, env *salestest.Env, sale *salestest.Sale, email string) *checkout.Order {
	t.Helper()
	o := sale.Hold(t, salestest.Guest(), checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	o, err := env.Svc.SetDetails(context.Background(), o, checkout.Details{Name: "Guest", Email: email})
	if err != nil {
		t.Fatal(err)
	}
	return o
}

// Scenario "Claim succeeds": both guest orders placed with the verified email move to the account.
func TestClaimGuestOrders(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	a := env.NewSale(t, 100, 100)
	b := env.NewSale(t, 100, 100)
	o1 := guestOrder(t, env, a, "Ana@Example.test")
	o2 := guestOrder(t, env, b, "ana@example.test")
	other := guestOrder(t, env, a, "someone@example.test")
	acct := checkout.Buyer{ID: "user-ana", Kind: "account", Email: "ana@example.test"}
	got, err := env.Svc.Claim(ctx, acct, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("claimed %v, want both of Ana's orders", got)
	}
	list, err := env.Svc.OrdersOf(ctx, "user-ana")
	if err != nil || len(list) != 2 {
		t.Fatalf("account lists %d orders (%v)", len(list), err)
	}
	ids := map[string]bool{list[0].ID: true, list[1].ID: true}
	if !ids[o1.ID] || !ids[o2.ID] || ids[other.ID] {
		t.Fatalf("listed %v", ids)
	}
}

// Scenario "Different email": an order placed with another email cannot be claimed.
func TestClaimRefusedForAnotherEmail(t *testing.T) {
	env := salestest.Setup(t)
	ctx := context.Background()
	sale := env.NewSale(t, 100, 100)
	o := guestOrder(t, env, sale, "someone@example.test")
	_, err := env.Svc.Claim(ctx, checkout.Buyer{ID: "user-ana", Kind: "account", Email: "ana@example.test"}, o.ID)
	if code(err) != apperr.ClaimRefused {
		t.Fatalf("want %s, got %v", apperr.ClaimRefused, err)
	}
	got, _ := env.Svc.GetIn(ctx, sale.Org, o.ID)
	if got.BuyerKind != "guest" {
		t.Fatal("the order changed hands")
	}
	// Without a verified email nothing can be claimed.
	if _, err := env.Svc.Claim(ctx, checkout.Buyer{ID: "user-x", Kind: "account"}, ""); code(err) != apperr.ClaimRefused {
		t.Fatalf("unverified: %v", err)
	}
}
