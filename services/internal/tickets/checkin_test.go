package tickets_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wallet"
)

func (x *env) topUp(t *testing.T, owner string, amount int64) {
	t.Helper()
	ctx := context.Background()
	if err := db.WithTx(ctx, x.Pool, db.RoleCore, x.sale.Org, func(tx pgx.Tx) error {
		bal, err := wallet.AccountTx(ctx, tx, x.sale.Org, wallet.FanBalance, owner, "RON")
		if err != nil {
			return err
		}
		clr, err := wallet.AccountTx(ctx, tx, x.sale.Org, wallet.PSPClearing, "a", "RON")
		if err != nil {
			return err
		}
		_, err = wallet.PostTx(ctx, tx, x.sale.Org, "topup", "test", "test top-up", "RON", []wallet.Line{{AccountID: clr, AmountMinor: -amount}, {AccountID: bal, AmountMinor: amount}})
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// Scenarios "Check in on time" (free) and "Late check-in" (the late fee, all-in, from the EPS
// Wallet; checked in only once it is paid).
func TestOnlineCheckIn(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	by := x.now.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	x.gaWith(t, func(p map[string]any) {
		p["checkinBy"] = by
		p["lateCheckin"] = map[string]any{"faceMinor": 2000, "allInMinor": 2050, "fanFeeMinor": 50, "organiserNetMinor": 2000}
	})
	b := salestest.Buyer()
	_, list := x.issued(t, b, ga(2))
	ev, err := x.svc.CheckIn(ctx, holder(b), list[0].ID, false)
	if err != nil || ev.Items[0].CheckedInAt == nil {
		t.Fatalf("on-time check-in: %+v %v", ev, err)
	}
	if n := x.count(t, "select count(*) from sales.journal_entries where organiser_id = $1 and ref like 'checkin:%'", x.sale.Org); n != 0 {
		t.Fatalf("an on-time check-in charged %d entries", n)
	}
	// Late: the fee is asked for, refused without the money, and paid from the wallet.
	x.now = x.now.Add(48 * time.Hour)
	if _, err := x.svc.CheckIn(ctx, holder(b), list[1].ID, false); code(err) != "EPS-TICKET-LATE-FEE" {
		t.Fatalf("late without paying: %v", err)
	}
	if _, err := x.svc.CheckIn(ctx, holder(b), list[1].ID, true); code(err) != "EPS-WALLET-INSUFFICIENT" {
		t.Fatalf("late with an empty wallet: %v", err)
	}
	x.topUp(t, b.ID, 5000)
	ev, err = x.svc.CheckIn(ctx, holder(b), list[1].ID, true)
	if err != nil || ev.Items[0].CheckedInAt == nil {
		t.Fatalf("late check-in: %v", err)
	}
	var bal int64
	if err := x.Pool.QueryRow(ctx, "select balance_minor from sales.wallet_accounts where owner = $1 and kind = 'fan_balance'", b.ID).Scan(&bal); err != nil || bal != 5000-2050 {
		t.Fatalf("balance %d (%v), want 2950: the all-in fee was taken", bal, err)
	}
	if _, err := x.svc.CheckIn(ctx, holder(b), list[1].ID, true); code(err) != tickets.CodeCheckedIn {
		t.Fatalf("second check-in: %v", err)
	}
	// A guest is asked to sign in to pay a late fee.
	g := salestest.Guest()
	_, gl := x.issued(t, g, ga(1))
	if _, err := x.svc.CheckIn(ctx, holder(g), gl[0].ID, true); code(err) != tickets.CodeSignIn {
		t.Fatalf("guest late check-in: %v", err)
	}
}
