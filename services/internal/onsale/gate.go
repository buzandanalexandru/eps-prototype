// Package onsale joins the on-sale engine to step 4's checkout (design D-11): the pass, the cap per
// order and the cap per person before a hold; the pass and the one-time code before a payment;
// and, from the order events, the end of each pass and cap entry.
package onsale

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
)

// Gate implements checkout.Gate.
type Gate struct {
	Sales  *sales.Service
	Redis  *redis.Client
	Pool   *pgxpool.Pool
	Passes *queue.PassCheck
	Caps   *botdefence.Caps
	Scorer *botdefence.Scorer
	OTP    *botdefence.OTP
	// Checkout cancels an order whose codes were all used and counts pre-sale-moment purchases.
	Checkout *checkout.Service
	// Secret keys the stored cap identity hashes (review sec L4).
	Secret []byte
	Now      func() time.Time
}

var _ checkout.Gate = (*Gate)(nil)

func (g *Gate) now() time.Time {
	if g.Now != nil {
		return g.Now()
	}
	return time.Now()
}

// PassRequired is the answer that sends the fan to the waiting room (never a block: the waiting
// room gives a pass).
func PassRequired(saleID, eventID string) *apperr.Problem {
	return apperr.New(http.StatusForbidden, apperr.PassRequired, "Join the waiting room to buy tickets for this sale.", true).
		With("saleId", saleID).With("eventId", eventID)
}

// badPass scores a pass with a good signature presented from another session by another account
// (D-6's "checkout with another session's pass", narrowed by review D-d): a person's own second
// browser, an expired or an ended pass is never scored, and a missing or forged pass names no place.
func (g *Gate) badPass(ctx context.Context, sale *sales.Sale, err error, sub string) {
	var r *queue.Rejected
	if g.Scorer == nil || !sale.Settings.BotDefence || !errors.As(err, &r) || r.Claims == nil || r.Reason != queue.RejectSession || r.Claims.Sub == sub {
		return
	}
	if k, e := queue.KeysOf(sale.OrganiserID, sale.EventID, sale.ID); e == nil {
		_, _ = g.Scorer.Add(ctx, k.Bot(r.Claims.QID), botdefence.BadPass)
	}
}

func (g *Gate) score(ctx context.Context, sale *sales.Sale, qid string) int {
	if g.Scorer == nil || qid == "" {
		return 0
	}
	k, err := queue.KeysOf(sale.OrganiserID, sale.EventID, sale.ID)
	if err != nil {
		return 0
	}
	n, _ := g.Scorer.Score(ctx, k.Bot(qid))
	return n
}

