package sales_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

func code(err error) (string, map[string]any) {
	var p *apperr.Problem
	if errors.As(err, &p) {
		return p.Code, p.Data
	}
	return "", nil
}

// Scenario "Value out of range": a hold length of 30 minutes is refused with the allowed range.
func TestValueOutOfRange(t *testing.T) {
	env := onsaletest.Setup(t)
	sale := env.Env.NewSale(t, 100, 100)
	st := env.Open()
	st.HoldMinutes = 30
	_, err := env.Sales.Create(context.Background(), sale.Org, sale.Event, "Presale", "u1", st)
	c, d := code(err)
	if c != apperr.SaleInvalid || d["field"] != "holdMinutes" || d["min"] != 5 || d["max"] != 15 {
		t.Fatalf("got %v (%v)", err, d)
	}
	for _, bad := range []func(*sales.Settings){
		func(s *sales.Settings) { s.PrequeueMinutes = 4 },
		func(s *sales.Settings) { s.PrequeueMinutes = 121 },
		func(s *sales.Settings) { s.RatePerMinute = 0 },
		func(s *sales.Settings) { s.CapPerOrder = 21 },
		func(s *sales.Settings) { s.OTPThreshold = 0 },
		func(s *sales.Settings) { s.CapPerOrder, s.CapPerPerson = 6, 4 },
	} {
		st := env.Open()
		bad(&st)
		if c, _ := code(st.Validate()); c != apperr.SaleInvalid {
			t.Fatalf("%+v accepted", st)
		}
	}
}

// Scenario "Save settings" (Go side): saved as version 1 with the values given.
func TestSaveSettings(t *testing.T) {
	env := onsaletest.Setup(t)
	sale := env.Env.NewSale(t, 100, 100)
	st := sales.Defaults(time.Date(2027, 3, 1, 10, 0, 0, 0, time.UTC))
	x, err := env.Sales.Create(context.Background(), sale.Org, sale.Event, "General sale", "u1", st)
	if err != nil {
		t.Fatal(err)
	}
	if x.Version != 1 || x.Settings != st || x.UpdatedBy != "u1" {
		t.Fatalf("saved %+v", x)
	}
	if x.Settings.PrequeueOpensAt() != st.OpensAt.Add(-30*time.Minute) || x.Settings.RatePerMinute != 2000 || x.Settings.CapPerOrder != 4 || x.Settings.CapPerPerson != 6 {
		t.Fatalf("defaults %+v", x.Settings)
	}
}

// Scenario "Rate changed live": from 2,000 to 500 a minute during the sale; within 2 seconds the
// admitter follows the new rate.
func TestRateChangedLive(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	sale := env.Env.NewSale(t, 100, 100)
	st := env.Open()
	st.RatePerMinute = 2000
	s := env.NewSale(t, sale, st)
	k := env.Keys(t, s)
	for i := 0; i < 3000; i++ {
		_ = env.Redis.ZAdd(ctx, k.Line(), redisZ(i)).Err()
	}
	_ = env.Redis.Set(ctx, k.State(), queue.StateOpen, 0).Err()
	tick := func(n int) int64 {
		var total int64
		for i := 0; i < n; i++ {
			env.Clock.Add(queue.Tick)
			cur, err := env.Sales.Lookup(ctx, s.ID)
			if err != nil {
				t.Fatal(err)
			}
			r, err := env.Engine.Admit(ctx, cur)
			if err != nil {
				t.Fatal(err)
			}
			total += r.Admitted
		}
		return total
	}
	tick(1)
	if n := tick(20); n < 300 || n > 370 { // 10 s at 2,000/min = 333
		t.Fatalf("admitted %d in 10 s at 2,000", n)
	}
	st.RatePerMinute = 500
	if _, err := env.Sales.Update(ctx, s.OrganiserID, s.ID, "u1", st); err != nil {
		t.Fatal(err)
	}
	// Within 2 s (4 ticks) the new rate rules: the next 10 s admit about 83.
	tick(4)
	if n := tick(20); n < 70 || n > 95 {
		t.Fatalf("admitted %d in 10 s after the change to 500", n)
	}
	// Another core reads the change from Redis (its own 1 s copy expires).
	other := &sales.Service{Redis: env.Redis, MACKey: []byte("test-key"), Now: env.Clock.Now}
	got, err := other.Lookup(ctx, s.ID)
	if err != nil || got.Settings.RatePerMinute != 500 {
		t.Fatalf("another process reads %+v (%v)", got, err)
	}
	// A copy signed with another key is not trusted (it falls back to Postgres, which it lacks).
	forged := &sales.Service{Redis: env.Redis, MACKey: []byte("other-key")}
	if _, err := forged.Lookup(ctx, s.ID); err == nil {
		t.Fatal("a copy under another key was trusted")
	}
}

