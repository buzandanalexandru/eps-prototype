// Package sales holds the sale moments of an event and their versioned settings (E1, design D-1):
// opening time, pre-queue window, queue on or off, the admission dial, the shopping limit, holds,
// caps, bot defence and OTP. Settings live in the sales schema (owned by Go) and are edited from
// Studio through the contract; every save is a new version. The current settings of an event's
// sales are kept in Redis (signed), so the gateway's admitter and the core's checkout gate follow a
// live change within 2 seconds without asking Postgres on every request.
package sales

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// Settings are one version of a sale's settings.
type Settings struct {
	OpensAt         time.Time `json:"opensAt"`
	PrequeueMinutes int       `json:"prequeueMinutes"`
	QueueOn         bool      `json:"queueOn"`
	RatePerMinute   int       `json:"ratePerMinute"`
	ActiveLimit     int       `json:"activeLimit"`
	Paused          bool      `json:"paused"`
	HoldMinutes     int       `json:"holdMinutes"`
	CapPerOrder     int       `json:"capPerOrder"`
	CapPerPerson    int       `json:"capPerPerson"`
	BotDefence      bool      `json:"botDefence"`
	OTPOn           bool      `json:"otpOn"`
	OTPThreshold    int       `json:"otpThreshold"`
}

// Defaults (D-1): pre-queue 30 minutes before, 2,000 fans a minute, 1,500 shopping at once, holds
// of 8 minutes, 4 per order and 6 per person, bot defence on, OTP off with a threshold of 4.
func Defaults(opensAt time.Time) Settings {
	return Settings{OpensAt: opensAt, PrequeueMinutes: 30, QueueOn: true, RatePerMinute: 2000, ActiveLimit: 1500,
		HoldMinutes: 8, CapPerOrder: 4, CapPerPerson: 6, BotDefence: true, OTPOn: false, OTPThreshold: 4}
}

// Ranges of every number (the spec's allowed ranges).
var ranges = []struct {
	field    string
	min, max int
	get      func(Settings) int
}{
	{"prequeueMinutes", 5, 120, func(s Settings) int { return s.PrequeueMinutes }},
	{"ratePerMinute", 10, 60000, func(s Settings) int { return s.RatePerMinute }},
	{"activeLimit", 1, 100000, func(s Settings) int { return s.ActiveLimit }},
	{"holdMinutes", 5, 15, func(s Settings) int { return s.HoldMinutes }},
	{"capPerOrder", 1, 20, func(s Settings) int { return s.CapPerOrder }},
	{"capPerPerson", 1, 50, func(s Settings) int { return s.CapPerPerson }},
	{"otpThreshold", 1, 20, func(s Settings) int { return s.OTPThreshold }},
}

// Validate refuses a value outside its range, naming the field and the range.
func (s Settings) Validate() *apperr.Problem {
	if s.OpensAt.IsZero() {
		return apperr.New(http.StatusUnprocessableEntity, apperr.SaleInvalid, "The opening time is required.", false).With("field", "opensAt")
	}
	for _, r := range ranges {
		if v := r.get(s); v < r.min || v > r.max {
			return apperr.New(http.StatusUnprocessableEntity, apperr.SaleInvalid, fmt.Sprintf("%s must be between %d and %d.", r.field, r.min, r.max), false).
				With("field", r.field).With("min", r.min).With("max", r.max)
		}
	}
	if s.CapPerOrder > s.CapPerPerson {
		return apperr.New(http.StatusUnprocessableEntity, apperr.SaleInvalid, "The cap per order cannot be above the cap per person.", false).With("field", "capPerOrder")
	}
	return nil
}

// PrequeueOpensAt is when fans may start joining the waiting room.
func (s Settings) PrequeueOpensAt() time.Time {
	return s.OpensAt.Add(-time.Duration(s.PrequeueMinutes) * time.Minute)
}

// Phase is where a sale stands at a time.
type Phase string