// paidTickets is the person's paid tickets for the event (the cap ledger, the record of truth).
func (g *Gate) paidTickets(ctx context.Context, org, event, identity string) (int, error) {
	var n int
	err := db.WithTx(ctx, g.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select coalesce(sum(tickets), 0) from sales.cap_ledger where event_id = $1 and identity_hash = $2`, event, identity).Scan(&n)
	})
	return n, err
}

func capReached(capN, left int) *apperr.Problem {
	return apperr.New(http.StatusUnprocessableEntity, apperr.CapReached, "You have reached the most tickets one person can buy for this event.", false).
		With("max", capN).With("left", left)
}

// BeforeHold checks a new hold (D-11): the event's open sale, its pass, the cap per order, then the
// cap per person (taken atomically once the deadline is known).
func (g *Gate) BeforeHold(ctx context.Context, h checkout.GateHold) (*checkout.GateResult, error) {
	list, err := g.Sales.Live(ctx, h.Org, h.EventID)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, nil // no sale moments: step 4's checkout as it was
	}
	now := g.now()
	sale, ok := sales.Current(list, now)
	if !ok {
		next := sales.Next(list, now)
		return nil, PassRequired(next.ID, h.EventID).With("opensAt", next.Settings.OpensAt)
	}
	st := sale.Settings
	c, err := g.Passes.Check(ctx, h.Buyer.Pass, h.Buyer.Session, h.Buyer.ID, h.Org, h.EventID, sale.ID, now, false)
	if err != nil {
		var r *queue.Rejected
		if errors.As(err, &r) {
			g.badPass(ctx, sale, err, h.Buyer.ID)
			return nil, PassRequired(sale.ID, h.EventID).With("reason", r.Reason)
		}
		return nil, err
	}
	if h.Tickets > st.CapPerOrder {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.HoldLimit, "Too many tickets in one order.", false).With("max", st.CapPerOrder)
	}
	capN := st.CapPerPerson
	if h.MaxPerAccount > 0 && h.MaxPerAccount < capN {
		capN = h.MaxPerAccount // the stricter of the two caps
	}
	ids := g.identities(h.Org, botdefence.Identities(h.Buyer.Kind, h.Buyer.ID, h.Buyer.Email))
	// The person's paid tickets: the ledger per identity (the most of them), plus what the buyer paid
	// before the event had sale moments (review D-g).
	legacy, err := g.Checkout.LegacyTickets(ctx, h.Org, h.EventID, h.Buyer.ID, h.Buyer.Email)
	if err != nil {
		return nil, err
	}
	base := map[string]int{}
	paid := 0
	for _, id := range ids {
		n, err := g.paidTickets(ctx, h.Org, h.EventID, id)
		if err != nil {
			return nil, err
		}
		base[id] = n + legacy
		paid = max(paid, n+legacy)
	}
	if paid+h.Tickets > capN {
		return nil, capReached(capN, max(0, capN-paid))
	}
	score := 0
	if st.BotDefence {
		score = g.score(ctx, sale, c.QID)
	}
	res := &checkout.GateResult{SaleID: sale.ID, PassQID: c.QID, CapIdentity: strings.Join(ids, ","), CapPerPerson: capN, CapTickets: h.Tickets,
		OTPRequired: st.OTPOn && (score >= botdefence.RiskyAt || h.Tickets > st.OTPThreshold), HoldMinutes: st.HoldMinutes}
	var taken []string
	undo := func(ctx context.Context) {
		for _, id := range taken {
			_ = g.Caps.Drop(ctx, h.Org, h.EventID, id, h.OrderID)
		}
	}
	res.Taken = func(ctx context.Context, until time.Time) error {
		for _, id := range ids {
			ok, left, err := g.Caps.Hold(ctx, h.Org, h.EventID, id, h.OrderID, h.Tickets, capN, base[id], until)
			if err != nil {
				undo(ctx)
				return err
			}
			if !ok {
				undo(ctx)
				return capReached(capN, left)
			}
			taken = append(taken, id)
		}
		return nil
	}
	res.Undo = undo
	return res, nil
}

// identities hashes a buyer's cap identities with the environment's key.
func (g *Gate) identities(org string, raw []string) []string {
	out := make([]string, len(raw))
	for i, r := range raw {
		out[i] = botdefence.IdentityHash(g.Secret, org, r)
	}
	return out
}

// BeforeDetails moves a guest's cap entry to the normalised email it gives (D-7), refusing an
// email whose person is already at the cap; an account with that verified email shares the cap
// (its orders count under the email too: review D-g).
func (g *Gate) BeforeDetails(ctx context.Context, o *checkout.Order, email string) (string, error) {
	if o.SaleID == nil || o.CapIdentity == nil || o.CapPerPerson == nil || o.CapTickets == nil {
		return "", nil
	}
	id := botdefence.IdentityHash(g.Secret, o.OrganiserID, botdefence.Identity("guest", o.BuyerID, email))
	if id == *o.CapIdentity {
		return id, nil
	}
	paid, err := g.paidTickets(ctx, o.OrganiserID, o.EventID, id)
	if err != nil {
		return "", err
	}
	legacy, err := g.Checkout.LegacyTickets(ctx, o.OrganiserID, o.EventID, "", email)
	if err != nil {
		return "", err
	}
	ok, left, err := g.Caps.Hold(ctx, o.OrganiserID, o.EventID, id, o.ID, *o.CapTickets, *o.CapPerPerson, paid+legacy, o.HoldExpiresAt.Add(checkout.AttemptLimit+checkout.HoldGrace))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", capReached(*o.CapPerPerson, left)
	}
	for _, old := range strings.Split(*o.CapIdentity, ",") {
		_ = g.Caps.Drop(ctx, o.OrganiserID, o.EventID, old, o.ID)
	}
	return id, nil
}

// saleOf is an order's sale as it stands now (nil for an order without one).
func (g *Gate) saleOf(ctx context.Context, o *checkout.Order) (*sales.Sale, error) {
	if o.SaleID == nil {
		return nil, nil
	}
	list, err := g.Sales.Live(ctx, o.OrganiserID, o.EventID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == *o.SaleID {
			return &list[i], nil
		}
	}
	return nil, nil
}

// BeforePay checks a payment (D-11): the order's own pass from the same session (its time may have
// run out: the hold bounds it), unless the payer opened a signed order link, and then the one-time
// code when the order needs one and OTP is still on.
func (g *Gate) BeforePay(ctx context.Context, o *checkout.Order, b checkout.Buyer, viaLink bool) error {
	sale, err := g.saleOf(ctx, o)
	if err != nil || sale == nil {
		return err
	}
	if !viaLink && o.PassQID != nil {
		c, err := g.Passes.Check(ctx, b.Pass, b.Session, o.BuyerID, o.OrganiserID, o.EventID, sale.ID, g.now(), true)
		if err != nil {
			var r *queue.Rejected
			if errors.As(err, &r) {
				g.badPass(ctx, sale, err, b.ID)
				return PassRequired(sale.ID, o.EventID).With("reason", r.Reason)
			}
			return err
		}
		if c.QID != *o.PassQID {
			return PassRequired(sale.ID, o.EventID).With("reason", queue.RejectSession)
		}
	}
	if o.OTPRequired && sale.Settings.OTPOn && o.OTPVerified == nil {
		return apperr.New(http.StatusConflict, apperr.OTPRequired, "Enter the code we sent to continue.", false)
	}
	return nil
}

// PassOnly is BeforePay's pass check alone (the OTP operations need the pass, not a code).
func (g *Gate) PassOnly(ctx context.Context, o *checkout.Order, b checkout.Buyer, viaLink bool) error {
	saved := o.OTPRequired
	o.OTPRequired = false
	defer func() { o.OTPRequired = saved }()
	return g.BeforePay(ctx, o, b, viaLink)
}

// OTPNeeded reports whether an order must enter a code before payment now.
func (g *Gate) OTPNeeded(ctx context.Context, o *checkout.Order) bool {
	sale, err := g.saleOf(ctx, o)
	return err == nil && sale != nil && o.OTPRequired && sale.Settings.OTPOn && o.OTPVerified == nil
}
