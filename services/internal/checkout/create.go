package checkout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/idem"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Item is one requested line: a product, a quantity and, for day and slot products, the day
// (YYYY-MM-DD) or the slot's local start (YYYY-MM-DDTHH:MM).
type Item struct {
	ProductID string `json:"productId"`
	Qty       int    `json:"qty"`
	Instance  string `json:"instance,omitempty"`
}

// CreateRequest asks for a hold.
type CreateRequest struct {
	EventID string `json:"eventId"`
	Version int    `json:"version"`
	Items   []Item `json:"items"`
	Locale  string `json:"locale"`
}

var holdsCreated = metrics.NewCounter("eps_holds_created_total", "Holds taken for new orders.")

// Hold length rules (D-7).
const (
	DefaultHoldMinutes = 8
	MinHoldMinutes     = 5
	MaxHoldMinutes     = 15
)

// HoldMinutes clamps the snapshot's hold length to 5..15 (8 when unset).
func HoldMinutes(r snapshot.Rules) int {
	m := r.HoldMinutes
	if m == 0 {
		m = DefaultHoldMinutes
	}
	if m < MinHoldMinutes {
		m = MinHoldMinutes
	}
	if m > MaxHoldMinutes {
		m = MaxHoldMinutes
	}
	return m
}

var instancePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}(T([01]\d|2[0-3]):[0-5]\d)?$`)

func invalid(detail string) *apperr.Problem {
	return apperr.New(http.StatusUnprocessableEntity, apperr.HoldInvalid, detail, false)
}

// Capacities are the live snapshot's capacity per pool instance.
func Capacities(s *snapshot.Snapshot) map[string]int64 {
	out := map[string]int64{}
	for _, p := range s.Pools {
		for _, in := range p.Instances {
			out[in.InstanceID] = in.Capacity
		}
	}
	return out
}

// drawsOf is the pool instances one unit of a product draws on for a line's day or slot.
func drawsOf(s *snapshot.Snapshot, p *snapshot.Product, instance string) ([]string, error) {
	pools := map[string]snapshot.Pool{}
	for _, pl := range s.Pools {
		pools[pl.PoolID] = pl
	}
	caps := Capacities(s)
	var out []string
	needsChoice := false
	for _, id := range p.PoolIDs {
		pl, ok := pools[id]
		if !ok {
			return nil, fmt.Errorf("pool %s missing", id)
		}
		var inst string
		switch pl.Scope {
		case "event":
			inst = id
		case "day":
			needsChoice = true
			if instance == "" {
				return nil, errors.New("choose a day")
			}
			inst = id + "@" + instance[:10]
		case "slot":
			needsChoice = true
			if len(instance) != 16 {
				return nil, errors.New("choose a start time")
			}
			inst = id + "@" + instance
		default:
			return nil, fmt.Errorf("pool scope %s", pl.Scope)
		}
		if _, ok := caps[inst]; !ok {
			return nil, fmt.Errorf("no such day or slot: %s", instance)
		}
		out = append(out, inst)
	}
	if !needsChoice && instance != "" {
		return nil, errors.New("this product has no day or slot")
	}
	if p.Type == "day_ticket" && instance != "" {
		var cfg struct {
			Day *string `json:"day"`
		}
		_ = json.Unmarshal(p.Config, &cfg)
		if cfg.Day != nil && *cfg.Day != "" && instance[:10] != *cfg.Day {
			return nil, errors.New("this ticket is for another day")
		}
	}
	if len(out) == 0 {
		return nil, errors.New("product draws on no pool")
	}
	return out, nil
}

// ProductSold is units sold per product of an event (for sales ladders).
func (s *Service) ProductSold(ctx context.Context, org, eventID string) (map[string]int64, error) {
	out := map[string]int64{}
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select l.product_id::text, sum(l.qty)::bigint from sales.order_lines l
			join sales.orders o on o.organiser_id = l.organiser_id and o.id = l.order_id
			where o.event_id = $1 and o.state in ('paid', 'issued') group by l.product_id`, eventID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var n int64
			if err := rows.Scan(&id, &n); err != nil {
				return err
			}
			out[id] = n
		}
		return rows.Err()
	})
	return out, err
}

// stored is what an idempotency record keeps for order creation.
type stored struct {
	OrderID string          `json:"orderId,omitempty"`
	Problem *apperr.Problem `json:"problem,omitempty"`
}

