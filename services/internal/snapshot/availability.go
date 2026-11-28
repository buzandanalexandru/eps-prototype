package snapshot

import (
	"context"
	"encoding/json"
	"sort"
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
}

// Availability is an event's public availability, from one snapshot version.
type Availability struct {
	EventID     string                `json:"eventId"`
	Version     int                   `json:"version"`
	GeneratedAt time.Time             `json:"generatedAt"`
	Products    []ProductAvailability `json:"products"`
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
	pools := map[string]Pool{}
	for _, p := range s.Pools {
		pools[p.PoolID] = p
	}
	out := Availability{EventID: s.EventID, Version: s.Version, GeneratedAt: now.UTC(), Products: []ProductAvailability{}}
	prods := append([]Product(nil), s.Products...)
	sort.SliceStable(prods, func(i, j int) bool { return prods[i].SortOrder < prods[j].SortOrder })
	for _, p := range prods {
		if !p.Visible {
			continue
		}
		idx := ActiveStep(p.PriceSteps, now, productSold[p.ProductID])
		pa := ProductAvailability{ProductID: p.ProductID, PriceStepIndex: idx, AllInMinor: p.PriceSteps[idx].AllInMinor}
		var remaining, capacity int64 = -1, 0
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
		default:
			pa.Status = OnSale
		}
		if pa.Status == OnSale && remaining > 0 && (remaining <= FewLeftAbsolute || remaining*10000 <= capacity*FewLeftShareBps) {
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
			var cached Availability
			if json.Unmarshal(b, &cached) == nil && cached.Version == snap.Version {
				return cached, nil
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
	out := Compute(snap, used, sold, now())
	if a.Redis != nil && kerr == nil {
		if b, err := json.Marshal(out); err == nil {
			_ = a.Redis.Set(ctx, key, b, ttl).Err()
		}
	}
	return out, nil
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