// Phases.
const (
	Before   Phase = "before"   // the pre-queue has not opened
	Prequeue Phase = "prequeue" // fans join the pre-queue; the order is drawn at opening
	Open     Phase = "open"     // the sale has opened
)

// PhaseAt is the sale's phase at t.
func (s Settings) PhaseAt(t time.Time) Phase {
	switch {
	case t.Before(s.PrequeueOpensAt()):
		return Before
	case t.Before(s.OpensAt):
		return Prequeue
	}
	return Open
}

// structural fields cannot change once the pre-queue opened (D-1).
func structuralChanged(a, b Settings) string {
	switch {
	case !a.OpensAt.Equal(b.OpensAt):
		return "opensAt"
	case a.PrequeueMinutes != b.PrequeueMinutes:
		return "prequeueMinutes"
	case a.QueueOn != b.QueueOn:
		return "queueOn"
	}
	return ""
}

// Sale is a sale moment with its current settings.
type Sale struct {
	ID          string    `json:"id"`
	OrganiserID string    `json:"organiserId"`
	EventID     string    `json:"eventId"`
	Name        string    `json:"name"`
	Version     int       `json:"version"`
	Settings    Settings  `json:"settings"`
	UpdatedAt   time.Time `json:"updatedAt"`
	UpdatedBy   string    `json:"updatedBy"`
}

// Version is one saved version of a sale's settings.
type Version struct {
	Version  int       `json:"version"`
	Settings Settings  `json:"settings"`
	At       time.Time `json:"at"`
	By       string    `json:"by"`
}

// Service reads and writes sales.
type Service struct {
	Pool  db.Beginner
	Redis *redis.Client
	// MACKey signs the copies kept in Redis (SNAPSHOT_CACHE_KEY).
	MACKey []byte
	// Now is the clock (tests); nil means time.Now.
	Now func() time.Time
	// Published says which organiser sells an event (its live snapshot); sales are created only
	// for a published event of the organiser.
	Published func(ctx context.Context, event string) (organiserID string, err error)

	mu    sync.Mutex
	local map[string]cached
}

type cached struct {
	at    time.Time
	sales []Sale
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func notFound() *apperr.Problem {
	return apperr.New(http.StatusNotFound, apperr.SaleNotFound, "Sale not found.", false)
}

const cols = `s.id::text, s.organiser_id::text, s.event_id::text, s.name, v.version, v.opens_at, v.prequeue_minutes, v.queue_on, v.rate_per_minute,
	v.active_limit, v.paused, v.hold_minutes, v.cap_per_order, v.cap_per_person, v.bot_defence, v.otp_on, v.otp_threshold, v.created_at, v.created_by`

func scanSale(row pgx.Row) (Sale, error) {
	var x Sale
	st := &x.Settings
	err := row.Scan(&x.ID, &x.OrganiserID, &x.EventID, &x.Name, &x.Version, &st.OpensAt, &st.PrequeueMinutes, &st.QueueOn, &st.RatePerMinute,
		&st.ActiveLimit, &st.Paused, &st.HoldMinutes, &st.CapPerOrder, &st.CapPerPerson, &st.BotDefence, &st.OTPOn, &st.OTPThreshold, &x.UpdatedAt, &x.UpdatedBy)
	st.OpensAt = st.OpensAt.UTC()
	x.UpdatedAt = x.UpdatedAt.UTC()
	return x, err
}

func insertVersion(ctx context.Context, tx pgx.Tx, org, saleID string, version int, st Settings, by string) error {
	_, err := tx.Exec(ctx, `insert into sales.sale_settings (organiser_id, sale_id, version, opens_at, prequeue_minutes, queue_on, rate_per_minute,
		active_limit, paused, hold_minutes, cap_per_order, cap_per_person, bot_defence, otp_on, otp_threshold, created_by)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)`,
		org, saleID, version, st.OpensAt, st.PrequeueMinutes, st.QueueOn, st.RatePerMinute, st.ActiveLimit, st.Paused, st.HoldMinutes,
		st.CapPerOrder, st.CapPerPerson, st.BotDefence, st.OTPOn, st.OTPThreshold, by)
	return err
}

// Create adds a sale moment to an event of the organiser (version 1 of its settings).
func (s *Service) Create(ctx context.Context, org, event, name, by string, st Settings) (*Sale, error) {
	if _, err := uuid.Parse(event); err != nil {
		return nil, apperr.New(http.StatusBadRequest, "invalid_request", "eventId must be a uuid.", false)
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return nil, apperr.New(http.StatusUnprocessableEntity, apperr.SaleInvalid, "A name of 1 to 80 characters is required.", false).With("field", "name")
	}
	if p := st.Validate(); p != nil {
		return nil, p
	}
	st.OpensAt = st.OpensAt.UTC().Truncate(time.Second)
	if s.Published != nil {
		owner, err := s.Published(ctx, event)
		if err != nil {
			return nil, err
		}
		if owner != org {
			return nil, apperr.New(http.StatusNotFound, apperr.SaleNotFound, "Publish the event before adding sale moments.", false)
		}
	}
	id := uuid.NewString()
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `insert into sales.sales (id, organiser_id, event_id, name, created_by) values ($1, $2, $3, $4, $5)`, id, org, event, name, by); err != nil {
			return err
		}
		if err := insertVersion(ctx, tx, org, id, 1, st, by); err != nil {
			return err
		}
		return writeEvent(ctx, tx, org, event, id, "sale.created", 1)
	})
	if err != nil {
		return nil, err
	}
	s.Refresh(ctx, org, event)
	out, _, err := s.Get(ctx, org, id)
	return out, err
}

