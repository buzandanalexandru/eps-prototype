package botdefence_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
)

func botSale(t *testing.T, env *onsaletest.Env) *sales.Sale {
	st := env.Open()
	st.BotDefence = true
	return env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
}

// Scenario "Refresh-spamming bot": no proof of work and a status poll every 200 ms reach 70.
func TestRefreshSpammingBot(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	s := botSale(t, env)
	f := onsaletest.NewFan()
	st, err := env.Engine.Join(ctx, s.ID, queue.JoinInput{Sub: f.Buyer.ID, Kind: "account", Session: f.Session, UA: "Mozilla/5.0 Chrome/120", IP: "192.0.2.50"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		env.Clock.Add(200 * time.Millisecond)
		if _, err := env.Engine.StatusOf(ctx, s.ID, st.QueueToken, f.Session, ""); err != nil {
			t.Fatal(err)
		}
	}
	c, _ := env.Engine.Verifier().Verify(st.QueueToken, queue.KindQueue, env.Clock.Now(), false)
	k := env.Keys(t, s)
	score, _ := env.Scorer.Score(ctx, k.Bot(c.QID))
	sig, _ := env.Scorer.Signals(ctx, k.Bot(c.QID))
	if score < botdefence.SuspectAt {
		t.Fatalf("score %d (%v), want 70 or more", score, sig)
	}
	t.Logf("bot score %d: %v", score, sig)
}

// Scenario "Ordinary fan": proof of work, polls every 10 s with jitter, two reloads: below 40.
func TestOrdinaryFan(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	s := botSale(t, env)
	f := onsaletest.NewFan()
	env.Join(t, s, f)
	for i, d := range []int{9100, 11800, 8300, 12700, 10400, 7900, 10900, 9600, 12100, 8800, 1200, 900, 10300, 9900} {
		env.Clock.Add(time.Duration(d) * time.Millisecond)
		if _, err := env.Engine.StatusOf(ctx, s.ID, f.Token, f.Session, ""); err != nil {
			t.Fatalf("poll %d: %v", i, err)
		}
	}
	// Two reloads: the page joins again (the same place) and polls at once.
	env.Join(t, s, f)
	env.Join(t, s, f)
	c, _ := env.Engine.Verifier().Verify(f.Token, queue.KindQueue, env.Clock.Now(), false)
	score, _ := env.Scorer.Score(ctx, env.Keys(t, s).Bot(c.QID))
	if score >= botdefence.RiskyAt {
		t.Fatalf("an ordinary fan scored %d", score)
	}
}

func TestHeadlessAndHostingRanges(t *testing.T) {
	for ua, want := range map[string]bool{
		"Mozilla/5.0 (X11; Linux x86_64) HeadlessChrome/120.0": true,
		"python-requests/2.31":                                 true,
		"":                                                     true,
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_0) Safari/604.1": false,
	} {
		if botdefence.IsHeadless(ua) != want {
			t.Errorf("IsHeadless(%q) != %v", ua, want)
		}
	}
	if !botdefence.InDataCentre("198.51.100.7") || botdefence.InDataCentre("192.0.2.7") || botdefence.Subnet("192.0.2.77") != "192.0.2.0-24" {
		t.Fatal("hosting ranges or subnets")
	}
}
