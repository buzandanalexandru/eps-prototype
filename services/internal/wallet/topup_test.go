package wallet_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wallet"
)

func waitAttempt(t *testing.T, r *rig, id, state string) *payments.Attempt {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		a, err := r.pay.GetAttempt(context.Background(), r.sale.Org, id)
		if err == nil && a.State == state {
			return a
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("attempt %s did not reach %s", id, state)
	return nil
}

// A confirmed top-up credits once, even when the same request is sent again.
func TestTopupConfirmedCreditsOnce(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	fan := salestest.Buyer()
	req := wallet.TopupRequest{AmountMinor: 20000, Currency: "RON", Method: payments.HostedCard, ReturnURL: "https://shop.example.test/wallet"}
	res, err := r.w.Topup(ctx, fan, r.sale.Org, req, "topup-key-0001")
	if err != nil {
		t.Fatal(err)
	}
	if b := r.balance(t, r.sale.Org, fan.ID); b != 0 {
		t.Fatalf("balance %d before the provider confirms", b)
	}
	again, err := r.w.Topup(ctx, fan, r.sale.Org, req, "topup-key-0001")
	if err != nil || again.Attempt.ID != res.Attempt.ID {
		t.Fatalf("replay: %v %v", again, err)
	}
	approve(t, res.ActionURL, "approve")
	waitAttempt(t, r, res.Attempt.ID, payments.Succeeded)
	if b := r.balance(t, r.sale.Org, fan.ID); b != 20000 {
		t.Fatalf("balance %d, want 200.00", b)
	}
	w, _ := r.w.Wallet(ctx, r.sale.Org, fan.ID, "RON")
	if len(w.History) != 1 || w.History[0].Kind != "topup" || w.History[0].AmountMinor != 20000 {
		t.Fatalf("history %+v", w.History)
	}
	r.audit(t, r.sale.Org)
}

// Scenario "Top-up declined": the balance does not change and the decline has its code.
func TestTopupDeclined(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	fan := salestest.Buyer()
	res, err := r.w.Topup(ctx, fan, r.sale.Org, wallet.TopupRequest{AmountMinor: 20000, Currency: "RON", Method: payments.HostedCard, ReturnURL: "https://shop.example.test/w"}, "topup-key-0002")
	if err != nil {
		t.Fatal(err)
	}
	approve(t, res.ActionURL, "decline_funds")
	a := waitAttempt(t, r, res.Attempt.ID, payments.Declined)
	if a.ErrorCode == nil || *a.ErrorCode != apperr.PayDeclined {
		t.Fatalf("code %v", a.ErrorCode)
	}
	if b := r.balance(t, r.sale.Org, fan.ID); b != 0 {
		t.Fatalf("balance %d after a decline", b)
	}
	// Outside the limits: refused before any charge.
	if _, err := r.w.Topup(ctx, fan, r.sale.Org, wallet.TopupRequest{AmountMinor: 5, Currency: "RON", Method: payments.HostedCard}, "topup-key-0003"); code(err) != apperr.WalletLimit {
		t.Fatalf("tiny top-up: %v", err)
	}
	// Guests have no wallet.
	if _, err := r.w.Topup(ctx, salestest.Guest(), r.sale.Org, wallet.TopupRequest{AmountMinor: 2000, Currency: "RON", Method: payments.HostedCard}, "topup-key-0004"); code(err) != apperr.Forbidden {
		t.Fatalf("guest: %v", err)
	}
}
