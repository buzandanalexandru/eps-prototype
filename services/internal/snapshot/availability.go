package snapshot

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/redisx"
)

// Usage is the stock source: units sold plus held per pool instance, and units sold per product
// (for sales ladders). Step 4 reads them from the sales schema; until then nothing is used.
type Usage interface {
	PoolUsage(ctx context.Context, organiserID, eventID string) (map[string]int64, error)
	ProductSold(ctx context.Context, organiserID, eventID string) (map[string]int64, error)
}

// NoUsage is the stock source before any sales table exists: every instance is unused.
type NoUsage struct{}

func (NoUsage) PoolUsage(context.Context, string, string) (map[string]int64, error) {
	return map[string]int64{}, nil
}
func (NoUsage) ProductSold(context.Context, string, string) (map[string]int64, error) {
	return map[string]int64{}, nil
}

// Status values of a product's availability.
const (
	OnSale  = "on_sale"
	NotYet  = "not_yet"
	Ended   = "ended"
	SoldOut = "sold_out"
)

// FewLeftAbsolute and FewLeftShareBps: remaining is shown only at or below 50 units or 5% of capacity.
const (
	FewLeftAbsolute = 50
	FewLeftShareBps = 500
)

// ProductAvailability is one product's public availability.
type ProductAvailability struct {
	ProductID      string     `json:"productId"`
	Status         string     `json:"status"`
	OpensAt        *time.Time `json:"opensAt,omitempty"`
	PriceStepIndex int        `json:"priceStepIndex"`
	AllInMinor     int64      `json:"allInMinor"`
	Remaining      *int64     `json:"remaining,omitempty"`
	// AllHeld is set on a sold-out product whose stock is held, not sold (step 5, the honest sold
	// out): some may come back.
	AllHeld bool `json:"allHeld,omitempty"`
}

// InstanceAvailability is one pool instance (an event pool, a day or a slot) that is sold out or
// has few left; instances not listed have room.
type InstanceAvailability struct {
	InstanceID string `json:"instanceId"`
	Status     string `json:"status"` // on_sale | sold_out
	Remaining  int64  `json:"remaining"`
	AllHeld    bool   `json:"allHeld,omitempty"`
}

// Availability is an event's public availability, from one snapshot version.
type Availability struct {
	EventID     string                 `json:"eventId"`
	Version     int                    `json:"version"`
	GeneratedAt time.Time              `json:"generatedAt"`
	Products    []ProductAvailability  `json:"products"`
	Instances   []InstanceAvailability `json:"instances"`
}

// fewLeft is the rule for showing a remaining count: at most 50, or at most 5% of capacity.
func fewLeft(remaining, capacity int64) bool {
	return remaining <= FewLeftAbsolute || remaining*10000 <= capacity*FewLeftShareBps
}

// ActiveStep is the index of the price step in force at `at` with `sold` units sold: the last step
// whose threshold is reached (the first has none). The same rule as lib/pricing/ladder.ts.
func ActiveStep(steps []PriceStep, at time.Time, sold int64) int {
	idx := 0
	for i := 1; i < len(steps); i++ {
		st := steps[i]
		reached := false
		if st.Mode == "by_date" {
			t, ok := parseTime(st.FromAt)
			reached = !ok || !t.After(at)
		} else {
			reached = st.AfterSold == nil || sold >= *st.AfterSold
		}
		if reached {
			idx = i
		}
	}
	return idx
}

// Compute derives availability from a snapshot and usage. Hidden products are left out. A product
// is sold out when any event-scoped pool it draws on is full; for day and slot pools, availability
// is per day or slot (the picker asks), so they do not decide the product's status here.
func Compute(s *Snapshot, poolUsed, productSold map[string]int64, now time.Time) Availability {
	return ComputeHeld(s, poolUsed, nil, productSold, now)
}

// HeldUsage is the units held (not sold) per pool instance (step 5's sharded inventory).
type HeldUsage interface {
	PoolHeld(ctx context.Context, organiserID, eventID string) (map[string]int64, error)
}

