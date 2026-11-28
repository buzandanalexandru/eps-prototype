package wallet_test

import (
	"context"
	"sync"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// One tap: 600 RON pays a 539 RON order in one step, 61 RON left, no provider called.
func TestPayFromWallet(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	fan := salestest.Buyer()
	r.fund(t, r.sale.Org, fan.ID, 60000)
	o := r.order(t, r.sale, fan, salestest.ProductPlus, 1)
	got, err := r.w.PayOrder(ctx, fan, o, "wallet-pay-one")
	if err != nil || got.State != checkout.Paid {
		t.Fatalf("pay: %v %v", got, err)
	}
	if b := r.balance(t, r.sale.Org, fan.ID); b != 6100 {
		t.Fatalf("balance %d, want 6100", b)
	}
	if n := r.psp.Stats()["charges_created"]; n != 0 {
		t.Fatalf("%d provider charges", n)
	}
	if used := r.sale.Used(t)[salestest.PoolPlus]; used != 1 {
		t.Fatalf("stock used %d", used)
	}
	// Repeating the press is a replay, not a second debit.
	if _, err := r.w.PayOrder(ctx, fan, got, "wallet-pay-one"); err != nil {
		t.Fatal(err)
	}
	if b := r.balance(t, r.sale.Org, fan.ID); b != 6100 {
		t.Fatalf("balance %d after a replay", b)
	}
}

// Scenario "Two payments race for one balance": two 400 RON orders on 600 RON, exactly one wins.
func TestTwoPaymentsRaceForOneBalance(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	fan := salestest.Buyer()
	r.fund(t, r.sale.Org, fan.ID, 60000)
	// Two events of the organiser (one live order per buyer per sale), 400.00 each.
	price := func(doc map[string]any) {
		ga := doc["products"].([]any)[0].(map[string]any)
		ga["priceSteps"] = []any{map[string]any{"index": 0, "mode": "by_date", "fromAt": nil, "afterSold": nil, "faceMinor": 40000, "allInMinor": 40000, "fanFeeMinor": 0, "organiserNetMinor": 39000}}
	}
	r.sale.Publish(t, price)
	sale2 := r.env.NewSaleFor(t, r.sale.Org, r.sale.Brand, 100, 100)
	sale2.Publish(t, price)
	o1 := r.order(t, r.sale, fan, salestest.ProductGA, 1)
	o2 := r.order(t, sale2, fan, salestest.ProductGA, 1)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, o := range []*checkout.Order{o1, o2} {
		wg.Add(1)
		go func(i int, o *checkout.Order) {
			defer wg.Done()
			_, errs[i] = r.w.PayOrder(ctx, fan, o, "")
		}(i, o)
	}
	wg.Wait()
	ok, refused := 0, 0
	for _, err := range errs {
		switch code(err) {
		case "":
			ok++
		case apperr.WalletInsufficient:
			refused++
		default:
			t.Fatalf("unexpected: %v", err)
		}
	}
	if ok != 1 || refused != 1 {
		t.Fatalf("%d paid, %d refused", ok, refused)
	}
	if b := r.balance(t, r.sale.Org, fan.ID); b != 20000 {
		t.Fatalf("balance %d, want 200.00", b)
	}
	r.audit(t, r.sale.Org)
}
