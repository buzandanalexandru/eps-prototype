// Package onsaletest builds the step 5 on-sale engine over salestest's published sale for the
// tests: sale settings, a waiting room with its own pass key and a test clock, and the checkout
// gate. Used by tests only.
package onsaletest

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsale"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// Clock is a test clock that only moves when told.
type Clock struct {
	mu sync.Mutex
	t  time.Time
}

// Now is the clock's time.
func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Add moves the clock.
func (c *Clock) Add(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

// Set puts the clock at t.
func (c *Clock) Set(t time.Time) {
	c.mu.Lock()
	c.t = t
	c.mu.Unlock()
}

// Env is the on-sale test environment.
type Env struct {
	*salestest.Env
	Clock  *Clock
	Sales  *sales.Service
	Engine *queue.Engine
	Gate   *onsale.Gate
	Life   *onsale.Lifecycle
	Scorer *botdefence.Scorer
	Caps   *botdefence.Caps
	Key    ed25519.PrivateKey
}

// Setup builds the engine with a clock at the real time (the checkout's own clock is real).
func Setup(t *testing.T) *Env {
	t.Helper()
	base := salestest.Setup(t)
	clk := &Clock{t: time.Now()}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	sv := &sales.Service{Pool: base.Pool, Redis: base.Redis, MACKey: []byte("test-key"), Now: clk.Now,
		Published: func(ctx context.Context, event string) (string, error) {
			s, err := base.Store.Load(ctx, event)
			if err != nil {
				return "", nil
			}
			return s.OrganiserID, nil
		}}
	scorer := &botdefence.Scorer{Redis: base.Redis}
	caps := &botdefence.Caps{Redis: base.Redis}
	engine := &queue.Engine{Redis: base.Redis, Sales: sv, Signer: queue.NewSigner(priv), Scorer: scorer, Now: clk.Now, JoinsPerMinute: 100000, NewGuestsPerMinute: 100000, Secret: []byte("test-secret")}
	gate := &onsale.Gate{Sales: sv, Redis: base.Redis, Pool: base.Pool, Passes: &queue.PassCheck{Verifier: queue.NewVerifier(priv.Public().(ed25519.PublicKey)), Redis: base.Redis},
		Caps: caps, Scorer: scorer, OTP: &botdefence.OTP{Redis: base.Redis, Now: clk.Now}, Checkout: base.Svc, Now: clk.Now, Secret: []byte("test-secret")}
	base.Svc.Gate = gate
	return &Env{Env: base, Clock: clk, Sales: sv, Engine: engine, Gate: gate, Scorer: scorer, Caps: caps, Key: priv,
		Life: &onsale.Lifecycle{Checkout: base.Svc, Engine: engine, Caps: caps}}
}

// Open are settings of a sale that opened a minute ago, with room to admit everyone at once.
func (e *Env) Open() sales.Settings {
	st := sales.Defaults(e.Clock.Now().Add(-time.Minute).Truncate(time.Second))
	st.RatePerMinute = 60000
	st.ActiveLimit = 100000
	st.BotDefence = false
	st.CapPerOrder = 6
	st.CapPerPerson = 10
	return st
}

// NewSale adds a sale moment to a published sale.
func (e *Env) NewSale(t *testing.T, s *salestest.Sale, st sales.Settings) *sales.Sale {
	t.Helper()
	x, err := e.Sales.Create(context.Background(), s.Org, s.Event, "General sale", "test-user", st)
	if err != nil {
		t.Fatal(err)
	}
	return x
}

// Fan is one browser in the waiting room.
type Fan struct {
	Buyer   checkout.Buyer
	Session string
	Token   string
	Pass    string
	IP      string
}

// NewFan is a fresh signed-in fan with its own session.
func NewFan() *Fan {
	return &Fan{Buyer: salestest.Buyer(), Session: uuid.NewString() + uuid.NewString(), IP: "192.0.2.10"}
}

// Join joins with a solved proof of work.
func (e *Env) Join(t *testing.T, sale *sales.Sale, f *Fan) *queue.Status {
	t.Helper()
	st, err := e.Engine.Join(context.Background(), sale.ID, queue.JoinInput{Sub: f.Buyer.ID, Kind: f.Buyer.Kind, Session: f.Session,
		Pow: queue.SolvePow(sale.ID, queue.SessionHash(f.Session)), UA: "Mozilla/5.0 (Macintosh) Safari/605", IP: f.IP})
	if err != nil {
		t.Fatal(err)
	}
	if st.QueueToken != "" {
		f.Token = st.QueueToken
	}
	if st.Pass != "" {
		f.Pass = st.Pass
	}
	return st
}

// Status polls (moving the clock past the 2-second answer cache first).
func (e *Env) Status(t *testing.T, sale *sales.Sale, f *Fan) *queue.Status {
	t.Helper()
	e.Clock.Add(2100 * time.Millisecond)
	st, err := e.Engine.StatusOf(context.Background(), sale.ID, f.Token, f.Session, "")
	if err != nil {
		t.Fatal(err)
	}
	if st.Pass != "" {
		f.Pass = st.Pass
	}
	return st
}

func mustKeys(t *testing.T, s *sales.Sale) queue.Keys {
	k, err := queue.KeysOf(s.OrganiserID, s.EventID, s.ID)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// Keys are the sale's queue keys.
func (e *Env) Keys(t *testing.T, s *sales.Sale) queue.Keys { return mustKeys(t, s) }

// Admitted joins a fan and lets it in (draw, then admitter ticks), returning it with its pass.
func (e *Env) Admitted(t *testing.T, sale *sales.Sale, f *Fan) *Fan {
	t.Helper()
	e.Join(t, sale, f)
	ctx := context.Background()
	for i := 0; i < 20 && f.Pass == ""; i++ {
		cur, err := e.Sales.Lookup(ctx, sale.ID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := e.Engine.Shuffle(ctx, cur); err != nil {
			t.Fatal(err)
		}
		e.Clock.Add(queue.Tick)
		if _, err := e.Engine.Admit(ctx, cur); err != nil {
			t.Fatal(err)
		}
		e.Status(t, sale, f)
	}
	if f.Pass == "" {
		t.Fatal("not admitted")
	}
	f.Buyer.Pass, f.Buyer.Session = f.Pass, f.Session
	return f
}