func writeEvent(ctx context.Context, tx pgx.Tx, org, event, sale, typ string, version int) error {
	_, err := outbox.Write(ctx, tx, outbox.Event{Topic: "sales", Type: typ, OrganiserID: org, EventID: event, Payload: map[string]any{"saleId": sale, "version": version}})
	return err
}

// List returns the event's sales with their current settings, by opening time.
func (s *Service) List(ctx context.Context, org, event string) ([]Sale, error) {
	var out []Sale
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select `+cols+` from sales.sales s join sales.sale_settings v on v.organiser_id = s.organiser_id and v.sale_id = s.id and v.version = s.settings_version
			where s.event_id = $1 order by v.opens_at, s.created_at`, event)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			x, err := scanSale(rows)
			if err != nil {
				return err
			}
			out = append(out, x)
		}
		return rows.Err()
	})
	return out, err
}

// Get returns a sale of the organiser with its history (newest first).
func (s *Service) Get(ctx context.Context, org, saleID string) (*Sale, []Version, error) {
	if _, err := uuid.Parse(saleID); err != nil {
		return nil, nil, notFound()
	}
	var out *Sale
	var hist []Version
	err := db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		x, err := scanSale(tx.QueryRow(ctx, `select `+cols+` from sales.sales s join sales.sale_settings v on v.organiser_id = s.organiser_id and v.sale_id = s.id and v.version = s.settings_version where s.id = $1`, saleID))
		if errors.Is(err, pgx.ErrNoRows) {
			return notFound()
		}
		if err != nil {
			return err
		}
		out = &x
		rows, err := tx.Query(ctx, `select version, opens_at, prequeue_minutes, queue_on, rate_per_minute, active_limit, paused, hold_minutes, cap_per_order,
			cap_per_person, bot_defence, otp_on, otp_threshold, created_at, created_by from sales.sale_settings where sale_id = $1 order by version desc limit 200`, saleID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var v Version
			st := &v.Settings
			if err := rows.Scan(&v.Version, &st.OpensAt, &st.PrequeueMinutes, &st.QueueOn, &st.RatePerMinute, &st.ActiveLimit, &st.Paused, &st.HoldMinutes,
				&st.CapPerOrder, &st.CapPerPerson, &st.BotDefence, &st.OTPOn, &st.OTPThreshold, &v.At, &v.By); err != nil {
				return err
			}
			st.OpensAt = st.OpensAt.UTC()
			v.At = v.At.UTC()
			hist = append(hist, v)
		}
		return rows.Err()
	})
	return out, hist, err
}

// Update saves a new version of a sale's settings. Live fields (rate, shopping limit, pause, bot
// defence, OTP, holds and caps) change at any time; opening time, pre-queue window and queue on/off
// are refused once the pre-queue opened (EPS-SALE-LOCKED).
func (s *Service) Update(ctx context.Context, org, saleID, by string, st Settings) (*Sale, error) {
	if p := st.Validate(); p != nil {
		return nil, p
	}
	st.OpensAt = st.OpensAt.UTC().Truncate(time.Second)
	cur, _, err := s.Get(ctx, org, saleID)
	if err != nil {
		return nil, err
	}
	if f := structuralChanged(cur.Settings, st); f != "" && cur.Settings.PhaseAt(s.now()) != Before {
		return nil, apperr.New(http.StatusConflict, apperr.SaleLocked, "The opening time, pre-queue and queue cannot change once the pre-queue has opened.", false).With("field", f)
	}
	err = db.WithTx(ctx, s.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var v int
		// The row lock serialises two saves of one sale.
		if err := tx.QueryRow(ctx, `update sales.sales set settings_version = settings_version + 1 where id = $1 returning settings_version`, saleID).Scan(&v); err != nil {
			return err
		}
		if err := insertVersion(ctx, tx, org, saleID, v, st, by); err != nil {
			return err
		}
		return writeEvent(ctx, tx, org, cur.EventID, saleID, "sale.settings_changed", v)
	})
	if err != nil {
		return nil, err
	}
	s.Refresh(ctx, org, cur.EventID)
	out, _, err := s.Get(ctx, org, saleID)
	return out, err
}

// Redis copies: the event's sales (signed) and a directory from sale id to organiser and event.

func salesKey(org, event string) string {
	k, _ := redisx.Key(org, event, "sales")
	return k
}

func dirKey(saleID string) string {
	k, _ := redisx.PlatformKey("sale-dir", saleID)
	return k
}

func (s *Service) sign(b []byte) string {
	m := hmac.New(sha256.New, s.MACKey)
	m.Write(b)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil)) + "." + string(b)
}

func (s *Service) open(v string) ([]byte, bool) {
	mac, body, ok := strings.Cut(v, ".")
	if !ok {
		return nil, false
	}
	m := hmac.New(sha256.New, s.MACKey)
	m.Write([]byte(body))
	want := base64.RawURLEncoding.EncodeToString(m.Sum(nil))
	return []byte(body), hmac.Equal([]byte(mac), []byte(want))
}

// Refresh writes the event's sales from Postgres into Redis.
func (s *Service) Refresh(ctx context.Context, org, event string) {
	if s.Redis == nil {
		return
	}
	list, err := s.List(ctx, org, event)
	if err != nil {
		return
	}
	s.store(ctx, org, event, list)
}

func (s *Service) store(ctx context.Context, org, event string, list []Sale) {
	b, _ := json.Marshal(list)
	pipe := s.Redis.Pipeline()
	pipe.Set(ctx, salesKey(org, event), s.sign(b), 24*time.Hour)
	for _, x := range list {
		pipe.Set(ctx, dirKey(x.ID), org+"/"+event, 24*time.Hour)
	}
	_, _ = pipe.Exec(ctx)
	s.mu.Lock()
	if s.local == nil {
		s.local = map[string]cached{}
	}
	delete(s.local, org+"/"+event)
	s.mu.Unlock()
}

// Live returns the event's sales as the hot path reads them: an in-process copy for 1 second, then
// the signed Redis copy, then Postgres (written back to Redis).
func (s *Service) Live(ctx context.Context, org, event string) ([]Sale, error) {
	key := org + "/" + event
	s.mu.Lock()
	if c, ok := s.local[key]; ok && s.now().Sub(c.at) < time.Second {
		s.mu.Unlock()
		return c.sales, nil
	}
	s.mu.Unlock()
	var list []Sale
	found := false
	if s.Redis != nil {
		if v, err := s.Redis.Get(ctx, salesKey(org, event)).Result(); err == nil {
			if b, ok := s.open(v); ok && json.Unmarshal(b, &list) == nil {
				found = true
			}
		}
	}
	if !found {
		if s.Pool == nil {
			return nil, errors.New("sales: no copy in Redis and no database")
		}
		var err error
		if list, err = s.List(ctx, org, event); err != nil {
			return nil, err
		}
		if s.Redis != nil {
			s.store(ctx, org, event, list)
		}
	}
	s.mu.Lock()
	if s.local == nil {
		s.local = map[string]cached{}
	}
	s.local[key] = cached{at: s.now(), sales: list}
	s.mu.Unlock()
	return list, nil
}

// Lookup finds a sale by id alone (the gateway's URLs carry only the sale): the directory names
// its organiser and event; without it, Postgres is asked once.
func (s *Service) Lookup(ctx context.Context, saleID string) (*Sale, error) {
	if _, err := uuid.Parse(saleID); err != nil {
		return nil, notFound()
	}
	org, event := "", ""
	if s.Redis != nil {
		if v, err := s.Redis.Get(ctx, dirKey(saleID)).Result(); err == nil {
			org, event, _ = strings.Cut(v, "/")
		}
	}
	if org == "" && s.Pool != nil {
		err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `select organiser_id::text, event_id::text from sales.sale_owner($1)`, saleID).Scan(&org, &event)
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notFound()
		}
		if err != nil {
			return nil, err
		}
	}
	if org == "" {
		return nil, notFound()
	}
	list, err := s.Live(ctx, org, event)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == saleID {
			return &list[i], nil
		}
	}
	return nil, notFound()
}

// Current is the sale that governs checkout at t: the latest sale already open. ok is false when
// the event has sales but none is open yet; an event without sales returns nil, true (step 4's
// checkout, no pass needed).
func Current(list []Sale, t time.Time) (*Sale, bool) {
	if len(list) == 0 {
		return nil, true
	}
	var cur *Sale
	for i := range list {
		if !t.Before(list[i].Settings.OpensAt) && (cur == nil || !list[i].Settings.OpensAt.Before(cur.Settings.OpensAt)) {
			cur = &list[i]
		}
	}
	return cur, cur != nil
}

// Next is the next sale to open after t, if any.
func Next(list []Sale, t time.Time) *Sale {
	var next *Sale
	for i := range list {
		if t.Before(list[i].Settings.OpensAt) && (next == nil || list[i].Settings.OpensAt.Before(next.Settings.OpensAt)) {
			next = &list[i]
		}
	}
	return next
}

// RefreshActive rewrites the Redis copies of every event with a sale opening within a day either
// side of now (the worker, every minute), so the gateway finds them after a Redis restart.
func (s *Service) RefreshActive(ctx context.Context) (int, error) {
	var pairs [][2]string
	now := s.now()
	err := db.WithTx(ctx, s.Pool, db.RoleCore, "", func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `select organiser_id::text, event_id::text from sales.events_with_sales($1, $2)`, now.Add(-72*time.Hour), now.Add(24*time.Hour))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var a, b string
			if err := rows.Scan(&a, &b); err != nil {
				return err
			}
			pairs = append(pairs, [2]string{a, b})
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	for _, p := range pairs {
		s.Refresh(ctx, p[0], p[1])
	}
	return len(pairs), nil
}
