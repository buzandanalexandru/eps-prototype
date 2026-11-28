package wallet_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/wallet"
)

// Scenario "Balanced entries": after a top-up, a payment, a stand payment and a season close
// every entry adds up to zero and every cached balance equals its lines.
func TestBalancedEntries(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	fan := salestest.Buyer()
	r.fund(t, r.sale.Org, fan.ID, 60000)
	o := r.order(t, r.sale, fan, salestest.ProductPlus, 1)
	if _, err := r.w.PayOrder(ctx, fan, o, "wallet-pay-0001"); err != nil {
		t.Fatal(err)
	}
	w, _ := r.w.Wallet(ctx, r.sale.Org, fan.ID, "RON")
	if _, err := r.w.Stand(ctx, r.sale.Org, w.Code, 2500, "Lemonade"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := r.w.CloseSeason(ctx, r.sale.Org, time.Now().AddDate(1, 0, 0)); err != nil {
		t.Fatal(err)
	}
	r.audit(t, r.sale.Org)
	// A correction is a reversing entry; the ledger still balances.
	err := db.WithTx(ctx, r.env.Pool, db.RoleCore, r.sale.Org, func(tx pgx.Tx) error {
		var id string
		if err := tx.QueryRow(ctx, "select id::text from sales.journal_entries where kind = 'stand_payment'").Scan(&id); err != nil {
			return err
		}
		_, err := wallet.ReverseTx(ctx, tx, r.sale.Org, id, "stand refund")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	r.audit(t, r.sale.Org)
	// Lines can never be edited.
	_, err = r.env.Pool.Exec(ctx, "update sales.journal_lines set amount_minor = amount_minor + 1")
	if err == nil {
		t.Fatal("a journal line was edited")
	}
}

// Scenario "Unbalanced entry refused": the transaction rolls back and no balance moves.
func TestUnbalancedEntryRefused(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	fan := salestest.Buyer()
	r.fund(t, r.sale.Org, fan.ID, 10000)
	err := db.WithTx(ctx, r.env.Pool, db.RoleCore, r.sale.Org, func(tx pgx.Tx) error {
		bal, _ := wallet.AccountTx(ctx, tx, r.sale.Org, wallet.FanBalance, fan.ID, "RON")
		sales, _ := wallet.AccountTx(ctx, tx, r.sale.Org, wallet.OrganiserSales, r.sale.Org, "RON")
		_, err := wallet.PostTx(ctx, tx, r.sale.Org, "order_payment", "x", "", "RON", []wallet.Line{{AccountID: bal, AmountMinor: -5000}, {AccountID: sales, AmountMinor: 4000}})
		return err
	})
	if err == nil {
		t.Fatal("an unbalanced entry committed")
	}
	if b := r.balance(t, r.sale.Org, fan.ID); b != 10000 {
		t.Fatalf("balance %d after the refused entry", b)
	}
	r.audit(t, r.sale.Org)
}

// Scenario "Other organiser's shop": a NIBIRU balance is never spendable at another organiser.
func TestWalletPerOrganiser(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	other := r.env.NewSale(t, 100, 100)
	fan := salestest.Buyer()
	r.fund(t, r.sale.Org, fan.ID, 30000)
	o := r.order(t, other, fan, salestest.ProductGA, 1)
	if _, err := r.w.PayOrder(ctx, fan, o, "wallet-pay-other"); code(err) != apperr.WalletInsufficient {
		t.Fatalf("paying at another organiser: %v", err)
	}
	if b := r.balance(t, r.sale.Org, fan.ID); b != 30000 {
		t.Fatalf("NIBIRU balance %d", b)
	}
	if got, _ := r.env.Svc.GetIn(ctx, other.Org, o.ID); got.State != checkout.Held {
		t.Fatalf("order %s, the hold stays", got.State)
	}
	ws, err := r.w.Wallets(ctx, fan.ID)
	if err != nil || len(ws) != 1 || ws[0].OrganiserID != r.sale.Org {
		t.Fatalf("wallets %v %v: each organiser's balance separately", ws, err)
	}
}
