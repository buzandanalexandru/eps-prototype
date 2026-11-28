// Package salestest builds a published sale for the checkout, payments and wallet tests: an
// organiser, a brand, an event with a live snapshot, and a checkout service over testcontainers
// Postgres and Redis. Used by tests only.
package salestest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

// Fixed ids of the sample sale's products and pools.
const (
	PoolArena     = "0c000000-0000-4000-8000-0000000000a1"
	PoolPlus      = "0c000000-0000-4000-8000-0000000000a2"
	PoolDay       = "0c000000-0000-4000-8000-0000000000a3"
	ProductGA     = "0d000000-0000-4000-8000-0000000000b1" // pass on: 539.00 face, 552.48 all-in
	ProductPlus   = "0d000000-0000-4000-8000-0000000000b2" // absorb: 539.00 all-in, Arena + GA Plus pools
	ProductHidden = "0d000000-0000-4000-8000-0000000000b3"
	ProductAddOn  = "0d000000-0000-4000-8000-0000000000b4" // needs GA
	ProductDay    = "0d000000-0000-4000-8000-0000000000b5" // a day ticket on the day pool
)

// Env is the test environment.
type Env struct {
	Pool  *pgxpool.Pool
	Redis *redis.Client
	Store *snapshot.Store
	Svc   *checkout.Service
	Log   *slog.Logger
	Index checkout.HoldIndex
}

// Sale is one organiser's published event.
type Sale struct {
	Org, Brand, Event string
	Version           int
	ArenaCap, PlusCap int64
	env               *Env
}

// Setup starts (once per test binary) the containers and returns a fresh database and a flushed
// Redis with a checkout service over them.
func Setup(t *testing.T) *Env {
	t.Helper()
	pool, _ := testdb.Migrated(t)
	rdb, err := redisx.Open(testdb.RedisURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rdb.Close() })
	if err := rdb.FlushAll(context.Background()).Err(); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := snapshot.NewStore(snapshot.DBSource{Pool: pool}, rdb, log, snapshot.Options{MemTTL: time.Millisecond, RedisTTL: time.Millisecond, MissingTTL: time.Millisecond, MACKey: []byte("test-key")})
	inv := &checkout.RedisInventory{Redis: rdb}
	svc := &checkout.Service{Pool: pool, Inv: inv, Snaps: store, Links: &checkout.Links{Key: []byte("link-key")}, Log: log}
	return &Env{Pool: pool, Redis: rdb, Store: store, Svc: svc, Log: log, Index: inv}
}

func str(s string) *string { return &s }

// NewSale creates an organiser with a published event: Arena (arenaCap) and GA Plus (plusCap)
// pools and the five sample products.
func (e *Env) NewSale(t *testing.T, arenaCap, plusCap int64) *Sale {
	t.Helper()
	org, brand := uuid.NewString(), uuid.NewString()
	slug := "t" + strings.ReplaceAll(org[:8], "-", "")
	ctx := context.Background()
	if _, err := e.Pool.Exec(ctx, "insert into control.organisers (id, slug, name) values ($1, $2, 'Org')", org, slug); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Pool.Exec(ctx, "insert into control.brands (id, organiser_id, slug, name) values ($1, $2, $3, 'Brand')", brand, org, slug+"-b"); err != nil {
		t.Fatal(err)
	}
	return e.NewSaleFor(t, org, brand, arenaCap, plusCap)
}

// NewSaleFor adds another published event to an existing organiser and brand.
func (e *Env) NewSaleFor(t *testing.T, org, brand string, arenaCap, plusCap int64) *Sale {
	t.Helper()
	s := &Sale{Org: org, Brand: brand, Event: uuid.NewString(), ArenaCap: arenaCap, PlusCap: plusCap, env: e}
	if _, err := e.Pool.Exec(context.Background(), "insert into control.events (id, organiser_id, brand_id, slug, starts_at, ends_at) values ($1, $2, $3, $4, '2027-07-15T14:00Z', '2027-07-19T00:00Z')",
		s.Event, org, brand, "ev-"+strings.ReplaceAll(s.Event[:8], "-", "")); err != nil {
		t.Fatal(err)
	}
	s.Publish(t, nil)
	return s
}

func step(face, fan, net int64) map[string]any {
	return map[string]any{"index": 0, "mode": "by_date", "fromAt": nil, "afterSold": nil, "faceMinor": face, "allInMinor": face + fan, "fanFeeMinor": fan, "organiserNetMinor": net}
}

func product(id, typ, name string, pools []string, st map[string]any, visible bool) map[string]any {
	return map[string]any{"productId": id, "type": typ, "name": map[string]string{"ro": name, "en": name}, "perks": []any{}, "zone": nil,
		"config": map[string]any{}, "named": false, "salesFrom": nil, "salesUntil": nil, "visible": visible, "maxPerOrder": 6, "sortOrder": 1,
		"poolIds": pools, "requires": []any{}, "priceSteps": []any{st}, "instalments": nil}
}

