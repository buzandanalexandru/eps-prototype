package queue_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

// Scenario "Before the window": a fan who arrives before the pre-queue opens is told when it
// opens and takes no place.
func TestJoinBeforeTheWindow(t *testing.T) {
	env := onsaletest.Setup(t)
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), func() (st salesSettings) {
		st = env.Open()
		st.OpensAt = env.Clock.Now().Add(2 * time.Hour).Truncate(time.Second)
		return
	}())
	st := env.Join(t, s, onsaletest.NewFan())
	if st.State != queue.PlaceBefore || st.QueueToken != "" || !st.PrequeueOpensAt.Equal(s.Settings.OpensAt.Add(-30*time.Minute)) {
		t.Fatalf("status %+v", st)
	}
	c, _ := queue.CountsOf(context.Background(), env.Redis, s.OrganiserID, s.EventID, s.ID, env.Clock.Now())
	if c.Prequeue != 0 || c.Line != 0 {
		t.Fatalf("a place was taken: %+v", c)
	}
}

// Scenario "Second join with the same account": another device gets the same place.
func TestSecondJoinSameAccount(t *testing.T) {
	env := onsaletest.Setup(t)
	st := env.Open()
	st.OpensAt = env.Clock.Now().Add(20 * time.Minute).Truncate(time.Second)
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	phone := onsaletest.NewFan()
	first := env.Join(t, s, phone)
	if first.State != queue.PlacePrequeue {
		t.Fatalf("state %s, want prequeue", first.State)
	}
	laptop := &onsaletest.Fan{Buyer: phone.Buyer, Session: phone.Session + "-laptop", IP: "192.0.2.77"}
	env.Join(t, s, laptop)
	a, _ := env.Engine.Verifier().Verify(phone.Token, queue.KindQueue, env.Clock.Now(), false)
	b, _ := env.Engine.Verifier().Verify(laptop.Token, queue.KindQueue, env.Clock.Now(), false)
	if a == nil || b == nil || a.QID != b.QID {
		t.Fatalf("two places: %v and %v", a, b)
	}
	c, _ := queue.CountsOf(context.Background(), env.Redis, s.OrganiserID, s.EventID, s.ID, env.Clock.Now())
	if c.Prequeue != 1 {
		t.Fatalf("pre-queue has %d places, want 1", c.Prequeue)
	}
}

// Scenario "No queue": with the queue off, a pass is issued at once and checkout opens.
func TestQueueOffIssuesPassAtOnce(t *testing.T) {
	env := onsaletest.Setup(t)
	sale := env.Env.NewSale(t, 100, 100)
	st := env.Open()
	st.QueueOn = false
	s := env.NewSale(t, sale, st)
	f := onsaletest.NewFan()
	got := env.Join(t, s, f)
	if got.State != queue.PlaceAdmitted || got.Pass == "" {
		t.Fatalf("status %+v, want a pass at once", got)
	}
	f.Buyer.Pass, f.Buyer.Session = f.Pass, f.Session
	sale.Hold(t, f.Buyer, salestestGA(2))
}

// D-5: an eleventh join in a minute from one address is refused with 429; proof of work is checked.
func TestJoinRateLimitAndProofOfWork(t *testing.T) {
	env := onsaletest.Setup(t)
	env.Engine.JoinsPerMinute = 10
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), env.Open())
	for i := 0; i < 10; i++ {
		env.Join(t, s, onsaletest.NewFan())
	}
	f := onsaletest.NewFan()
	_, err := env.Engine.Join(context.Background(), s.ID, queue.JoinInput{Sub: f.Buyer.ID, Kind: "account", Session: f.Session, IP: f.IP})
	if !errors.Is(err, queue.ErrRateLimited) {
		t.Fatalf("11th join: %v", err)
	}
	sid := queue.SessionHash("a-session-secret-of-the-page")
	if !queue.PowOK(s.ID, sid, queue.SolvePow(s.ID, sid)) || queue.PowOK(s.ID, sid, "1") && queue.PowOK(s.ID, sid, "2") {
		t.Fatal("proof of work check")
	}
}
