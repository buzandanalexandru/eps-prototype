package tickets

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Ticket states.
const (
	Valid     = "valid"
	Suspended = "suspended"
	Cancelled = "cancelled"
)

// Problem codes of this package.
const (
	CodeNotFound   = "EPS-TICKET-NOT-FOUND"
	CodeMoved      = "EPS-TICKET-MOVED"
	CodeSuspended  = "EPS-TICKET-SUSPENDED"
	CodeCancelled  = "EPS-TICKET-CANCELLED"
	CodeNeedsName  = "EPS-TICKET-NEEDS-NAME"
	CodeNameLocked = "EPS-TICKET-NAME-LOCKED"
	CodeConflict   = "EPS-TICKET-CONFLICT"
	CodeNoCheckIn  = "EPS-TICKET-NO-CHECKIN"
	CodeCheckedIn  = "EPS-TICKET-CHECKED-IN"
	CodeDayPicked  = "EPS-TICKET-DAY-PICKED"
	CodeBadDay     = "EPS-TICKET-BAD-DAY"
	CodeEnded      = "EPS-TICKET-EVENT-ENDED"
	CodeRateLimit  = "EPS-RATE-LIMITED"
	CodeSignIn     = "EPS-TICKET-SIGN-IN"
	CodeLiveOrder  = "EPS-ERASE-LIVE-ORDER"
)

// ErrConflict means a concurrent reissue changed the ticket first.
var ErrConflict = errors.New("tickets: ticket changed concurrently")

func notFound() *apperr.Problem {
	return apperr.New(http.StatusNotFound, CodeNotFound, "Ticket not found.", false)
}