// Doc is the snapshot document of the next version; edit changes it before it is stored.
func (s *Sale) doc(version int, edit func(map[string]any)) map[string]any {
	inst := func(cap int64, id string) []any {
		return []any{map[string]any{"instanceId": id, "date": nil, "slotStart": nil, "capacity": cap}}
	}
	ga := product(ProductGA, "pass", "General Access", []string{PoolArena}, step(53900, 1348, 53900), true)
	ga["instalments"] = map[string]any{"depositMinor": 1000, "depositBps": nil, "monthlyCount": 9, "dayOfMonth": 1, "lastDueBy": "2027-07-08"}
	plus := product(ProductPlus, "pass", "GA Plus", []string{PoolArena, PoolPlus}, step(53900, 0, 52552), true)
	plus["instalments"] = map[string]any{"depositMinor": 1000, "depositBps": nil, "monthlyCount": 9, "dayOfMonth": 1, "lastDueBy": "2027-07-08"}
	hidden := product(ProductHidden, "pass", "Crew", []string{PoolArena}, step(1000, 25, 1000), false)
	addon := product(ProductAddOn, "add_on", "Camping", []string{PoolArena}, step(10000, 250, 10000), true)
	addon["requires"] = []any{ProductGA}
	day := product(ProductDay, "day_ticket", "Day ticket", []string{PoolDay}, step(20000, 500, 20000), true)
	doc := map[string]any{
		"schemaVersion": 1, "organiserId": s.Org, "brandId": s.Brand, "eventId": s.Event, "version": version,
		"publishedAt": time.Now().UTC().Format(time.RFC3339), "currency": "RON", "timezone": "Europe/Bucharest", "isSample": false,
		"fee":   map[string]any{"policy": "pass_on", "bps": 250, "fixedMinor": 0},
		"event": map[string]any{"slug": "galaxia-2027", "name": map[string]string{"ro": "Galaxia", "en": "Galaxia"}, "description": map[string]string{"ro": "", "en": ""}, "startsAt": "2027-07-15T14:00:00.000Z", "endsAt": "2027-07-19T00:00:00.000Z", "timezone": "Europe/Bucharest", "venue": nil, "days": []any{}, "lineup": []any{}},
		"rules": map[string]any{"minAge": nil, "consentUnderAge": nil, "maxPerAccount": 6, "holdMinutes": 8, "checkoutQuestions": map[string]any{"afterContact": []any{}, "consents": []any{}}},
		"pools": []any{
			map[string]any{"poolId": PoolArena, "name": "Arena", "scope": "event", "instances": inst(s.ArenaCap, PoolArena)},
			map[string]any{"poolId": PoolPlus, "name": "GA Plus", "scope": "event", "instances": inst(s.PlusCap, PoolPlus)},
			map[string]any{"poolId": PoolDay, "name": "Days", "scope": "day", "instances": []any{
				map[string]any{"instanceId": PoolDay + "@2027-07-15", "date": "2027-07-15", "slotStart": nil, "capacity": 100},
				map[string]any{"instanceId": PoolDay + "@2027-07-16", "date": "2027-07-16", "slotStart": nil, "capacity": 100},
			}},
		},
		"products": []any{ga, plus, hidden, addon, day},
	}
	if edit != nil {
		edit(doc)
	}
	return doc
}

// Publish stores the next snapshot version (after edit) and makes it live.
func (s *Sale) Publish(t *testing.T, edit func(doc map[string]any)) {
	t.Helper()
	ctx := context.Background()
	s.Version++
	id := uuid.NewString()
	d := s.doc(s.Version, edit)
	d["snapshotId"] = id
	b, _ := json.Marshal(d)
	sum := sha256.Sum256(b)
	if _, err := s.env.Pool.Exec(ctx, `insert into control.sale_snapshots (id, organiser_id, event_id, version, schema_version, document_text, sha256, terms_sha256)
		values ($1, $2, $3, $4, 1, $5, $6, $6)`, id, s.Org, s.Event, s.Version, string(b), hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
	if _, err := s.env.Pool.Exec(ctx, "update control.events set live_snapshot_version = $2, status = 'published' where id = $1", s.Event, s.Version); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond) // past the store's 1 ms caches
}

// OffSale takes the event off sale.
func (s *Sale) OffSale(t *testing.T) {
	if _, err := s.env.Pool.Exec(context.Background(), "update control.events set live_snapshot_version = null where id = $1", s.Event); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
}

// SetCapacity republishes with new capacities.
func (s *Sale) SetCapacity(t *testing.T, arena, plus int64) {
	s.ArenaCap, s.PlusCap = arena, plus
	s.Publish(t, nil)
}

// Buyer returns a fresh account buyer.
func Buyer() checkout.Buyer { return checkout.Buyer{ID: uuid.NewString(), Kind: "account"} }

// Guest returns a fresh guest buyer.
func Guest() checkout.Buyer { return checkout.Buyer{ID: "guest:" + uuid.NewString(), Kind: "guest"} }

// Request is a hold request for this sale's live version.
func (s *Sale) Request(items ...checkout.Item) checkout.CreateRequest {
	return checkout.CreateRequest{EventID: s.Event, Version: s.Version, Items: items, Locale: "en"}
}

// Hold creates an order and fails the test on error.
func (s *Sale) Hold(t *testing.T, b checkout.Buyer, items ...checkout.Item) *checkout.Order {
	t.Helper()
	o, _, err := s.env.Svc.CreateOrder(context.Background(), b, s.Request(items...), "")
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	return o
}

// Used is the inventory's units sold plus held per instance.
func (s *Sale) Used(t *testing.T) map[string]int64 {
	t.Helper()
	u, err := checkout.Usage{Svc: s.env.Svc}.PoolUsage(context.Background(), s.Org, s.Event)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// Details gives contact details to an order.
func (s *Sale) Details(t *testing.T, o *checkout.Order) *checkout.Order {
	t.Helper()
	o2, err := s.env.Svc.SetDetails(context.Background(), o, checkout.Details{Name: "Ana Pop", Email: "ana@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	return o2
}

// SetHoldDeadline moves an order's deadline (as the migrating owner) and its inventory hold.
func (s *Sale) SetHoldDeadline(t *testing.T, orderID string, at time.Time) {
	t.Helper()
	if _, err := s.env.Pool.Exec(context.Background(), "update sales.orders set hold_expires_at = $2 where id = $1", orderID, at); err != nil {
		t.Fatal(err)
	}
}

var _ = str