// ComputeHeld is Compute knowing which units are only held: a full pool with holds left is still
// "sold_out" (nothing can be bought now) but marked allHeld, so the shop can say some may come back.
func ComputeHeld(s *Snapshot, poolUsed, poolHeld, productSold map[string]int64, now time.Time) Availability {
	pools := map[string]Pool{}
	for _, p := range s.Pools {
		pools[p.PoolID] = p
	}
	out := Availability{EventID: s.EventID, Version: s.Version, GeneratedAt: now.UTC(), Products: []ProductAvailability{}, Instances: []InstanceAvailability{}}
	// Per instance (days and slots included): only those something was taken from and that are
	// sold out or have few left, so an answer stays small for a season of 1,480 slots.
	for _, p := range s.Pools {
		for _, in := range p.Instances {
			used := poolUsed[in.InstanceID]
			if used <= 0 {
				continue
			}
			left := in.Capacity - used
			if left < 0 {
				left = 0
			}
			if !fewLeft(left, in.Capacity) {
				continue
			}
			st := OnSale
			if left == 0 {
				st = SoldOut
			}
			out.Instances = append(out.Instances, InstanceAvailability{InstanceID: in.InstanceID, Status: st, Remaining: left, AllHeld: left == 0 && poolHeld[in.InstanceID] > 0})
		}
	}
	prods := append([]Product(nil), s.Products...)
	sort.SliceStable(prods, func(i, j int) bool { return prods[i].SortOrder < prods[j].SortOrder })
	for _, p := range prods {
		if !p.Visible {
			continue
		}
		idx := ActiveStep(p.PriceSteps, now, productSold[p.ProductID])
		pa := ProductAvailability{ProductID: p.ProductID, PriceStepIndex: idx, AllInMinor: p.PriceSteps[idx].AllInMinor}
		var remaining, capacity int64 = -1, 0
		held := false
		for _, id := range p.PoolIDs {
			pool, ok := pools[id]
			if !ok || pool.Scope != "event" || len(pool.Instances) != 1 {
				continue
			}
			in := pool.Instances[0]
			left := in.Capacity - poolUsed[in.InstanceID]
			if left < 0 {
				left = 0
			}
			if remaining < 0 || left < remaining {
				remaining, capacity = left, in.Capacity
			}
			if left == 0 && poolHeld[in.InstanceID] > 0 {
				held = true
			}
		}
		from, hasFrom := parseTime(p.SalesFrom)
		until, hasUntil := parseTime(p.SalesUntil)
		switch {
		case hasUntil && !now.Before(until):
			pa.Status = Ended
		case hasFrom && now.Before(from):
			pa.Status = NotYet
			f := from.UTC()
			pa.OpensAt = &f
		case remaining == 0:
			pa.Status = SoldOut
			pa.AllHeld = held
		default:
			pa.Status = OnSale
		}
		if pa.Status == OnSale && remaining > 0 && fewLeft(remaining, capacity) {
			r := remaining
			pa.Remaining = &r
		}
		out.Products = append(out.Products, pa)
	}
	return out
}

// Availability serves an event's public availability, cached AvailabilityTTL in Redis (shared by
// every core) so a crowd refreshing a shop costs one computation per event per window.
type AvailabilityService struct {
	Store *Store
	Usage Usage
	// Held tells held from sold units (nil: every full pool reads plain sold out).
	Held  HeldUsage
	Redis *redis.Client
	TTL   time.Duration
	Now   func() time.Time
}

// AvailabilityTTL is how long an answer is reused (and the Cache-Control max-age).
const AvailabilityTTL = 2 * time.Second

func (a *AvailabilityService) Get(ctx context.Context, eventID string) (Availability, error) {
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	ttl := a.TTL
	if ttl <= 0 {
		ttl = AvailabilityTTL
	}
	snap, err := a.Store.Load(ctx, eventID)
	if err != nil {
		return Availability{}, err
	}
	key, kerr := redisx.Key(snap.OrganiserID, eventID, "availability", "v"+itoa(snap.Version))
	if a.Redis != nil && kerr == nil {
		if b, err := a.Redis.Get(ctx, key).Bytes(); err == nil {
			// "<signature>.<json>": an answer without a valid signature is ignored.
			if sig, body, ok := strings.Cut(string(b), "."); ok && a.Store.Verify(sig, "availability", key, body) {
				var cached Availability
				if json.Unmarshal([]byte(body), &cached) == nil && cached.Version == snap.Version {
					return cached, nil
				}
			}
		}
	}
	usage := a.Usage
	if usage == nil {
		usage = NoUsage{}
	}
	used, err := usage.PoolUsage(ctx, snap.OrganiserID, eventID)
	if err != nil {
		return Availability{}, err
	}
	sold, err := usage.ProductSold(ctx, snap.OrganiserID, eventID)
	if err != nil {
		return Availability{}, err
	}
	var held map[string]int64
	if a.Held != nil {
		if held, err = a.Held.PoolHeld(ctx, snap.OrganiserID, eventID); err != nil {
			return Availability{}, err
		}
	}
	out := ComputeHeld(snap, used, held, sold, now())
	if a.Redis != nil && kerr == nil {
		if b, err := json.Marshal(out); err == nil {
			_ = a.Redis.Set(ctx, key, a.Store.Sign("availability", key, string(b))+"."+string(b), ttl).Err()
		}
	}
	return out, nil
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