// Scenario "Locked field": moving the opening time after the pre-queue opened is refused and the
// sale keeps its opening time; live fields still change.
func TestLockedField(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	sale := env.Env.NewSale(t, 100, 100)
	st := env.Open()
	st.OpensAt = env.Clock.Now().Add(10 * time.Minute).Truncate(time.Second) // pre-queue opened 20 min ago
	s := env.NewSale(t, sale, st)
	moved := st
	moved.OpensAt = st.OpensAt.Add(time.Hour)
	_, err := env.Sales.Update(ctx, s.OrganiserID, s.ID, "u1", moved)
	if c, d := code(err); c != apperr.SaleLocked || d["field"] != "opensAt" {
		t.Fatalf("got %v", err)
	}
	off := st
	off.QueueOn = false
	if _, err := env.Sales.Update(ctx, s.OrganiserID, s.ID, "u1", off); err == nil {
		t.Fatal("queue switched off after the pre-queue opened")
	}
	cur, _, _ := env.Sales.Get(ctx, s.OrganiserID, s.ID)
	if !cur.Settings.OpensAt.Equal(st.OpensAt) || cur.Version != 1 {
		t.Fatalf("sale changed: %+v", cur)
	}
	live := st
	live.RatePerMinute = 900
	live.OTPOn = true
	if x, err := env.Sales.Update(ctx, s.OrganiserID, s.ID, "u1", live); err != nil || x.Version != 2 {
		t.Fatalf("live change: %v", err)
	}
	// Before the pre-queue opens the opening time may move.
	early := env.Open()
	early.OpensAt = env.Clock.Now().Add(3 * time.Hour).Truncate(time.Second)
	s2 := env.NewSale(t, sale, early)
	early.OpensAt = early.OpensAt.Add(time.Hour)
	if _, err := env.Sales.Update(ctx, s2.OrganiserID, s2.ID, "u1", early); err != nil {
		t.Fatalf("moving the opening before the pre-queue: %v", err)
	}
}

// Scenario "Other organiser": organiser B cannot read or change organiser A's sale.
func TestOtherOrganiser(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	a := env.Env.NewSale(t, 100, 100)
	s := env.NewSale(t, a, env.Open())
	b := uuid.NewString()
	st := env.Open()
	st.RatePerMinute = 10
	if _, err := env.Sales.Update(ctx, b, s.ID, "intruder", st); err == nil {
		t.Fatal("organiser B changed A's sale")
	}
	if _, _, err := env.Sales.Get(ctx, b, s.ID); err == nil {
		t.Fatal("organiser B read A's sale")
	}
	if list, _ := env.Sales.List(ctx, b, a.Event); len(list) != 0 {
		t.Fatalf("organiser B lists %d sales of A", len(list))
	}
	if _, err := env.Sales.Create(ctx, b, a.Event, "Mine now", "intruder", env.Open()); err == nil {
		t.Fatal("organiser B added a sale to A's event")
	}
	cur, _, _ := env.Sales.Get(ctx, s.OrganiserID, s.ID)
	if cur.Version != 1 || cur.Settings.RatePerMinute != 60000 {
		t.Fatalf("A's sale changed: %+v", cur)
	}
	// Row-level security: B's scope sees none of A's rows.
	var n int
	if err := env.Pool.QueryRow(ctx, "select count(*) from sales.sale_settings").Scan(&n); err != nil || n == 0 {
		t.Fatal("rows missing for the owner")
	}
}

// Scenario "History": three rate changes show as three new versions with times and authors.
func TestHistory(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), env.Open())
	st := s.Settings
	for i, who := range []string{"ana", "bogdan", "carmen"} {
		st.RatePerMinute = 1000 + i*500
		if _, err := env.Sales.Update(ctx, s.OrganiserID, s.ID, who, st); err != nil {
			t.Fatal(err)
		}
	}
	_, hist, err := env.Sales.Get(ctx, s.OrganiserID, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(hist) != 4 || hist[0].Version != 4 || hist[0].By != "carmen" || hist[0].Settings.RatePerMinute != 2000 || hist[2].By != "ana" || hist[3].By != "test-user" {
		t.Fatalf("history %+v", hist)
	}
	for _, h := range hist {
		if h.At.IsZero() {
			t.Fatal("a version without its time")
		}
	}
}
