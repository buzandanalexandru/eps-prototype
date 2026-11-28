package tickets

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Item is a ticket as the holder's trip wallet shows it: the ticket with what its snapshot says
// about it (design D9).
type Item struct {
	*Ticket
	ProductType string        `json:"productType"`
	ProductName snapshot.L10n `json:"productName"`
	Zone        *string       `json:"zone,omitempty"`
	// StartsAt is the item's own time (a slot, a day ticket's day, a picked day); nil: the event's.
	StartsAt      *time.Time `json:"startsAt,omitempty"`
	Day           *string    `json:"day,omitempty"`
	NeedsName     bool       `json:"needsName"`
	Named         bool       `json:"named"`
	PersonaliseBy *time.Time `json:"personaliseBy,omitempty"`
	NameLocked    bool       `json:"nameLocked"`
	CheckinBy     *time.Time `json:"checkinBy,omitempty"`
	LateFeeMinor  *int64     `json:"lateCheckinFeeMinor,omitempty"`
	PickDays      []string   `json:"pickDays,omitempty"`
	Overdue       *Overdue   `json:"overdue,omitempty"`
}

// Overdue is the instalment a suspended ticket waits for.
type Overdue struct {
	AmountMinor int64   `json:"amountMinor"`
	GraceUntil  *string `json:"graceUntil,omitempty"`
}

// Event groups a holder's items of one event.
type Event struct {
	EventID     string          `json:"eventId"`
	OrganiserID string          `json:"organiserId"`
	BrandID     string          `json:"brandId"`
	Name        snapshot.L10n   `json:"name"`
	StartsAt    string          `json:"startsAt"`
	EndsAt      string          `json:"endsAt"`
	Timezone    string          `json:"timezone"`
	Venue       *snapshot.Venue `json:"venue,omitempty"`
	Currency    string          `json:"currency"`
	IsSample    bool            `json:"isSample"`
	Items       []*Item         `json:"items"`
	Pending     []Pending       `json:"pending"`
}

// Pending is a paid order whose tickets are being issued.
type Pending struct {
	OrderID string        `json:"orderId"`
	Lines   []PendingLine `json:"lines"`
}

// PendingLine is one line of a pending order.
type PendingLine struct {
	ProductID   string        `json:"productId"`
	ProductName snapshot.L10n `json:"productName"`
	Qty         int           `json:"qty"`
}

// Trip is everything a holder holds, by event.
type Trip struct {
	ServerTime time.Time `json:"serverTime"`
	Events     []*Event  `json:"events"`
}

func parseTime(v *string) *time.Time {
	if v == nil || *v == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *v)
	if err != nil {
		return nil
	}
	return &t
}

// itemTime is an item's own time from its pool instance (<pool>@<date>[T<hh:mm>]) or picked day,
// in the event's time zone: a slot's start, else the day's opening time.
func itemTime(snap *snapshot.Snapshot, instance *string, picked *string) (*time.Time, *string) {
	day, clock := "", ""
	if picked != nil {
		day = *picked
	} else if instance != nil {
		if _, rest, ok := strings.Cut(*instance, "@"); ok {
			day, clock, _ = strings.Cut(rest, "T")
		}
	}
	if day == "" {
		return nil, nil
	}
	if clock == "" {
		clock = "00:00"
		for _, d := range snap.Event.Days {
			if d.Date == day && d.Opens != "" {
				clock = d.Opens
			}
		}
	}
	loc, err := time.LoadLocation(snap.Timezone)
	if err != nil || snap.Timezone == "" {
		loc = time.UTC
	}
	t, err := time.ParseInLocation("2006-01-02 15:04", day+" "+clock, loc)
	if err != nil {
		return nil, &day
	}
	t = t.UTC()
	return &t, &day
}

