package checkout

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Order states (design D-1).
const (
	Held          = "held"
	Paying        = "paying"
	PaymentFailed = "payment_failed"
	Paid          = "paid"
	Issued        = "issued"
	Expired       = "expired"
	Cancelled     = "cancelled"
	Refunded      = "refunded"
)

// LiveStates hold stock without being sold.
var LiveStates = []string{Held, Paying, PaymentFailed}

// allowedFrom is the transition table: target state → states it may be entered from.
var allowedFrom = map[string][]string{
	Paying:        {Held, PaymentFailed},
	Paid:          {Paying, Held, PaymentFailed, Expired, Cancelled}, // the last two only for a late payment that re-took its stock
	PaymentFailed: {Paying},
	Expired:       {Held, PaymentFailed},
	Cancelled:     {Held, PaymentFailed},
	Issued:        {Paid},
	Refunded:      {Paying, PaymentFailed, Expired, Cancelled},
}

// CanMove reports whether the transition table allows from → to.
func CanMove(from, to string) bool {
	for _, f := range allowedFrom[to] {
		if f == from {
			return true
		}
	}
	return false
}

// Buyer is who places an order: an account (user id, and the email when it is verified) or a guest.
type Buyer struct {
	ID    string // user id, or guest:<id>
	Kind  string // account | guest
	Email string // verified email of an account ("" for guests)
}

// Line is one priced order line.
type Line struct {
	LineNo                int           `json:"lineNo"`
	ProductID             string        `json:"productId"`
	Name                  snapshot.L10n `json:"name"`
	Qty                   int           `json:"qty"`
	Instance              *string       `json:"instance,omitempty"`
	PriceStepIndex        int           `json:"priceStepIndex"`
	UnitAllInMinor        int64         `json:"unitAllInMinor"`
	UnitFaceMinor         int64         `json:"unitFaceMinor"`
	UnitFanFeeMinor       int64         `json:"unitFanFeeMinor"`
	UnitOrganiserNetMinor int64         `json:"unitOrganiserNetMinor"`
}

