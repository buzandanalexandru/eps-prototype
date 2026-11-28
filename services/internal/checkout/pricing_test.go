package checkout_test

import (
	"context"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// Scenario "Same price as the shop": the checkout total is the shop's all-in price, and the stored
// lines carry the fee split of the fee policy.
func TestCheckoutTotalIsTheShopsAllInPrice(t *testing.T) {
	env := salestest.Setup(t)
	sale := env.NewSale(t, 100, 100)
	// GA Plus: shown at 539.00 (the organiser absorbs the fee).
	o := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductPlus, Qty: 1})
	if o.TotalMinor != 53900 {
		t.Fatalf("total %d, want 53900", o.TotalMinor)
	}
	got, err := env.Svc.GetIn(context.Background(), sale.Org, o.ID)
	if err != nil {
		t.Fatal(err)
	}
	l := got.Lines[0]
	if l.UnitAllInMinor != 53900 || l.UnitFaceMinor != 53900 || l.UnitFanFeeMinor != 0 || l.UnitOrganiserNetMinor != 52552 {
		t.Fatalf("stored line %+v, want the absorb split 539.00 / 0 / 525.52", l)
	}
	// GA passes the fee on: 552.48 each, the same number the shop shows.
	o2 := sale.Hold(t, salestest.Buyer(), checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	if o2.TotalMinor != 2*55248 || o2.FanFeeMinor != 2*1348 || o2.OrganiserNetMinor != 2*53900 {
		t.Fatalf("GA order %d / fee %d / net %d", o2.TotalMinor, o2.FanFeeMinor, o2.OrganiserNetMinor)
	}
}

// Orders are priced from the snapshot version they name, even after a newer publish.
func TestPricedFromTheNamedVersion(t *testing.T) {
	env := salestest.Setup(t)
	sale := env.NewSale(t, 100, 100)
	v1 := sale.Version
	sale.Publish(t, func(doc map[string]any) {
		ga := doc["products"].([]any)[0].(map[string]any)
		ga["priceSteps"] = []any{map[string]any{"index": 0, "mode": "by_date", "fromAt": nil, "afterSold": nil, "faceMinor": 59900, "allInMinor": 61398, "fanFeeMinor": 1498, "organiserNetMinor": 59900}}
	})
	req := sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 1})
	req.Version = v1
	o, _, err := env.Svc.CreateOrder(context.Background(), salestest.Buyer(), req, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.TotalMinor != 55248 || o.SnapshotVersion != v1 {
		t.Fatalf("order of v%d priced %d", o.SnapshotVersion, o.TotalMinor)
	}
}