// itemOf joins a ticket with its snapshot product.
func (s *Service) itemOf(ctx context.Context, tx pgx.Tx, t *Ticket, snap *snapshot.Snapshot, plans map[string]*checkout.Plan) *Item {
	it := &Item{Ticket: t}
	if snap == nil {
		return it
	}
	p := snap.Product(t.ProductID)
	if p == nil {
		return it
	}
	it.ProductType, it.ProductName, it.Zone, it.Named = p.Type, p.Name, p.Zone, p.Named
	it.StartsAt, it.Day = itemTime(snap, t.InstanceID, t.PickedDay)
	now := s.now()
	if p.Named {
		it.PersonaliseBy = nameDeadline(snap, p)
		it.NameLocked = it.PersonaliseBy != nil && !now.Before(*it.PersonaliseBy)
		it.NeedsName = t.HolderName == nil
	}
	it.CheckinBy = parseTime(p.CheckinBy)
	if it.CheckinBy != nil && p.LateCheckin != nil && p.LateCheckin.AllInMinor > 0 {
		fee := p.LateCheckin.AllInMinor
		it.LateFeeMinor = &fee
	}
	if p.Type == "flex_day" && t.PickedDay == nil && t.State == Valid {
		it.PickDays = configDays(p)
	}
	if t.State == Suspended && tx != nil {
		pl, ok := plans[t.OrderID]
		if !ok {
			pl, _ = checkout.PlanTx(ctx, tx, t.OrderID)
			plans[t.OrderID] = pl
		}
		if pl != nil {
			for _, part := range pl.Parts {
				if part.State == "overdue" {
					it.Overdue = &Overdue{AmountMinor: part.AmountMinor, GraceUntil: part.GraceUntil}
					break
				}
			}
		}
	}
	return it
}

// nameDeadline is a named product's personalisation deadline: its own, else the event's start.
func nameDeadline(snap *snapshot.Snapshot, p *snapshot.Product) *time.Time {
	if d := parseTime(p.PersonaliseBy); d != nil {
		return d
	}
	if t, ok := eventStart(snap); ok {
		return &t
	}
	return nil
}

func configDays(p *snapshot.Product) []string {
	var c struct {
		Days []string `json:"days"`
	}
	if len(p.Config) == 0 || jsonUnmarshal(p.Config, &c) != nil {
		return nil
	}
	return c.Days
}

func eventOf(snap *snapshot.Snapshot, org, eventID string) *Event {
	e := &Event{EventID: eventID, OrganiserID: org, Items: []*Item{}, Pending: []Pending{}}
	if snap != nil {
		e.BrandID, e.Name, e.StartsAt, e.EndsAt, e.Timezone, e.Venue, e.Currency, e.IsSample =
			snap.BrandID, snap.Event.Name, snap.Event.StartsAt, snap.Event.EndsAt, snap.Timezone, snap.Event.Venue, snap.Currency, snap.IsSample
	}
	return e
}