// Ticket is a ticket as stored.
type Ticket struct {
	ID              string     `json:"id"`
	OrganiserID     string     `json:"organiserId"`
	EventID         string     `json:"eventId"`
	OrderID         string     `json:"orderId"`
	LineNo          int        `json:"lineNo"`
	UnitNo          int        `json:"unitNo"`
	ProductID       string     `json:"productId"`
	InstanceID      *string    `json:"instance,omitempty"`
	SnapshotVersion int        `json:"snapshotVersion"`
	Generation      int        `json:"generation"`
	HolderKind      string     `json:"holderKind"`
	HolderID        string     `json:"-"`
	PrevHolderID    *string    `json:"-"`
	State           string     `json:"state"`
	CancelReason    *string    `json:"cancelReason,omitempty"`
	HolderName      *string    `json:"holderName,omitempty"`
	PickedDay       *string    `json:"pickedDay,omitempty"`
	CheckedInAt     *time.Time `json:"checkedInAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

const ticketCols = `id::text, organiser_id::text, event_id::text, order_id::text, line_no, unit_no, product_id::text, instance_id, snapshot_version,
	generation, holder_kind, holder_id, prev_holder_id, state, cancel_reason, holder_name, to_char(picked_day, 'YYYY-MM-DD'), checked_in_at, created_at`

func scanTicket(r pgx.Row) (*Ticket, error) {
	var t Ticket
	err := r.Scan(&t.ID, &t.OrganiserID, &t.EventID, &t.OrderID, &t.LineNo, &t.UnitNo, &t.ProductID, &t.InstanceID, &t.SnapshotVersion,
		&t.Generation, &t.HolderKind, &t.HolderID, &t.PrevHolderID, &t.State, &t.CancelReason, &t.HolderName, &t.PickedDay, &t.CheckedInAt, &t.CreatedAt)
	return &t, err
}

func queryTickets(ctx context.Context, tx pgx.Tx, where string, args ...any) ([]*Ticket, error) {
	rows, err := tx.Query(ctx, "select "+ticketCols+" from sales.tickets where "+where+" order by event_id, order_id, line_no, unit_no", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Ticket
	for rows.Next() {
		t, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Service is the tickets side of the core and the worker.
type Service struct {
	Pool  *pgxpool.Pool
	Snaps checkout.SnapshotSource
	Keys  *KeyRing
	Links *checkout.Links
	Redis *redis.Client
	// Pay charges a late check-in fee from the holder's EPS Wallet inside tx (wallet.Service).
	Pay func(ctx context.Context, tx pgx.Tx, org, owner, currency string, amount int64, ref, memo string) (string, error)
	// IdentitySecret keys the waiting-room identity hashes (the erasure recomputes them).
	IdentitySecret []byte
	Log            *slog.Logger
	Now            func() time.Time
	// CodesPerMinute limits code requests per holder (0: 30).
	CodesPerMinute int
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

var (
	issuedC    = metrics.NewCounter("eps_tickets_issued_total", "Tickets issued from confirmed payments.")
	reissuedC  = metrics.NewCounter("eps_tickets_reissued_total", "Ticket reissues (a new generation).")
	codesC     = metrics.NewCounter("eps_ticket_code_requests_total", "Code buffers served to holders.")
	issueLag   = metrics.NewHistogram("eps_ticket_issue_lag_seconds", "Seconds from an order's payment to its tickets.", []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 30, 60, 300})
	backfilled = metrics.NewCounter("eps_tickets_backfilled_orders_total", "Orders issued by the backstop or the one-off backfill.")
)

// ---------------------------------------------------------------------------------------------
// Issuing (design D1)

// Issue issues one ticket per paid unit of an order, idempotently, in one transaction under the
// order's row lock: tickets with the unit key (order, line, unit), the order moved to issued and
// one order.issued event listing the new tickets. A rehearsal order, or one that is not paid, gets
// none (checkout.Issuable). It returns how many tickets it made.
func (s *Service) Issue(ctx context.Context, org, orderID string) (int, error) {
	n := 0
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		n, err = s.IssueTx(ctx, tx, orderID)
		return err
	})
	return n, err
}

// IssueTx is Issue inside the caller's transaction (organiser scope).
func (s *Service) IssueTx(ctx context.Context, tx pgx.Tx, orderID string) (int, error) {
	o, err := checkout.LoadTx(ctx, tx, orderID, true)
	if err != nil {
		if errors.Is(err, checkout.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	if !checkout.Issuable(o) || (o.State != checkout.Paid && o.State != checkout.Issued) {
		return 0, nil
	}
	var lines, units []int32
	var products []string
	var instances []*string
	for _, l := range o.Lines {
		for u := 1; u <= l.Qty; u++ {
			lines = append(lines, int32(l.LineNo))
			units = append(units, int32(u))
			products = append(products, l.ProductID)
			instances = append(instances, l.Instance)
		}
	}
	rows, err := tx.Query(ctx, `insert into sales.tickets (id, organiser_id, event_id, order_id, line_no, unit_no, product_id, instance_id, snapshot_version, holder_kind, holder_id)
		select gen_random_uuid(), $1, $2, $3, u.line_no, u.unit_no, u.product_id::uuid, u.instance_id, $4, $5, $6
		from unnest($7::int[], $8::int[], $9::text[], $10::text[]) as u(line_no, unit_no, product_id, instance_id)
		on conflict (organiser_id, order_id, line_no, unit_no) do nothing returning id::text`,
		o.OrganiserID, o.EventID, o.ID, o.SnapshotVersion, o.BuyerKind, o.BuyerID, lines, units, products, instances)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if o.State == checkout.Paid {
		if _, _, err := checkout.Transition(ctx, tx, o.ID, checkout.Issued, "tickets issued", ""); err != nil {
			return 0, err
		}
	}
	if len(ids) > 0 {
		if _, err := outbox.Write(ctx, tx, outbox.Event{Topic: outbox.TopicOrders, Type: "order.issued", OrganiserID: o.OrganiserID, EventID: o.EventID,
			Payload: map[string]any{"orderId": o.ID, "eventId": o.EventID, "snapshotVersion": o.SnapshotVersion, "ticketIds": ids}}); err != nil {
			return 0, err
		}
		issuedC.Add(int64(len(ids)))
		if o.PaidAt != nil {
			issueLag.Observe(s.now().Sub(*o.PaidAt).Seconds())
		}
	}
	// An instalment plan already overdue when the tickets appear suspends them at once.
	if err := s.syncSuspensionTx(ctx, tx, o); err != nil {
		return 0, err
	}
	return len(ids), nil
}

// ---------------------------------------------------------------------------------------------
// The door feed and reissuing (design D6)

// FeedTx appends a row to an event's door feed with the event's next sequence number. The
// counter's row lock orders concurrent writers, so rows commit in sequence order.
func FeedTx(ctx context.Context, tx pgx.Tx, org, eventID, kind, ticketID string, generation *int, data map[string]any) (int64, error) {
	var seq int64
	if err := tx.QueryRow(ctx, `insert into sales.door_counters (organiser_id, event_id, seq) values ($1, $2, 1)
		on conflict (organiser_id, event_id) do update set seq = sales.door_counters.seq + 1 returning seq`, org, eventID).Scan(&seq); err != nil {
		return 0, err
	}
	if data == nil {
		data = map[string]any{}
	}
	_, err := tx.Exec(ctx, `insert into sales.door_feed (organiser_id, event_id, seq, kind, ticket_id, generation, data) values ($1, $2, $3, $4, $5, $6, $7)`,
		org, eventID, seq, kind, ticketID, generation, data)
	return seq, err
}

// Change is what a reissue changes besides the generation.
type Change struct {
	Reason     string // claimed, transferred, resold, upgraded, day_picked, cancelled, erased
	HolderKind string // a new holder ("" keeps the holder)
	HolderID   string
	PickedDay  string // a day pick ("" keeps it)
	Cancel     bool
	ClearName  bool
}

// ReissueTx moves a ticket to its next generation in the caller's transaction (organiser scope) and
// records the revocation of every earlier generation in the event's door feed: the primitive that
// claims, day picks and cancellations use here, and transfers, resale and upgrades later. It
// changes the ticket only if it is still at generation `expect`, so two concurrent reissues cannot
// both succeed: the second waits for the first's row lock and then finds another generation
// (ErrConflict).
func ReissueTx(ctx context.Context, tx pgx.Tx, ticketID string, expect int, c Change) (int, error) {
	if expect >= 65535 {
		return 0, ErrConflict
	}
	set := "generation = generation + 1, updated_at = now()"
	args := []any{ticketID, expect}
	if c.HolderID != "" {
		args = append(args, c.HolderKind, c.HolderID)
		set += ", prev_holder_id = holder_id, holder_kind = $3, holder_id = $4"
	}
	if c.PickedDay != "" {
		args = append(args, c.PickedDay)
		set += ", picked_day = $" + strconv.Itoa(len(args)) + "::date"
	}
	if c.Cancel {
		args = append(args, c.Reason)
		set += ", state = 'cancelled', cancel_reason = $" + strconv.Itoa(len(args))
	}
	if c.ClearName {
		set += ", holder_name = null"
	}
	var org, event string
	var gen int
	err := tx.QueryRow(ctx, "update sales.tickets set "+set+" where id = $1 and generation = $2 and state <> 'cancelled' returning organiser_id::text, event_id::text, generation", args...).Scan(&org, &event, &gen)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrConflict
	}
	if err != nil {
		return 0, err
	}
	data := map[string]any{"reason": c.Reason}
	if c.PickedDay != "" {
		data["day"] = c.PickedDay
	}
	if _, err := FeedTx(ctx, tx, org, event, "revoked", ticketID, &gen, data); err != nil {
		return 0, err
	}
	reissuedC.Inc()
	return gen, nil
}

// Reissue runs ReissueTx in its own transaction.
func (s *Service) Reissue(ctx context.Context, org, ticketID string, expect int, c Change) (int, error) {
	var gen int
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		gen, err = ReissueTx(ctx, tx, ticketID, expect, c)
		return err
	})
	return gen, err
}

// ---------------------------------------------------------------------------------------------
// Order events: suspension, cancellation, claims. Each handler recomputes the tickets from the
// order's current truth under its row lock, so a duplicate or out-of-order delivery converges.

// SyncOrder brings an order's tickets in line with the order: issued if paid, suspended while its
// instalment plan has an overdue payment, cancelled when the order was cancelled or refunded, and
// moved to the account that claimed a guest order.
func (s *Service) SyncOrder(ctx context.Context, org, orderID string) error {
	return db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		o, err := checkout.LoadTx(ctx, tx, orderID, true)
		if errors.Is(err, checkout.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		switch o.State {
		case checkout.Paid, checkout.Issued:
			if _, err := s.IssueTx(ctx, tx, orderID); err != nil {
				return err
			}
			return s.claimTx(ctx, tx, o)
		case checkout.Cancelled, checkout.Refunded:
			return s.cancelOrderTx(ctx, tx, o, "order_"+o.State)
		}
		return nil
	})
}

// syncSuspensionTx suspends the order's valid tickets while its active plan has an overdue
// payment, and lifts the suspension once none is overdue (design D6: a suspension and its lifting
// are door-feed rows).
func (s *Service) syncSuspensionTx(ctx context.Context, tx pgx.Tx, o *checkout.Order) error {
	p, err := checkout.PlanTx(ctx, tx, o.ID)
	if err != nil {
		return err
	}
	overdue := false
	if p != nil && p.State == "active" {
		for _, part := range p.Parts {
			if part.State == "overdue" {
				overdue = true
			}
		}
	}
	from, to, kind := Valid, Suspended, "suspended"
	if !overdue {
		from, to, kind = Suspended, Valid, "lifted"
	}
	rows, err := tx.Query(ctx, `update sales.tickets set state = $2, updated_at = now() where order_id = $1 and state = $3 returning id::text, generation`, o.ID, to, from)
	if err != nil {
		return err
	}
	type ch struct {
		id  string
		gen int
	}
	var changed []ch
	for rows.Next() {
		var c ch
		if err := rows.Scan(&c.id, &c.gen); err != nil {
			rows.Close()
			return err
		}
		changed = append(changed, c)
	}
	rows.Close()
	for _, c := range changed {
		g := c.gen
		if _, err := FeedTx(ctx, tx, o.OrganiserID, o.EventID, kind, c.id, &g, map[string]any{"reason": "instalment"}); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (s *Service) cancelOrderTx(ctx context.Context, tx pgx.Tx, o *checkout.Order, reason string) error {
	list, err := queryTickets(ctx, tx, "order_id = $1 and state <> 'cancelled'", o.ID)
	if err != nil {
		return err
	}
	for _, t := range list {
		if _, err := ReissueTx(ctx, tx, t.ID, t.Generation, Change{Reason: reason, Cancel: true}); err != nil {
			return err
		}
	}
	return nil
}

// claimTx moves a claimed guest order's tickets that its guest still holds to the account now
// buying it (a reissue: the guest link's codes stop).
func (s *Service) claimTx(ctx context.Context, tx pgx.Tx, o *checkout.Order) error {
	if o.BuyerKind != "account" {
		return nil
	}
	list, err := queryTickets(ctx, tx, "order_id = $1 and holder_kind = 'guest' and state <> 'cancelled'", o.ID)
	if err != nil {
		return err
	}
	for _, t := range list {
		if _, err := ReissueTx(ctx, tx, t.ID, t.Generation, Change{Reason: "claimed", HolderKind: "account", HolderID: o.BuyerID}); err != nil {
			return err
		}
	}
	return nil
}

// HandleEvent is the worker's issuer consumer (it replaces step 4's stub): order.paid issues,
// instalment events sync the suspension, cancellation and refund cancel, a claim moves the
// tickets. Each recomputes from the order's truth.
func (s *Service) HandleEvent(ctx context.Context, e outbox.Envelope) error {
	switch e.Type {
	case "order.paid", "order.claimed", "order.cancelled", "order.refunded":
	case "order.instalment_overdue", "order.instalment_paid":
	default:
		return nil
	}
	id := orderIDOf(e.Payload)
	if id == "" || e.OrganiserID == "" {
		return nil
	}
	if e.Type == "order.instalment_overdue" || e.Type == "order.instalment_paid" {
		return db.WithTx(ctx, s.Pool, db.RoleCore, e.OrganiserID, func(tx pgx.Tx) error {
			o, err := checkout.LoadTx(ctx, tx, id, true)
			if errors.Is(err, checkout.ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			return s.syncSuspensionTx(ctx, tx, o)
		})
	}
	return s.SyncOrder(ctx, e.OrganiserID, id)
}

// Backstop issues orders paid at least age ago that still have no tickets (a lost event), at most
// limit per call. It returns how many orders it issued.
func (s *Service) Backstop(ctx context.Context, age time.Duration, limit int) (int, error) {
	type ref struct{ org, id string }
	var refs []ref
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select organiser_id::text, order_id::text from sales.paid_orders_unissued($1, $2)", age, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r ref
			if err := rows.Scan(&r.org, &r.id); err != nil {
				return err
			}
			refs = append(refs, r)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range refs {
		if err := s.SyncOrder(ctx, r.org, r.id); err != nil {
			return n, err
		}
		n++
	}
	backfilled.Add(int64(n))
	return n, nil
}

// Backfill issues tickets for orders that step 4's stub marked issued without tickets, a page at
// a time in id order (one-off after the step 7 deploy; idempotent, resumable from `after`). It
// returns the last order id it handled ("" when none was left) and how many it issued.
func (s *Service) Backfill(ctx context.Context, after string, limit int) (string, int, error) {
	if after == "" {
		after = "00000000-0000-0000-0000-000000000000"
	}
	type ref struct{ org, id string }
	var refs []ref
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "select organiser_id::text, order_id::text from sales.issued_orders_without_tickets($1, $2)", after, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var r ref
			if err := rows.Scan(&r.org, &r.id); err != nil {
				return err
			}
			refs = append(refs, r)
		}
		return rows.Err()
	})
	if err != nil || len(refs) == 0 {
		return "", 0, err
	}
	n := 0
	for _, r := range refs {
		if _, err := s.Issue(ctx, r.org, r.id); err != nil {
			return r.id, n, err
		}
		n++
	}
	backfilled.Add(int64(n))
	return refs[len(refs)-1].id, n, nil
}

// RunBackfill walks the whole backfill once (pages of 200, a short pause between), then returns.
func (s *Service) RunBackfill(ctx context.Context) (int, error) {
	after, total := "", 0
	for ctx.Err() == nil {
		last, n, err := s.Backfill(ctx, after, 200)
		total += n
		if err != nil || last == "" {
			return total, err
		}
		after = last
		select {
		case <-ctx.Done():
		case <-time.After(100 * time.Millisecond):
		}
	}
	return total, ctx.Err()
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }

func orderIDOf(payload []byte) string {
	var p struct {
		OrderID string `json:"orderId"`
	}
	if jsonUnmarshal(payload, &p) != nil {
		return ""
	}
	if _, err := uuid.Parse(p.OrderID); err != nil {
		return ""
	}
	return p.OrderID
}

// ---------------------------------------------------------------------------------------------
// Holder reads

// Holder is the caller: an account id, or guest:<id>.
type Holder struct {
	ID   string
	Kind string // account | guest
}

// organiserOf finds the organiser of a ticket (SECURITY DEFINER index function).
func (s *Service) organiserOf(ctx context.Context, ticketID string) (string, error) {
	if _, err := uuid.Parse(ticketID); err != nil {
		return "", notFound()
	}
	var org *string
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select sales.ticket_organiser($1)::text", ticketID).Scan(&org)
	})
	if err != nil {
		return "", err
	}
	if org == nil {
		return "", notFound()
	}
	return *org, nil
}

// loadForHolder reads a ticket the holder holds; anyone else gets "not found", except the holder
// before its last reissue, who is told it moved.
func loadForHolder(ctx context.Context, tx pgx.Tx, ticketID string, h Holder, lock bool) (*Ticket, error) {
	q := "select " + ticketCols + " from sales.tickets where id = $1"
	if lock {
		q += " for update"
	}
	t, err := scanTicket(tx.QueryRow(ctx, q, ticketID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, notFound()
	}
	if err != nil {
		return nil, err
	}
	if t.HolderID != h.ID {
		if t.PrevHolderID != nil && *t.PrevHolderID == h.ID {
			return nil, apperr.New(http.StatusGone, CodeMoved, "This ticket has moved to someone else.", false)
		}
		return nil, notFound()
	}
	return t, nil
}

// Codes are a holder's buffer of signed codes.
type Codes struct {
	ServerTime time.Time `json:"serverTime"`
	TicketID   string    `json:"ticketId"`
	Generation int       `json:"generation"`
	WindowSecs int       `json:"windowSeconds"`
	Codes      []Code    `json:"codes"`
}

// Code is one window's signed code.
type Code struct {
	Window   uint32    `json:"window"`
	ValidAt  time.Time `json:"validAt"`
	Code     string    `json:"code"`
	ExpireAt time.Time `json:"expiresAt"`
}

// CodesFor returns the current window's code and the next 39 (design D4) to the ticket's current
// holder, for its current generation. A ticket that is suspended, cancelled or still waiting for
// its name gets no code.
func (s *Service) CodesFor(ctx context.Context, h Holder, ticketID string) (*Codes, error) {
	if s.Keys == nil {
		return nil, ErrNoKey
	}
	if err := s.limitCodes(ctx, h.ID); err != nil {
		return nil, err
	}
	org, err := s.organiserOf(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	var t *Ticket
	if err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		t, err = loadForHolder(ctx, tx, ticketID, h, false)
		return err
	}); err != nil {
		return nil, err
	}
	return s.codesOf(ctx, t)
}

func (s *Service) codesOf(ctx context.Context, t *Ticket) (*Codes, error) {
	switch t.State {
	case Suspended:
		return nil, apperr.New(http.StatusConflict, CodeSuspended, "Payment overdue: pay the instalment to show your code.", false)
	case Cancelled:
		return nil, apperr.New(http.StatusConflict, CodeCancelled, "This ticket was cancelled.", false)
	}
	snap, _ := s.Snaps.LoadVersion(ctx, t.EventID, t.SnapshotVersion)
	if snap != nil {
		if p := snap.Product(t.ProductID); p != nil && p.Named && t.HolderName == nil {
			return nil, apperr.New(http.StatusConflict, CodeNeedsName, "Name this ticket to show its code.", false)
		}
		if end, ok := eventEnd(snap); ok && s.now().After(end.Add(12*time.Hour)) {
			return nil, apperr.New(http.StatusConflict, CodeEnded, "The event has ended.", false)
		}
	}
	now := s.now()
	w := WindowOf(now)
	out := &Codes{ServerTime: now.UTC(), TicketID: t.ID, Generation: t.Generation, WindowSecs: int(Window / time.Second), Codes: make([]Code, 0, BufferWindows)}
	for i := uint32(0); i < BufferWindows; i++ {
		c, err := s.Keys.Sign(t.EventID, t.ID, uint16(t.Generation), w+i)
		if err != nil {
			return nil, err
		}
		out.Codes = append(out.Codes, Code{Window: w + i, ValidAt: WindowStart(w + i), ExpireAt: WindowStart(w + i + 1), Code: c})
	}
	codesC.Inc()
	return out, nil
}

// limitCodes allows CodesPerMinute buffers per holder per minute (each signs 40 codes).
func (s *Service) limitCodes(ctx context.Context, holder string) error {
	if s.Redis == nil {
		return nil
	}
	lim := s.CodesPerMinute
	if lim <= 0 {
		lim = 30
	}
	k, _ := redisx.PlatformKey("rl", "codes", holderKey(holder), strconv.FormatInt(s.now().Unix()/60, 10))
	n, err := s.Redis.Incr(ctx, k).Result()
	if err != nil {
		return nil // Redis down: codes still work (the holder check is in Postgres)
	}
	if n == 1 {
		_ = s.Redis.Expire(ctx, k, 70*time.Second).Err()
	}
	if n > int64(lim) {
		return apperr.New(http.StatusTooManyRequests, CodeRateLimit, "Too many code requests. Wait a minute.", true)
	}
	return nil
}

func holderKey(h string) string {
	out := make([]byte, 0, len(h))
	for i := 0; i < len(h) && i < 80; i++ {
		c := h[i]
		if c == ':' || c == '{' || c == '}' || c == ' ' {
			c = '_'
		}
		out = append(out, c)
	}
	return string(out)
}

func eventEnd(snap *snapshot.Snapshot) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, snap.Event.EndsAt)
	return t, err == nil
}

func eventStart(snap *snapshot.Snapshot) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, snap.Event.StartsAt)
	return t, err == nil
}
