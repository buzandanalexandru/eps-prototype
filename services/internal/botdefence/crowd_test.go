package botdefence_test

import (
	"context"
	"fmt"
	mrand "math/rand/v2"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
)

// client is one simulated browser or script in the crowd.
type client struct {
	bot   bool
	kind  string
	sub   string
	pow   bool
	ua    string
	ip    string
	print string
	polls []time.Duration // gaps between status polls
	tab   string
	qid   string
}

func jitter(r *mrand.Rand, base, spread time.Duration) time.Duration {
	return base - spread + time.Duration(r.Int64N(int64(2*spread)+1))
}

// crowd builds 10,000 human-like fans and 1,000 scripted bots.
func crowd(r *mrand.Rand) []*client {
	uas := []string{
		"Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 Mobile/15E148 Safari/604.1",
		"Mozilla/5.0 (Linux; Android 14; Pixel 8) AppleWebKit/537.36 Chrome/123.0 Mobile Safari/537.36",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/123.0 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 14_4) AppleWebKit/605.1.15 Version/17.4 Safari/605.1.15",
	}
	var out []*client
	// Fans: residential addresses over 400 /24s, a household shares an address (up to 3 places),
	// polls every 10 s with ±3 s of jitter, and 1 in 10 reloads twice.
	for i := 0; i < 10000; i++ {
		c := &client{kind: "account", sub: uuid.NewString(), pow: true, ua: uas[r.IntN(len(uas))],
			ip: fmt.Sprintf("10.%d.%d.%d", (i/3)%400/200+1, (i/3)%200, (i/3)/400+1), print: fmt.Sprintf("fp-%d", i)}
		if i%4 == 0 {
			c.kind, c.sub = "guest", "guest:"+uuid.NewString()
		}
		for p := 0; p < 12; p++ {
			c.polls = append(c.polls, jitter(r, 10*time.Second, 3*time.Second))
		}
		if i%10 == 0 {
			c.polls = append(c.polls[:5], append([]time.Duration{800 * time.Millisecond, 600 * time.Millisecond}, c.polls[5:]...)...)
		}
		out = append(out, c)
	}
	// Bots, in five families of 190, and 50 careful ones that behave like people.
	for i := 0; i < 1000; i++ {
		c := &client{bot: true, kind: "guest", sub: "guest:" + uuid.NewString(), ua: uas[r.IntN(len(uas))], ip: fmt.Sprintf("192.0.2.%d", 1+i%250)}
		switch {
		case i < 190: // refresh spammer: no proof of work, polls every 200 ms
			for p := 0; p < 35; p++ {
				c.polls = append(c.polls, 200*time.Millisecond)
			}
		case i < 380: // headless farm in a data centre, polls every 5 s exactly
			c.pow, c.ua, c.ip = true, "Mozilla/5.0 (X11; Linux x86_64) HeadlessChrome/123.0", fmt.Sprintf("198.51.100.%d", i%40)
			for p := 0; p < 12; p++ {
				c.polls = append(c.polls, 5*time.Second)
			}
		case i < 570: // proof of work, many places from one address, polls every 1.5 s
			c.pow, c.ip = true, fmt.Sprintf("203.0.113.%d", i%5)
			for p := 0; p < 30; p++ {
				c.polls = append(c.polls, 1500*time.Millisecond)
			}
		case i < 760: // no proof of work, jittered fast polling from homes
			c.ip = fmt.Sprintf("172.16.%d.%d", i%200, i%7+1)
			for p := 0; p < 32; p++ {
				c.polls = append(c.polls, jitter(&*r, time.Second, 400*time.Millisecond))
			}
		case i < 950: // one browser print shared by many accounts, scripted client from a hosting range
			c.pow, c.ua, c.print, c.ip = true, "python-requests/2.31", "fp-farm", fmt.Sprintf("198.51.100.%d", 100+i%150)
			for p := 0; p < 12; p++ {
				c.polls = append(c.polls, 3*time.Second)
			}
		default: // careful bots that look like people (not expected to be caught)
			c.pow, c.ip = true, fmt.Sprintf("172.30.%d.4", i)
			for p := 0; p < 12; p++ {
				c.polls = append(c.polls, jitter(r, 10*time.Second, 3*time.Second))
			}
		}
		out = append(out, c)
	}
	return out
}

// Scenario "Simulated crowd": 10,000 human-like fans and 1,000 bots go through the waiting room;
// fewer than 1% of the fans and more than 90% of the bots are marked suspect.
func TestSimulatedCrowd(t *testing.T) {
	if testing.Short() {
		t.Skip("11,000 clients")
	}
	env := onsaletest.Setup(t)
	ctx := context.Background()
	st := env.Open()
	st.BotDefence = true
	st.OpensAt = env.Clock.Now().Add(20 * time.Minute).Truncate(time.Second)
	s := env.NewSale(t, env.Env.NewSale(t, 100, 100), st)
	k := env.Keys(t, s)
	r := mrand.New(mrand.NewPCG(1, 2))
	clients := crowd(r)
	start := time.Now()
	var wg sync.WaitGroup
	work := make(chan *client)
	errs := make(chan error, 8)
	for w := 0; w < 8; w++ {
		// Each worker has its own clock (each client's timeline is its own).
		clk := &onsaletest.Clock{}
		clk.Set(env.Clock.Now())
		sv := &sales.Service{Pool: env.Pool, Redis: env.Redis, MACKey: []byte("test-key"), Now: clk.Now}
		eng := *env.Engine
		eng.Sales, eng.Now = sv, clk.Now
		wg.Add(1)
		go func() {
			defer wg.Done()
			for c := range work {
				clk.Set(env.Clock.Now())
				session := uuid.NewString() + uuid.NewString()
				pow := ""
				if c.pow {
					pow = queue.SolvePow(s.ID, queue.SessionHash(session))
				}
				in := queue.JoinInput{Sub: c.sub, Kind: c.kind, Session: session, Pow: pow, UA: c.ua, IP: c.ip, Print: c.print}
				st, err := eng.Join(ctx, s.ID, in)
				if err != nil {
					errs <- err
					return
				}
				vc, _ := eng.Verifier().Verify(st.QueueToken, queue.KindQueue, clk.Now(), false)
				c.qid = vc.QID
				for _, gap := range c.polls {
					clk.Add(gap)
					if _, err := eng.StatusOf(ctx, s.ID, st.QueueToken, session, c.tab); err != nil {
						errs <- err
						return
					}
				}
			}
		}()
	}
	for _, c := range clients {
		work <- c
	}
	close(work)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var fans, fanFlagged, bots, botFlagged, fanRisky int
	for _, c := range clients {
		sc, _ := env.Scorer.Score(ctx, k.Bot(c.qid))
		if c.bot {
			bots++
			if sc >= botdefence.SuspectAt {
				botFlagged++
			}
		} else {
			fans++
			if sc >= botdefence.SuspectAt {
				fanFlagged++
			}
			if sc >= botdefence.RiskyAt {
				fanRisky++
			}
		}
	}
	t.Logf("%d clients in %s: %d of %d fans suspect (%d risky), %d of %d bots suspect", len(clients), time.Since(start), fanFlagged, fans, fanRisky, botFlagged, bots)
	if float64(fanFlagged) >= 0.01*float64(fans) {
		t.Fatalf("%d of %d fans marked suspect (1%% or more)", fanFlagged, fans)
	}
	if float64(botFlagged) <= 0.9*float64(bots) {
		t.Fatalf("only %d of %d bots marked suspect (90%% or fewer)", botFlagged, bots)
	}
}