// CreateOrder holds stock for a buyer and records the order (state held). A repeat with the same
// idempotency key returns the same order; a buyer with a live order for the sale gets that order
// back (existing = true) and nothing more is taken.
func (s *Service) CreateOrder(ctx context.Context, b Buyer, req CreateRequest, idemKey string) (o *Order, existing bool, err error) {
	if b.ID == "" || (b.Kind != "account" && b.Kind != "guest") {
		return nil, false, apperr.New(http.StatusUnauthorized, apperr.Forbidden, "A buyer is required.", false)
	}
	if _, err := uuid.Parse(req.EventID); err != nil {
		return nil, false, invalid("eventId must be a uuid")
	}
	live, err := s.Snaps.Load(ctx, req.EventID)
	if errors.Is(err, snapshot.ErrNotFound) {
		return nil, false, apperr.New(http.StatusNotFound, apperr.EventOffSale, "This event is not on sale.", false)
	}
	if err != nil {
		return nil, false, err
	}
	org := live.OrganiserID
	scope := "orders:" + b.ID
	if idemKey != "" {
		replay, err := idem.Begin(ctx, s.Pool, org, scope, idemKey, idem.Hash(req))
		switch {
		case errors.Is(err, idem.ErrMismatch):
			return nil, false, apperr.New(http.StatusUnprocessableEntity, apperr.IdempotencyMismatch, "This Idempotency-Key was used for another request.", false)
		case errors.Is(err, idem.ErrInProgress):
			return nil, false, apperr.New(http.StatusConflict, apperr.InProgress, "The same request is still running.", true)
		case errors.Is(err, idem.ErrBadKey):
			return nil, false, invalid(err.Error())
		case err != nil:
			return nil, false, err
		}
		if replay != nil {
			var st stored
			_ = json.Unmarshal(replay.Response, &st)
			if st.Problem != nil {
				return nil, false, st.Problem
			}
			o, err := s.GetIn(ctx, org, st.OrderID)
			return o, true, err
		}
	}
	o, existing, err = s.create(ctx, b, req, live)
	if idemKey != "" {
		var p *apperr.Problem
		switch {
		case err == nil:
			_ = idem.Finish(ctx, s.Pool, org, scope, idemKey, http.StatusCreated, stored{OrderID: o.ID})
		case errors.As(err, &p) && !p.Retryable && p.Code != apperr.HoldSoldOut:
			// Sold out is not kept under the key (review M3): stock may come back.
			_ = idem.Finish(ctx, s.Pool, org, scope, idemKey, p.Status, stored{Problem: p})
		default:
			idem.Abort(ctx, s.Pool, org, scope, idemKey)
		}
	}
	return o, existing, err
}

