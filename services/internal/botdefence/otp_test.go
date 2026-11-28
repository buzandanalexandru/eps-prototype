package botdefence_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/botdefence"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
)

func otpSale(t *testing.T, env *onsaletest.Env, on bool) (*onsaletest.Env, func(n int) (*checkout.Order, *onsaletest.Fan)) {
	sale := env.Env.NewSale(t, 100, 100)
	st := env.Open()
	st.OTPOn, st.OTPThreshold, st.BotDefence = on, 4, true
	st.CapPerOrder, st.CapPerPerson = 8, 10
	s := env.NewSale(t, sale, st)
	return env, func(n int) (*checkout.Order, *onsaletest.Fan) {
		f := env.Admitted(t, s, onsaletest.NewFan())
		return sale.Hold(t, f.Buyer, ga(n)), f
	}
}

// Ordinary and bulk buyers (Go side of otp.spec.ts): 2 tickets skip the code; 6 with a threshold of
// 4 must enter it before payment.
func TestOTPRule(t *testing.T) {
	env, buy := otpSale(t, onsaletest.Setup(t), true)
	ctx := context.Background()
	small, f := buy(2)
	if small.OTPRequired || env.Gate.BeforePay(ctx, small, f.Buyer, false) != nil {
		t.Fatalf("an ordinary order asked for a code: %+v", small)
	}
	bulk, g := buy(6)
	if !bulk.OTPRequired {
		t.Fatal("a bulk order was not asked for a code")
	}
	if c, _ := problem(env.Gate.BeforePay(ctx, bulk, g.Buyer, false)); c != apperr.OTPRequired {
		t.Fatalf("payment without the code: %s", c)
	}
	ch, err := env.Gate.SendOTP(ctx, bulk)
	if err != nil || len(ch.Code) != 6 {
		t.Fatalf("send: %+v %v", ch, err)
	}
	if err := env.Gate.VerifyOTP(ctx, bulk, ch.Code); err != nil {
		t.Fatal(err)
	}
	cur, _ := env.Svc.GetIn(ctx, bulk.OrganiserID, bulk.ID)
	if cur.OTPVerified == nil || env.Gate.BeforePay(ctx, cur, g.Buyer, false) != nil {
		t.Fatalf("verified order still blocked: %+v", cur)
	}
}

// Scenario "OTP switched off": no buyer is asked, whatever the score or quantity.
func TestOTPSwitchedOff(t *testing.T) {
	env, buy := otpSale(t, onsaletest.Setup(t), false)
	ctx := context.Background()
	o, f := buy(6)
	if o.OTPRequired || env.Gate.BeforePay(ctx, o, f.Buyer, false) != nil {
		t.Fatalf("a code was asked with OTP off: %+v", o)
	}
	if _, err := env.Gate.SendOTP(ctx, o); err == nil {
		t.Fatal("a code was sent with OTP off")
	}
	// Switching OTP off after a hold lifts the code too.
	env2, buy2 := otpSale(t, onsaletest.Setup(t), true)
	o2, f2 := buy2(6)
	list, _ := env2.Sales.List(ctx, o2.OrganiserID, o2.EventID)
	st := list[0].Settings
	st.OTPOn = false
	if _, err := env2.Sales.Update(ctx, o2.OrganiserID, list[0].ID, "u1", st); err != nil {
		t.Fatal(err)
	}
	env2.Clock.Add(1100 * time.Millisecond)
	if err := env2.Gate.BeforePay(ctx, o2, f2.Buyer, false); err != nil {
		t.Fatalf("OTP off but still asked: %v", err)
	}
}

// Scenario "Wrong code three times": with every resend used, the order is cancelled, its stock is
// free again and the buyer's score rises.
func TestOTPExhausted(t *testing.T) {
	env, buy := otpSale(t, onsaletest.Setup(t), true)
	ctx := context.Background()
	o, _ := buy(6)
	for send := 0; send < 1+botdefence.MaxResends; send++ {
		if _, err := env.Gate.SendOTP(ctx, o); err != nil {
			t.Fatalf("send %d: %v", send, err)
		}
		for try := 0; try < botdefence.MaxTries; try++ {
			err := env.Gate.VerifyOTP(ctx, o, "000000")
			c, d := problem(err)
			last := send == botdefence.MaxResends && try == botdefence.MaxTries-1
			if last && c != apperr.OTPLocked || !last && c != apperr.OTPWrong {
				t.Fatalf("send %d try %d: %s %v", send, try, c, d)
			}
		}
	}
	if _, err := env.Gate.SendOTP(ctx, o); err == nil {
		t.Fatal("a fifth code was sent")
	}
	cur, _ := env.Svc.GetIn(ctx, o.OrganiserID, o.ID)
	if cur.State != checkout.Cancelled {
		t.Fatalf("order %s, want cancelled", cur.State)
	}
	if used := cur.Draws; len(used) == 0 {
		t.Fatal("no draws")
	}
	k, _ := keysOf(env, o)
	if sc, _ := env.Scorer.Score(ctx, k.Bot(*o.PassQID)); sc < botdefence.OTPExhausted.Points {
		t.Fatalf("score %d did not rise", sc)
	}
	if u := used(t, env, o); u != 0 {
		t.Fatalf("stock still taken: %d", u)
	}
}

// Scenario "Expired code": a correct code after 5 minutes is refused and a resend is offered.
func TestOTPExpired(t *testing.T) {
	env, buy := otpSale(t, onsaletest.Setup(t), true)
	ctx := context.Background()
	o, _ := buy(5)
	ch, err := env.Gate.SendOTP(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	env.Clock.Add(botdefence.CodeLife + time.Second)
	c, d := problem(env.Gate.VerifyOTP(ctx, o, ch.Code))
	if c != apperr.OTPExpired || d["resendsLeft"] != botdefence.MaxResends {
		t.Fatalf("got %s %v", c, d)
	}
	ch2, err := env.Gate.SendOTP(ctx, o)
	if err != nil {
		t.Fatal(err)
	}
	if err := env.Gate.VerifyOTP(ctx, o, ch2.Code); err != nil {
		t.Fatalf("the new code: %v", err)
	}
}
