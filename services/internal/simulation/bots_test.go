package simulation_test

import (
	"context"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/simtest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/simulation"
)

// Scenarios "Queue skipper" and "Ground truth reported": bots of every kind try their behaviour
// against the real defences; skippers are refused, the summary shows bots sent, flagged by the
// system, admitted and their tickets, and no real fan is flagged.
func TestBotsAgainstTheDefences(t *testing.T) {
	s := simtest.Start(t)
	ev, real := s.RealSale(t, 2000, 20)
	ctx := context.Background()
	rh, _ := s.Sim.CreateRehearsal(ctx, ev.Org, real.ID, "test")
	if !rh.Settings.BotDefence {
		t.Fatal("the rehearsal should keep the sale's bot defence")
	}
	_, sum := runToEnd(t, s, ev.Org, rh.ID, simulation.Knobs{Fans: 150, BotShare: f(0.3), OpensInSeconds: 30, PollSeconds: 5, DurationSeconds: 60,
		DeclineRate: f(0), AbandonRate: f(0), ProviderA: &simulation.ProviderPlan{Mode: "none"}})
	tr := sum.Truth
	if tr.BotsSent != 45 || tr.FansSent != 105 {
		t.Fatalf("sent: %d bots, %d fans", tr.BotsSent, tr.FansSent)
	}
	skippers, headless := 0, 0
	for j := 0; j < tr.BotsSent; j++ {
		switch simulation.BotKind(j) {
		case simulation.BotSkipper:
			skippers++
		case simulation.BotHeadless:
			headless++
		}
	}
	if tr.SkipsRefused != 2*skippers {
		t.Fatalf("skips refused %d, want %d (no pass and a forged pass, per skipper)", tr.SkipsRefused, 2*skippers)
	}
	// Headless bots (no proof of work, an automation agent, a hosting address) are suspects.
	if sum.BotsFlagged < headless {
		t.Fatalf("bots flagged %d, want at least the %d headless bots", sum.BotsFlagged, headless)
	}
	if sum.FansFlagged != 0 {
		t.Fatalf("%d real fans flagged", sum.FansFlagged)
	}
	if int64(sum.BotsFlagged) > sum.PlacesFlagged || tr.BotsAdmitted > tr.BotsSent || tr.BotTickets < 0 {
		t.Fatalf("summary inconsistent: %+v", sum)
	}
	if tr.CapsRefused < 1 {
		t.Fatalf("cap breakers were not refused: %d", tr.CapsRefused)
	}
}