func (s *Service) liveOrderOf(ctx context.Context, org, eventID, buyer string) (string, error) {
	var id string
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select id::text from sales.orders where event_id = $1 and buyer_id = $2 and state = any($3) limit 1`, eventID, buyer, LiveStates).Scan(&id)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return id, err
}

func (s *Service) create(ctx context.Context, b Buyer, req CreateRequest, live *snapshot.Snapshot) (*Order, bool, error) {
	org := live.OrganiserID
	if id, err := s.liveOrderOf(ctx, org, req.EventID, b.ID); err != nil {
		return nil, false, err
	} else if id != "" {
		o, err := s.GetIn(ctx, org, id)
		return o, true, err
	}
	if req.Version < 1 || req.Version > live.Version {
		return nil, false, invalid("unknown snapshot version")
	}
	// Prices come only from the live snapshot (review D-a): a page of an earlier version is told
	// the prices changed, with the live version and its total, and never priced from it.
	priced := live
	if len(req.Items) == 0 || len(req.Items) > 50 {
		return nil, false, invalid("an order has 1 to 50 lines")
	}
	locale := req.Locale
	if locale != "en" {
		locale = "ro"
	}
	now := s.now()
	// Merge duplicate lines, check each against both the priced and the live version.
	type key struct{ product, instance string }
	merged := map[key]int{}
	var order []key
	for _, it := range req.Items {
		if _, err := uuid.Parse(it.ProductID); err != nil {
			return nil, false, invalid("productId must be a uuid")
		}
		if it.Qty < 1 || it.Qty > 100 {
			return nil, false, invalid("quantities are 1 to 100")
		}
		if it.Instance != "" && !instancePattern.MatchString(it.Instance) {
			return nil, false, invalid("instance must be YYYY-MM-DD or YYYY-MM-DDTHH:MM")
		}
		k := key{it.ProductID, it.Instance}
		if _, ok := merged[k]; !ok {
			order = append(order, k)
		}
		merged[k] += it.Qty
	}
	sold, err := s.ProductSold(ctx, org, req.EventID)
	if err != nil {
		return nil, false, err
	}
	perProduct := map[string]int{}
	tickets := 0
	inOrder := map[string]bool{}
	for _, k := range order {
		inOrder[k.product] = true
	}
	draws := map[string]int64{}
	var lines []Line
	var total, face, fanFee, net int64
	for i, k := range order {
		qty := merged[k]
		pp := priced.Product(k.product)
		lp := live.Product(k.product)
		// Hidden, unknown or from another snapshot: refused, nothing taken.
		if pp == nil || lp == nil || !pp.Visible || !lp.Visible || len(pp.PriceSteps) == 0 {
			return nil, false, invalid("a product is not on sale").With("productId", k.product)
		}
		if from, ok := parseTS(lp.SalesFrom); ok && now.Before(from) {
			return nil, false, invalid("sales have not opened for a product").With("productId", k.product)
		}
		if until, ok := parseTS(lp.SalesUntil); ok && !now.Before(until) {
			return nil, false, invalid("sales have ended for a product").With("productId", k.product)
		}
		perProduct[k.product] += qty
		if max := lp.MaxPerOrder; max > 0 && perProduct[k.product] > max {
			return nil, false, apperr.New(http.StatusUnprocessableEntity, apperr.HoldLimit, fmt.Sprintf("At most %d per order.", max), false).With("productId", k.product).With("max", max)
		}
		if lp.Type != "add_on" {
			tickets += qty
		}
		if len(lp.Requires) > 0 {
			ok := false
			for _, r := range lp.Requires {
				if inOrder[r] {
					ok = true
				}
			}
			if !ok {
				return nil, false, invalid("an add-on needs a ticket in the same order").With("productId", k.product)
			}
		}
		insts, err := drawsOf(live, lp, k.instance)
		if err != nil {
			return nil, false, invalid(err.Error()).With("productId", k.product)
		}
		for _, in := range insts {
			draws[in] += int64(qty)
		}
		idx := snapshot.ActiveStep(pp.PriceSteps, now, sold[k.product])
		st := pp.PriceSteps[idx]
		var inst *string
		if k.instance != "" {
			v := k.instance
			inst = &v
		}
		lines = append(lines, Line{LineNo: i + 1, ProductID: k.product, Name: pp.Name, Qty: qty, Instance: inst, PriceStepIndex: idx,
			UnitAllInMinor: st.AllInMinor, UnitFaceMinor: st.FaceMinor, UnitFanFeeMinor: st.FanFeeMinor, UnitOrganiserNetMinor: st.OrganiserNetMinor})
		total += st.AllInMinor * int64(qty)
		face += st.FaceMinor * int64(qty)
		fanFee += st.FanFeeMinor * int64(qty)
		net += st.OrganiserNetMinor * int64(qty)
	}
	// The per-account maximum counts tickets this buyer already bought for the event.
	if live.Rules.MaxPerAccount != nil && *live.Rules.MaxPerAccount > 0 {
		have, err := s.ticketsOf(ctx, org, req.EventID, b.ID, live)
		if err != nil {
			return nil, false, err
		}
		if have+tickets > *live.Rules.MaxPerAccount {
			return nil, false, apperr.New(http.StatusUnprocessableEntity, apperr.HoldLimit, fmt.Sprintf("At most %d tickets per person for this event.", *live.Rules.MaxPerAccount), false).
				With("max", *live.Rules.MaxPerAccount).With("have", have)
		}
	}
	if req.Version != live.Version {
		return nil, false, apperr.New(http.StatusUnprocessableEntity, apperr.PricesChanged, "Prices changed, please review.", true).
			With("version", live.Version).With("totalMinor", total).With("currency", live.Currency)
	}
	id := uuid.NewString()
	holdMinutes := HoldMinutes(live.Rules)
	// Step 5 (D-11): the sale's pass, cap per order and cap per person, before any stock is taken.
	var gate *GateResult
	if s.Gate != nil {
		maxAcct := 0
		if live.Rules.MaxPerAccount != nil {
			maxAcct = *live.Rules.MaxPerAccount
		}
		g, err := s.Gate.BeforeHold(ctx, GateHold{Buyer: b, Org: org, EventID: req.EventID, OrderID: id, Tickets: tickets, MaxPerAccount: maxAcct, HoldMinutes: holdMinutes})
		if err != nil {
			return nil, false, err
		}
		gate = g
		if g != nil && g.HoldMinutes >= MinHoldMinutes && g.HoldMinutes <= MaxHoldMinutes {
			holdMinutes = g.HoldMinutes
		}
	}
	deadline := now.Add(time.Duration(holdMinutes) * time.Minute).Truncate(time.Millisecond)
	if gate != nil && gate.Taken != nil {
		if err := gate.Taken(ctx, deadline.Add(AttemptLimit+HoldGrace)); err != nil {
			return nil, false, err
		}
	}
	undoGate := func() {
		if gate != nil && gate.Undo != nil {
			gate.Undo(context.WithoutCancel(ctx))
		}
	}
	caps := Capacities(live)
	if err := s.reserve(ctx, Hold{OrganiserID: org, EventID: req.EventID, ID: id, Draws: draws, Deadline: deadline}, caps); err != nil {
		undoGate()
		var short *ErrShort
		if errors.As(err, &short) {
			soldOuts.Inc()
			name := productDrawing(live, order2products(lines), short.Instance)
			return nil, false, apperr.New(http.StatusConflict, apperr.HoldSoldOut, "Not enough tickets left.", false).With("instanceId", short.Instance).With("productId", name)
		}
		return nil, false, err
	}
	o := &Order{ID: id, OrganiserID: org, EventID: req.EventID, SnapshotVersion: req.Version, BuyerKind: b.Kind, BuyerID: b.ID, Locale: locale, Currency: live.Currency,
		State: Held, TotalMinor: total, FaceMinor: face, FanFeeMinor: fanFee, OrganiserNetMinor: net, Draws: draws, HoldExpiresAt: deadline, HoldRef: id, PayPlan: "full",
		Answers: map[string]any{}, CreatedAt: now, Lines: lines}
	if gate != nil {
		if gate.SaleID != "" {
			o.SaleID = &gate.SaleID
		}
		if gate.PassQID != "" {
			o.PassQID = &gate.PassQID
		}
		if gate.CapIdentity != "" {
			o.CapIdentity = &gate.CapIdentity
			o.CapPerPerson = &gate.CapPerPerson
			o.CapTickets = &gate.CapTickets
		}
		o.OTPRequired = gate.OTPRequired
	}
	if b.Kind == "account" && b.Email != "" {
		e := strings.ToLower(strings.TrimSpace(b.Email))
		o.BuyerEmail = &e
	}
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		dj, _ := json.Marshal(draws)
		if _, err := tx.Exec(ctx, `insert into sales.orders (id, organiser_id, event_id, snapshot_version, buyer_kind, buyer_id, buyer_email, locale, currency, state,
			total_minor, face_minor, fan_fee_minor, organiser_net_minor, draws, hold_expires_at, hold_base_expires_at, hold_ref, created_at,
			sale_id, pass_qid, cap_identity, cap_per_person, cap_tickets, otp_required)
			values ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, 'held', $10, $11, $12, $13, $14, $15, $15, $1::text, $16, $17, $18, $19, $20, $21, $22)`,
			id, org, req.EventID, req.Version, b.Kind, b.ID, o.BuyerEmail, locale, live.Currency, total, face, fanFee, net, dj, deadline, now,
			o.SaleID, o.PassQID, o.CapIdentity, o.CapPerPerson, o.CapTickets, o.OTPRequired); err != nil {
			return err
		}
		for _, l := range lines {
			if _, err := tx.Exec(ctx, `insert into sales.order_lines (organiser_id, order_id, line_no, product_id, qty, instance_id, price_step_index,
				unit_all_in_minor, unit_face_minor, unit_fan_fee_minor, unit_organiser_net_minor) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
				org, id, l.LineNo, l.ProductID, l.Qty, l.Instance, l.PriceStepIndex, l.UnitAllInMinor, l.UnitFaceMinor, l.UnitFanFeeMinor, l.UnitOrganiserNetMinor); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `insert into sales.order_history (organiser_id, order_id, from_state, to_state, reason) values ($1, $2, null, 'held', 'hold')`, org, id); err != nil {
			return err
		}
		return emit(ctx, tx, o, "order.held", map[string]any{"holdExpiresAt": deadline})
	})
	if err != nil {
		_, _ = s.Inv.Release(context.WithoutCancel(ctx), org, req.EventID, id)
		undoGate()
		var pe *pgconn.PgError
		if errors.As(err, &pe) && pe.Code == "23505" && pe.ConstraintName == "orders_one_live_per_buyer" {
			// A second request raced the first: return the order that won.
			if other, err2 := s.liveOrderOf(ctx, org, req.EventID, b.ID); err2 == nil && other != "" {
				o, err := s.GetIn(ctx, org, other)
				return o, true, err
			}
		}
		return nil, false, err
	}
	o.History = []HistoryEntry{{To: Held, Reason: "hold", At: now}}
	holdsCreated.Inc()
	return o, false, nil
}

