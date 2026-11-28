package wallet_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// Scenario "Season closes": 80 RON becomes next-season credit with its expiry; the ledger shows the
// move, and the credit pays like a balance at the same organiser.
func TestSeasonCloses(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	fan := salestest.Buyer()
	r.fund(t, r.sale.Org, fan.ID, 8000)
	expires := time.Now().AddDate(1, 0, 0).Truncate(time.Second)
	n, total, err := r.w.CloseSeason(ctx, r.sale.Org, expires)
	if err != nil || n != 1 || total != 8000 {
		t.Fatalf("close: %d %d %v", n, total, err)
	}
	w, _ := r.w.Wallet(ctx, r.sale.Org, fan.ID, "RON")
	if w.BalanceMinor != 0 || w.CreditMinor != 8000 || w.CreditExpiresAt == nil || !w.CreditExpiresAt.Equal(expires) {
		t.Fatalf("wallet %+v", w)
	}
	if len(w.History) != 3 || w.History[0].Kind != "season_close" {
		t.Fatalf("history %+v", w.History)
	}
	r.audit(t, r.sale.Org)
	// A stand charge spends the credit.
	if _, err := r.w.Stand(ctx, r.sale.Org, w.Code, 2500, "Pizza"); err != nil {
		t.Fatal(err)
	}
	w, _ = r.w.Wallet(ctx, r.sale.Org, fan.ID, "RON")
	if w.CreditMinor != 5500 {
		t.Fatalf("credit %d after a 25.00 stand payment", w.CreditMinor)
	}
	_ = checkout.Paid
}

// Expired credit cannot be spent.
func TestExpiredCreditNotSpendable(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	fan := salestest.Buyer()
	r.fund(t, r.sale.Org, fan.ID, 8000)
	if _, _, err := r.w.CloseSeason(ctx, r.sale.Org, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := r.env.Pool.Exec(ctx, "update sales.wallet_accounts set expires_at = now() - interval '1 minute' where kind = 'fan_season_credit'"); err != nil {
		t.Fatal(err)
	}
	w, _ := r.w.Wallet(ctx, r.sale.Org, fan.ID, "RON")
	if _, err := r.w.Stand(ctx, r.sale.Org, w.Code, 100, "Water"); code(err) != "EPS-WALLET-INSUFFICIENT" {
		t.Fatalf("spent expired credit: %v", err)
	}
}