// TripOf is the holder's trip: every ticket they hold now, across organisers, and their paid orders
// still being issued, grouped by event and ordered by event start (design D9).
func (s *Service) TripOf(ctx context.Context, h Holder) (*Trip, error) {
	orgs := map[string]bool{}
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select organiser_id::text from sales.holder_organisers($1)
			union select distinct organiser_id::text from sales.orders_of_buyer($1, 200)`, h.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var o string
			if err := rows.Scan(&o); err != nil {
				return err
			}
			orgs[o] = true
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	trip := &Trip{ServerTime: s.now().UTC(), Events: []*Event{}}
	byEvent := map[string]*Event{}
	for org := range orgs {
		if err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
			list, err := queryTickets(ctx, tx, "holder_id = $1", h.ID)
			if err != nil {
				return err
			}
			pending, err := pendingOrdersTx(ctx, tx, "buyer_id = $1", h.ID)
			if err != nil {
				return err
			}
			plans := map[string]*checkout.Plan{}
			for _, t := range list {
				snap, _ := s.Snaps.LoadVersion(ctx, t.EventID, t.SnapshotVersion)
				e := byEvent[t.EventID]
				if e == nil {
					e = eventOf(snap, org, t.EventID)
					byEvent[t.EventID] = e
				}
				e.Items = append(e.Items, s.itemOf(ctx, tx, t, snap, plans))
			}
			for _, p := range pending {
				e := byEvent[p.eventID]
				if e == nil {
					snap, _ := s.Snaps.LoadVersion(ctx, p.eventID, p.version)
					e = eventOf(snap, org, p.eventID)
					byEvent[p.eventID] = e
				}
				snap, _ := s.Snaps.LoadVersion(ctx, p.eventID, p.version)
				e.Pending = append(e.Pending, named(snap, p.Pending))
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}
	for _, e := range byEvent {
		trip.Events = append(trip.Events, e)
	}
	slices.SortFunc(trip.Events, func(a, b *Event) int {
		if c := strings.Compare(a.StartsAt, b.StartsAt); c != 0 {
			return c
		}
		return strings.Compare(a.EventID, b.EventID)
	})
	return trip, nil
}

type pendingOrder struct {
	Pending
	eventID string
	version int
}

// pendingOrdersTx are paid (not yet issued) orders matching where, with their lines' names.
func pendingOrdersTx(ctx context.Context, tx pgx.Tx, where string, args ...any) ([]pendingOrder, error) {
	rows, err := tx.Query(ctx, `select o.id::text, o.event_id::text, o.snapshot_version, l.product_id::text, l.qty from sales.orders o
		join sales.order_lines l on l.organiser_id = o.organiser_id and l.order_id = o.id
		where o.state = 'paid' and not o.rehearsal and o.`+where+` order by o.created_at, l.line_no`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []pendingOrder
	idx := map[string]int{}
	type line struct {
		product string
		qty     int
	}
	lines := map[string][]line{}
	for rows.Next() {
		var id, ev, product string
		var v, qty int
		if err := rows.Scan(&id, &ev, &v, &product, &qty); err != nil {
			return nil, err
		}
		if _, ok := idx[id]; !ok {
			idx[id] = len(out)
			out = append(out, pendingOrder{Pending: Pending{OrderID: id, Lines: []PendingLine{}}, eventID: ev, version: v})
		}
		lines[id] = append(lines[id], line{product, qty})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		for _, l := range lines[out[i].OrderID] {
			out[i].Lines = append(out[i].Lines, PendingLine{ProductID: l.product, Qty: l.qty})
		}
	}
	return out, nil
}

// named fills a pending order's product names from its snapshot.
func named(snap *snapshot.Snapshot, p Pending) Pending {
	for i := range p.Lines {
		if snap != nil {
			if pr := snap.Product(p.Lines[i].ProductID); pr != nil {
				p.Lines[i].ProductName = pr.Name
			}
		}
	}
	return p
}

// TicketFor is one ticket the holder holds, with its event (the live ticket page).
func (s *Service) TicketFor(ctx context.Context, h Holder, ticketID string) (*Event, error) {
	org, err := s.organiserOf(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	var e *Event
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		t, err := loadForHolder(ctx, tx, ticketID, h, false)
		if err != nil {
			return err
		}
		snap, _ := s.Snaps.LoadVersion(ctx, t.EventID, t.SnapshotVersion)
		e = eventOf(snap, org, t.EventID)
		e.Items = append(e.Items, s.itemOf(ctx, tx, t, snap, map[string]*checkout.Plan{}))
		return nil
	})
	return e, err
}

// mutate loads a ticket the holder holds under its row lock with its snapshot and runs f.
func (s *Service) mutate(ctx context.Context, h Holder, ticketID string, f func(tx pgx.Tx, t *Ticket, snap *snapshot.Snapshot, p *snapshot.Product) error) (*Event, error) {
	org, err := s.organiserOf(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		t, err := loadForHolder(ctx, tx, ticketID, h, true)
		if err != nil {
			return err
		}
		if t.State == Cancelled {
			return apperr.New(http.StatusConflict, CodeCancelled, "This ticket was cancelled.", false)
		}
		snap, err := s.Snaps.LoadVersion(ctx, t.EventID, t.SnapshotVersion)
		if err != nil {
			return err
		}
		p := snap.Product(t.ProductID)
		if p == nil {
			return notFound()
		}
		return f(tx, t, snap, p)
	})
	if err != nil {
		return nil, err
	}
	return s.TicketFor(ctx, h, ticketID)
}

// Personalise names a ticket (and is the only way to change the name) until the product's
// personalisation deadline, the event's start when it has none. After it, the name is locked. The
// door learns the new name through a manifest row of its feed.
func (s *Service) Personalise(ctx context.Context, h Holder, ticketID, name string) (*Event, error) {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || utf8.RuneCountInString(name) > 120 || strings.ContainsAny(name, "<>\x00") {
		return nil, apperr.New(http.StatusUnprocessableEntity, "invalid_request", "Enter the name of the person who will use this ticket (at most 120 characters).", false)
	}
	return s.mutate(ctx, h, ticketID, func(tx pgx.Tx, t *Ticket, snap *snapshot.Snapshot, p *snapshot.Product) error {
		if d := nameDeadline(snap, p); d != nil && !s.now().Before(*d) {
			return apperr.New(http.StatusConflict, CodeNameLocked, "The name is locked: the deadline to name this ticket has passed.", false).With("deadline", d.UTC().Format(time.RFC3339))
		}
		if _, err := tx.Exec(ctx, "update sales.tickets set holder_name = $2, updated_at = now() where id = $1", t.ID, name); err != nil {
			return err
		}
		g := t.Generation
		_, err := FeedTx(ctx, tx, t.OrganiserID, t.EventID, "manifest", t.ID, &g, map[string]any{"name": name})
		return err
	})
}

// CheckIn checks a ticket in online: free until the product's check-in deadline; after it and
// until the event ends, only by paying the late check-in fee (all-in) from the holder's EPS Wallet
// in the same transaction (O6). A guest must sign in to pay a late check-in.
func (s *Service) CheckIn(ctx context.Context, h Holder, ticketID string, payLate bool) (*Event, error) {
	return s.mutate(ctx, h, ticketID, func(tx pgx.Tx, t *Ticket, snap *snapshot.Snapshot, p *snapshot.Product) error {
		by := parseTime(p.CheckinBy)
		if by == nil {
			return apperr.New(http.StatusConflict, CodeNoCheckIn, "This ticket has no online check-in.", false)
		}
		if t.CheckedInAt != nil {
			return apperr.New(http.StatusConflict, CodeCheckedIn, "Already checked in.", false)
		}
		if t.State == Suspended {
			return apperr.New(http.StatusConflict, CodeSuspended, "Payment overdue: pay the instalment first.", false)
		}
		now := s.now()
		if end, ok := eventEnd(snap); ok && now.After(end) {
			return apperr.New(http.StatusConflict, CodeEnded, "The event has ended.", false)
		}
		var entry *string
		if !now.Before(*by) && p.LateCheckin != nil && p.LateCheckin.AllInMinor > 0 {
			if !payLate {
				return apperr.New(http.StatusPaymentRequired, "EPS-TICKET-LATE-FEE", "The check-in deadline has passed: a late check-in fee applies.", false).
					With("feeMinor", p.LateCheckin.AllInMinor).With("currency", snap.Currency)
			}
			if h.Kind != "account" || s.Pay == nil {
				return apperr.New(http.StatusForbidden, CodeSignIn, "Sign in to pay the late check-in fee from your EPS Wallet.", false)
			}
			id, err := s.Pay(ctx, tx, t.OrganiserID, h.ID, snap.Currency, p.LateCheckin.AllInMinor, "checkin:"+t.ID, "late check-in")
			if err != nil {
				return err
			}
			entry = &id
		}
		_, err := tx.Exec(ctx, "update sales.tickets set checked_in_at = $2, checkin_entry_id = $3::uuid, updated_at = now() where id = $1", t.ID, now, entry)
		return err
	})
}

// PickDay turns a flexible day ticket into a ticket for one of its days, once: a reissue (the
// ticket's next generation, its old codes revoked).
func (s *Service) PickDay(ctx context.Context, h Holder, ticketID, day string) (*Event, error) {
	return s.mutate(ctx, h, ticketID, func(tx pgx.Tx, t *Ticket, snap *snapshot.Snapshot, p *snapshot.Product) error {
		if p.Type != "flex_day" {
			return apperr.New(http.StatusConflict, CodeBadDay, "This ticket is not a flexible day ticket.", false)
		}
		if t.PickedDay != nil {
			return apperr.New(http.StatusConflict, CodeDayPicked, "The day is already picked.", false)
		}
		if t.State != Valid {
			return apperr.New(http.StatusConflict, CodeSuspended, "This ticket cannot change now.", false)
		}
		if !slices.Contains(configDays(p), day) {
			return apperr.New(http.StatusUnprocessableEntity, CodeBadDay, "Pick one of the ticket's days.", false)
		}
		loc, err := time.LoadLocation(snap.Timezone)
		if err != nil {
			loc = time.UTC
		}
		if d, err := time.ParseInLocation("2006-01-02", day, loc); err != nil || d.AddDate(0, 0, 1).Before(s.now()) {
			return apperr.New(http.StatusUnprocessableEntity, CodeBadDay, "That day has passed.", false)
		}
		_, err = ReissueTx(ctx, tx, t.ID, t.Generation, Change{Reason: "day_picked", PickedDay: day})
		if errors.Is(err, ErrConflict) {
			return apperr.New(http.StatusConflict, CodeConflict, "The ticket changed meanwhile; try again.", true)
		}
		if err != nil {
			return err
		}
		g := t.Generation + 1
		_, err = FeedTx(ctx, tx, t.OrganiserID, t.EventID, "manifest", t.ID, &g, map[string]any{"day": day})
		return err
	})
}

// ---------------------------------------------------------------------------------------------
// The guest's signed tickets link (design D10)

// LinkFor is a guest order's tickets link token (until two days after its event), or "" for an
// order that is not a guest's issued order.
func (s *Service) LinkFor(ctx context.Context, o *checkout.Order) string {
	if o == nil || s.Links == nil || o.BuyerKind != "guest" || o.State != checkout.Issued {
		return ""
	}
	until := s.now().Add(30 * 24 * time.Hour)
	if snap, err := s.Snaps.LoadVersion(ctx, o.EventID, o.SnapshotVersion); err == nil {
		if end, ok := eventEnd(snap); ok && end.After(s.now()) {
			until = end.Add(48 * time.Hour)
		}
	}
	return s.Links.TicketsLinkFor(o.ID, until)
}

// LinkHolder checks a tickets link and returns its order's organiser and the guest it names. A
// changed, expired or unknown link is "not found".
func (s *Service) LinkHolder(ctx context.Context, token string) (string, *checkout.Order, Holder, error) {
	bad := apperr.New(http.StatusNotFound, "EPS-LINK-NOT-VALID", "Link not valid.", false)
	if s.Links == nil {
		return "", nil, Holder{}, bad
	}
	id := s.Links.OrderOf(token)
	if id == "" {
		return "", nil, Holder{}, bad
	}
	if _, err := s.Links.Check(token, id, checkout.LinkTickets); err != nil {
		return "", nil, Holder{}, bad
	}
	var org *string
	if err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select sales.order_organiser($1)::text", id).Scan(&org)
	}); err != nil {
		return "", nil, Holder{}, err
	}
	if org == nil {
		return "", nil, Holder{}, bad
	}
	var o *checkout.Order
	err := db.WithTx(ctx, s.Pool, db.RoleCore, *org, func(tx pgx.Tx) error {
		var err error
		o, err = checkout.LoadTx(ctx, tx, id, false)
		return err
	})
	if err != nil {
		return "", nil, Holder{}, bad
	}
	if o.BuyerKind != "guest" {
		// Claimed into an account: the account's wallet has the tickets now.
		return "", nil, Holder{}, apperr.New(http.StatusGone, CodeMoved, "These tickets were added to an account: sign in to see them.", false)
	}
	return *org, o, Holder{ID: o.BuyerID, Kind: "guest"}, nil
}

// LinkTrip is the order's tickets that its guest still holds (the link's page).
func (s *Service) LinkTrip(ctx context.Context, token string) (*Event, error) {
	org, o, h, err := s.LinkHolder(ctx, token)
	if err != nil {
		return nil, err
	}
	snap, _ := s.Snaps.LoadVersion(ctx, o.EventID, o.SnapshotVersion)
	e := eventOf(snap, org, o.EventID)
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		list, err := queryTickets(ctx, tx, "order_id = $1 and holder_id = $2", o.ID, h.ID)
		if err != nil {
			return err
		}
		plans := map[string]*checkout.Plan{}
		for _, t := range list {
			e.Items = append(e.Items, s.itemOf(ctx, tx, t, snap, plans))
		}
		if o.State == checkout.Paid {
			p, err := pendingOrdersTx(ctx, tx, "id = $1", o.ID)
			if err != nil {
				return err
			}
			for _, x := range p {
				e.Pending = append(e.Pending, named(snap, x.Pending))
			}
		}
		return nil
	})
	return e, err
}

// inLinkOrder checks that a ticket belongs to the link's order.
func (s *Service) inLinkOrder(ctx context.Context, org, orderID, ticketID string) error {
	var n int
	if err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, "select count(*) from sales.tickets where id = $1 and order_id = $2", ticketID, orderID).Scan(&n)
	}); err != nil {
		return err
	}
	if n == 0 {
		return notFound()
	}
	return nil
}

// LinkCodes are a ticket's codes through its order's tickets link.
func (s *Service) LinkCodes(ctx context.Context, token, ticketID string) (*Codes, error) {
	org, o, h, err := s.LinkHolder(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := s.inLinkOrder(ctx, org, o.ID, ticketID); err != nil {
		return nil, err
	}
	return s.CodesFor(ctx, h, ticketID)
}

// LinkPersonalise names a ticket through its order's tickets link.
func (s *Service) LinkPersonalise(ctx context.Context, token, ticketID, name string) (*Event, error) {
	org, o, h, err := s.LinkHolder(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := s.inLinkOrder(ctx, org, o.ID, ticketID); err != nil {
		return nil, err
	}
	return s.Personalise(ctx, h, ticketID, name)
}