func order2products(lines []Line) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		out = append(out, l.ProductID)
	}
	return out
}

// productDrawing names the first ordered product that draws on an instance.
func productDrawing(s *snapshot.Snapshot, products []string, instance string) string {
	pool := instance
	if i := strings.IndexByte(instance, '@'); i >= 0 {
		pool = instance[:i]
	}
	for _, id := range products {
		if p := s.Product(id); p != nil {
			for _, pl := range p.PoolIDs {
				if pl == pool {
					return id
				}
			}
		}
	}
	return ""
}

// ticketsOf counts tickets (not add-ons) a buyer holds or bought for an event.
func (s *Service) ticketsOf(ctx context.Context, org, eventID, buyer string, live *snapshot.Snapshot) (int, error) {
	addOns := []string{}
	for _, p := range live.Products {
		if p.Type == "add_on" {
			addOns = append(addOns, p.ProductID)
		}
	}
	var n int
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `select coalesce(sum(l.qty), 0) from sales.order_lines l join sales.orders o on o.organiser_id = l.organiser_id and o.id = l.order_id
			where o.event_id = $1 and o.buyer_id = $2 and o.state in ('held', 'paying', 'payment_failed', 'paid', 'issued') and not (l.product_id::text = any($3))`,
			eventID, buyer, addOns).Scan(&n)
	})
	return n, err
}