// HistoryEntry is one state change.
type HistoryEntry struct {
	From   *string   `json:"from"`
	To     string    `json:"to"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// Order is an order as stored.
type Order struct {
	ID                string           `json:"id"`
	OrganiserID       string           `json:"organiserId"`
	EventID           string           `json:"eventId"`
	SnapshotVersion   int              `json:"snapshotVersion"`
	BuyerKind         string           `json:"buyerKind"`
	BuyerID           string           `json:"-"`
	BuyerEmail        *string          `json:"buyerEmail,omitempty"`
	BuyerName         *string          `json:"buyerName,omitempty"`
	Locale            string           `json:"locale"`
	Currency          string           `json:"currency"`
	State             string           `json:"state"`
	TotalMinor        int64            `json:"totalMinor"`
	FaceMinor         int64            `json:"faceMinor"`
	FanFeeMinor       int64            `json:"fanFeeMinor"`
	OrganiserNetMinor int64            `json:"organiserNetMinor"`
	Draws             map[string]int64 `json:"draws"`
	HoldExpiresAt     time.Time        `json:"holdExpiresAt"`
	HoldRef           string           `json:"-"`
	PayPlan           string           `json:"payPlan"`
	Answers           map[string]any   `json:"answers"`
	DetailsGiven      bool             `json:"detailsGiven"`
	LastErrorCode     *string          `json:"lastErrorCode,omitempty"`
	CreatedAt         time.Time        `json:"createdAt"`
	PaidAt            *time.Time       `json:"paidAt,omitempty"`
	Lines             []Line           `json:"lines"`
	History           []HistoryEntry   `json:"history"`
}

// SnapshotSource is the published sale terms (snapshot.Store).
type SnapshotSource interface {
	Load(ctx context.Context, eventID string) (*snapshot.Snapshot, error)
	LoadVersion(ctx context.Context, eventID string, version int) (*snapshot.Snapshot, error)
}

// Service is the checkout: holds, orders and their states.
type Service struct {
	Pool  *pgxpool.Pool
	Inv   Inventory
	Snaps SnapshotSource
	Links *Links
	Log   *slog.Logger
	Now   func() time.Time
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *Service) log() *slog.Logger {
	if s.Log != nil {
		return s.Log
	}
	return slog.Default()
}

// ErrNotFound means no such order (or not visible to the caller).
var ErrNotFound = errors.New("checkout: order not found")

func notFound() *apperr.Problem {
	return apperr.New(http.StatusNotFound, apperr.OrderNotFound, "Order not found.", false)
}

// OrganiserOf finds the organiser of an order by its id ("" when there is none).
func (s *Service) OrganiserOf(ctx context.Context, orderID string) (string, error) {
	var org *string
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select sales.order_organiser($1::uuid)::text", orderID).Scan(&org)
	})
	if err != nil || org == nil {
		return "", err
	}
	return *org, nil
}

const orderCols = `id::text, organiser_id::text, event_id::text, snapshot_version, buyer_kind, buyer_id, buyer_email, buyer_name, locale, currency, state,
	total_minor, face_minor, fan_fee_minor, organiser_net_minor, draws, hold_expires_at, hold_ref, pay_plan, answers, details_given, last_error_code, created_at, paid_at`

func scanOrder(row pgx.Row) (*Order, error) {
	var o Order
	var draws, answers []byte
	err := row.Scan(&o.ID, &o.OrganiserID, &o.EventID, &o.SnapshotVersion, &o.BuyerKind, &o.BuyerID, &o.BuyerEmail, &o.BuyerName, &o.Locale, &o.Currency, &o.State,
		&o.TotalMinor, &o.FaceMinor, &o.FanFeeMinor, &o.OrganiserNetMinor, &draws, &o.HoldExpiresAt, &o.HoldRef, &o.PayPlan, &answers, &o.DetailsGiven, &o.LastErrorCode, &o.CreatedAt, &o.PaidAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(draws, &o.Draws)
	_ = json.Unmarshal(answers, &o.Answers)
	if o.Answers == nil {
		o.Answers = map[string]any{}
	}
	return &o, nil
}

// LoadTx reads an order with its lines and history inside a transaction (lock: FOR UPDATE).
func LoadTx(ctx context.Context, tx pgx.Tx, orderID string, lock bool) (*Order, error) {
	q := "select " + orderCols + " from sales.orders where id = $1"
	if lock {
		q += " for update"
	}
	o, err := scanOrder(tx.QueryRow(ctx, q, orderID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `select line_no, product_id::text, qty, instance_id, price_step_index, unit_all_in_minor, unit_face_minor, unit_fan_fee_minor, unit_organiser_net_minor
		from sales.order_lines where order_id = $1 order by line_no`, orderID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var l Line
		if err := rows.Scan(&l.LineNo, &l.ProductID, &l.Qty, &l.Instance, &l.PriceStepIndex, &l.UnitAllInMinor, &l.UnitFaceMinor, &l.UnitFanFeeMinor, &l.UnitOrganiserNetMinor); err != nil {
			rows.Close()
			return nil, err
		}
		o.Lines = append(o.Lines, l)
	}
	rows.Close()
	hrows, err := tx.Query(ctx, `select from_state, to_state, reason, at from sales.order_history where order_id = $1 order by seq`, orderID)
	if err != nil {
		return nil, err
	}
	for hrows.Next() {
		var h HistoryEntry
		if err := hrows.Scan(&h.From, &h.To, &h.Reason, &h.At); err != nil {
			hrows.Close()
			return nil, err
		}
		o.History = append(o.History, h)
	}
	hrows.Close()
	return o, hrows.Err()
}

// Get reads an order by id (under its organiser's scope), with product names from its snapshot.
func (s *Service) Get(ctx context.Context, orderID string) (*Order, error) {
	org, err := s.OrganiserOf(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if org == "" {
		return nil, ErrNotFound
	}
	return s.GetIn(ctx, org, orderID)
}

// GetIn reads an order of a known organiser.
func (s *Service) GetIn(ctx context.Context, org, orderID string) (*Order, error) {
	var o *Order
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var err error
		o, err = LoadTx(ctx, tx, orderID, false)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.name(ctx, o)
	return o, nil
}

// name fills product names from the order's snapshot version (best effort).
func (s *Service) name(ctx context.Context, o *Order) {
	if s.Snaps == nil {
		return
	}
	snap, err := s.Snaps.LoadVersion(ctx, o.EventID, o.SnapshotVersion)
	if err != nil {
		return
	}
	for i := range o.Lines {
		if p := snap.Product(o.Lines[i].ProductID); p != nil {
			o.Lines[i].Name = p.Name
		}
	}
}

// Transition moves an order inside tx, guarded by the transition table: the UPDATE succeeds only
// from an allowed state, so of two racing transitions exactly one wins. It returns the state the
// order was in and whether it moved. set adds column assignments (SQL with $4.. args).
func Transition(ctx context.Context, tx pgx.Tx, orderID, to, reason string, set string, args ...any) (string, bool, error) {
	from := allowedFrom[to]
	if len(from) == 0 {
		return "", false, errors.New("checkout: unknown target state " + to)
	}
	q := `with old as (select state from sales.orders where id = $1 for update)
		update sales.orders o set state = $2, updated_at = now()` + set + `
		from old where o.id = $1 and old.state = any($3) returning old.state`
	var prev string
	err := tx.QueryRow(ctx, q, append([]any{orderID, to, from}, args...)...).Scan(&prev)
	if errors.Is(err, pgx.ErrNoRows) {
		var cur string
		if err := tx.QueryRow(ctx, "select state from sales.orders where id = $1", orderID).Scan(&cur); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return "", false, ErrNotFound
			}
			return "", false, err
		}
		return cur, false, nil
	}
	if err != nil {
		return "", false, err
	}
	if _, err := tx.Exec(ctx, `insert into sales.order_history (organiser_id, order_id, from_state, to_state, reason)
		select organiser_id, id, $2, $3, $4 from sales.orders where id = $1`, orderID, prev, to, reason); err != nil {
		return "", false, err
	}
	return prev, true, nil
}

// emit writes an order event inside tx.
func emit(ctx context.Context, tx pgx.Tx, o *Order, typ string, extra map[string]any) error {
	payload := map[string]any{"orderId": o.ID, "eventId": o.EventID, "snapshotVersion": o.SnapshotVersion, "totalMinor": o.TotalMinor, "currency": o.Currency}
	for k, v := range extra {
		payload[k] = v
	}
	_, err := outbox.Write(ctx, tx, outbox.Event{Topic: outbox.TopicOrders, Type: typ, OrganiserID: o.OrganiserID, EventID: o.EventID, Payload: payload})
	return err
}

// stateProblem is the answer for an action the order's state does not allow.
func stateProblem(state string) *apperr.Problem {
	if state == Expired {
		return apperr.New(http.StatusConflict, apperr.HoldExpired, "The hold ended and the tickets were released.", false).With("state", state)
	}
	return apperr.New(http.StatusConflict, apperr.OrderState, "The order cannot do that in its current state.", false).With("state", state)
}
