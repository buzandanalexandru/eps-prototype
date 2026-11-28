package snapshot_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func fixtureSnap(t *testing.T) *snapshot.Snapshot {
	t.Helper()
	s, err := snapshot.Parse(fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

var demoNow = time.Date(2026, 12, 5, 10, 0, 0, 0, time.UTC)

func product(a snapshot.Availability, id string) *snapshot.ProductAvailability {
	for i := range a.Products {
		if a.Products[i].ProductID == id {
			return &a.Products[i]
		}
	}
	return nil
}

const gaID = "0d000000-0000-4000-8000-000000000001"
const campingID = "0d000000-0000-4000-8000-000000000002"
const arenaPool = "0c000000-0000-4000-8000-000000000001"

// Scenario "Before sales start": status not yet, with the opening time.
func TestNotYetOnSale(t *testing.T) {
	s := fixtureSnap(t)
	tomorrow := demoNow.Add(24 * time.Hour).Format(time.RFC3339)
	s.Products[0].SalesFrom = &tomorrow
	a := snapshot.Compute(s, nil, nil, demoNow)
	ga := product(a, gaID)
	if ga.Status != snapshot.NotYet || ga.OpensAt == nil || !ga.OpensAt.Equal(demoNow.Add(24*time.Hour)) {
		t.Fatalf("GA: %+v", ga)
	}
	if ga.Remaining != nil {
		t.Fatal("remaining shown before sales open")
	}
}

// Scenario "Plenty left": on sale, no remaining count; few left: the count appears.
func TestRemainingOnlyWhenFewLeft(t *testing.T) {
	s := fixtureSnap(t)
	a := snapshot.Compute(s, map[string]int64{arenaPool: 30000}, nil, demoNow)
	if ga := product(a, gaID); ga.Status != snapshot.OnSale || ga.Remaining != nil || ga.AllInMinor != 55248 {
		t.Fatalf("plenty left: %+v", ga)
	}
	a = snapshot.Compute(s, map[string]int64{arenaPool: 38001}, nil, demoNow) // 1,999 left = 4.9975%
	if ga := product(a, gaID); ga.Remaining == nil || *ga.Remaining != 1999 {
		t.Fatalf("5%% left: %+v", ga)
	}
	a = snapshot.Compute(s, map[string]int64{arenaPool: 40000}, nil, demoNow)
	if ga := product(a, gaID); ga.Status != snapshot.SoldOut {
		t.Fatalf("full: %+v", ga)
	}
}

// Scenario "Hidden products": not in the public answer.
func TestHiddenProductsLeftOut(t *testing.T) {
	s := fixtureSnap(t)
	s.Products[1].Visible = false
	a := snapshot.Compute(s, nil, nil, demoNow)
	if product(a, campingID) != nil || product(a, gaID) == nil {
		t.Fatalf("products: %+v", a.Products)
	}
}

func TestEndedAndSalesLadder(t *testing.T) {
	s := fixtureSnap(t)
	past := demoNow.Add(-time.Hour).Format(time.RFC3339)
	s.Products[1].SalesUntil = &past
	a := snapshot.Compute(s, nil, map[string]int64{gaID: 10000}, demoNow)
	if c := product(a, campingID); c.Status != snapshot.Ended {
		t.Fatalf("camping: %+v", c)
	}
	if ga := product(a, gaID); ga.PriceStepIndex != 1 || ga.AllInMinor != 61398 {
		t.Fatalf("GA after 10,000 sold: %+v", ga)
	}
}

type countingUsage struct{ n int }

func (c *countingUsage) PoolUsage(context.Context, string, string) (map[string]int64, error) {
	c.n++
	return map[string]int64{}, nil
}
func (c *countingUsage) ProductSold(context.Context, string, string) (map[string]int64, error) {
	return map[string]int64{}, nil
}

// The answer is computed once per event per 2-second window and shared through Redis.
func TestAvailabilityCachedTwoSeconds(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	w := newWorld(t, pool)
	w.publish(t, 1, 53900)
	rdb := redisClient(t)
	usage := &countingUsage{}
	svc := &snapshot.AvailabilityService{Store: snapshot.NewStore(snapshot.DBSource{Pool: pool}, rdb, nil, snapshot.Options{}), Usage: usage, Redis: rdb, TTL: 300 * time.Millisecond, Now: func() time.Time { return demoNow }}
	a1, err := svc.Get(ctx, w.event)
	if err != nil {
		t.Fatal(err)
	}
	a2, _ := svc.Get(ctx, w.event)
	if usage.n != 1 {
		t.Fatalf("computed %d times within the window", usage.n)
	}
	b1, _ := json.Marshal(a1)
	b2, _ := json.Marshal(a2)
	if string(b1) != string(b2) {
		t.Fatal("cached answer differs")
	}
	time.Sleep(400 * time.Millisecond)
	_, _ = svc.Get(ctx, w.event)
	if usage.n != 2 {
		t.Fatalf("not recomputed after the window: %d", usage.n)
	}
	if snapshot.AvailabilityTTL != 2*time.Second {
		t.Fatal("availability window is 2 seconds")
	}
}

// Per-instance availability: a sold-out day and a day with few left are listed; untouched ones not.
func TestInstancesSoldOutOrFewLeft(t *testing.T) {
	s := fixtureSnap(t)
	d := "0c000000-0000-4000-8000-0000000000d1"
	d1, d2, d3 := "2027-07-15", "2027-07-16", "2027-07-17"
	s.Pools = append(s.Pools, snapshot.Pool{PoolID: d, Name: "Per day", Scope: "day", Instances: []snapshot.PoolInstance{
		{InstanceID: d + "@" + d1, Date: &d1, Capacity: 8000},
		{InstanceID: d + "@" + d2, Date: &d2, Capacity: 8000},
		{InstanceID: d + "@" + d3, Date: &d3, Capacity: 8000},
	}})
	a := snapshot.Compute(s, map[string]int64{d + "@" + d1: 8000, d + "@" + d2: 7990, d + "@" + d3: 10}, nil, demoNow)
	if len(a.Instances) != 2 {
		t.Fatalf("instances: %+v", a.Instances)
	}
	if a.Instances[0].InstanceID != d+"@"+d1 || a.Instances[0].Status != snapshot.SoldOut || a.Instances[0].Remaining != 0 {
		t.Fatalf("sold-out day: %+v", a.Instances[0])
	}
	if a.Instances[1].InstanceID != d+"@"+d2 || a.Instances[1].Status != snapshot.OnSale || a.Instances[1].Remaining != 10 {
		t.Fatalf("few-left day: %+v", a.Instances[1])
	}
}