// reserve takes a hold, loading the event's counters from Postgres first if Redis lost them.
func (s *Service) reserve(ctx context.Context, h Hold, caps map[string]int64) error {
	err := s.Inv.Reserve(ctx, h, caps)
	if errors.Is(err, ErrNotReady) {
		if err := s.Rebuild(ctx, h.OrganiserID, h.EventID); err != nil {
			return err
		}
		err = s.Inv.Reserve(ctx, h, caps)
	}
	return err
}

// Rebuild loads an event's counters and live holds into the inventory from the orders. One core
// at a time per event (review D-j): the others wait for its result.
func (s *Service) Rebuild(ctx context.Context, org, eventID string) error {
	_, err, _ := s.rebuilds.Do(org+"/"+eventID, func() (any, error) {
		for i := 0; i < 50; i++ {
			got, unlock, err := s.Inv.Lock(ctx, org, eventID, 10*time.Second)
			if err != nil {
				return nil, err
			}
			if got {
				defer unlock()
				return nil, s.rebuild(ctx, org, eventID)
			}
			time.Sleep(100 * time.Millisecond)
		}
		return nil, errors.New("checkout: inventory rebuild is taking too long")
	})
	return err
}

func (s *Service) rebuild(ctx context.Context, org, eventID string) error {
	used := map[string]int64{}
	sold := map[string]int64{}
	var holds []Hold
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select hold_ref, state, draws, hold_expires_at from sales.orders
			where event_id = $1 and state in ('held', 'paying', 'payment_failed', 'paid', 'issued')`, eventID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ref, state string
			var dj []byte
			var dl time.Time
			if err := rows.Scan(&ref, &state, &dj, &dl); err != nil {
				return err
			}
			var d map[string]int64
			_ = json.Unmarshal(dj, &d)
			for k, v := range d {
				used[k] += v
				if state == Paid || state == Issued {
					sold[k] += v
				}
			}
			if state != Paid && state != Issued {
				holds = append(holds, Hold{OrganiserID: org, EventID: eventID, ID: ref, Draws: d, Deadline: dl})
			}
		}
		return rows.Err()
	})
	if err != nil {
		return err
	}
	return s.Inv.Rebuild(ctx, org, eventID, used, sold, holds)
}

func parseTS(v *string) (time.Time, bool) {
	if v == nil || *v == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, *v)
	return t, err == nil
}

// instancesOf lists every pool instance of a snapshot, sorted.
func instancesOf(s *snapshot.Snapshot) []string {
	var out []string
	for _, p := range s.Pools {
		for _, in := range p.Instances {
			out = append(out, in.InstanceID)
		}
	}
	sort.Strings(out)
	return out
}
