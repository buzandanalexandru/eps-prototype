// Package onsale joins the on-sale engine to step 4's checkout (design D-11): the pass, the cap per
// order and the cap per person before a hold; the pass and the one-time code before a payment;
// and, from the order events, the end of each pass and cap entry.
package onsale

import (
	"context"
	"errors"
	"net/http"
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
	// Checkout cancels an order whose codes were all used.
	Checkout *checkout.Service
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

// badPass scores a refused pass whose signature was good (another session's pass): D-6's
// "checkout with another session's pass". A missing or forged pass has no place to score.
func (g *Gate) badPass(ctx context.Context, sale *sales.Sale, err error) {
	var r *queue.Rejected
	if g.Scorer == nil || !sale.Settings.BotDefence || !errors.As(err, &r) || r.Claims == nil {
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
			g.badPass(ctx, sale, err)
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
	identity := botdefence.IdentityHash(h.Org, botdefence.Identity(h.Buyer.Kind, h.Buyer.ID, h.Buyer.Email))
	paid, err := g.paidTickets(ctx, h.Org, h.EventID, identity)
	if err != nil {
		return nil, err
	}
	if paid+h.Tickets > capN {
		return nil, capReached(capN, max(0, capN-paid))
	}
	score := 0
	if st.BotDefence {
		score = g.score(ctx, sale, c.QID)
	}
	res := &checkout.GateResult{SaleID: sale.ID, PassQID: c.QID, CapIdentity: identity, CapPerPerson: capN, CapTickets: h.Tickets,
		OTPRequired: st.OTPOn && (score >= botdefence.RiskyAt || h.Tickets > st.OTPThreshold), HoldMinutes: st.HoldMinutes}
	res.Taken = func(ctx context.Context, until time.Time) error {
		ok, left, err := g.Caps.Hold(ctx, h.Org, h.EventID, identity, h.OrderID, h.Tickets, capN, paid, until)
		if err != nil {
			return err
		}
		if !ok {
			return capReached(capN, left)
		}
		return nil
	}
	res.Undo = func(ctx context.Context) { _ = g.Caps.Drop(ctx, h.Org, h.EventID, identity, h.OrderID) }
	return res, nil
}

// BeforeDetails moves a guest's cap entry to the normalised email it gives (D-7), refusing an
// email whose person is already at the cap.
func (g *Gate) BeforeDetails(ctx context.Context, o *checkout.Order, email string) (string, error) {
	if o.SaleID == nil || o.CapIdentity == nil || o.CapPerPerson == nil || o.CapTickets == nil {
		return "", nil
	}
	id := botdefence.IdentityHash(o.OrganiserID, botdefence.Identity("guest", o.BuyerID, email))
	if id == *o.CapIdentity {
		return id, nil
	}
	paid, err := g.paidTickets(ctx, o.OrganiserID, o.EventID, id)
	if err != nil {
		return "", err
	}
	ok, left, err := g.Caps.Hold(ctx, o.OrganiserID, o.EventID, id, o.ID, *o.CapTickets, *o.CapPerPerson, paid, o.HoldExpiresAt.Add(checkout.AttemptLimit+checkout.HoldGrace))
	if err != nil {
		return "", err
	}
	if !ok {
		return "", capReached(*o.CapPerPerson, left)
	}
	_ = g.Caps.Drop(ctx, o.OrganiserID, o.EventID, *o.CapIdentity, o.ID)
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
				g.badPass(ctx, sale, err)
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
