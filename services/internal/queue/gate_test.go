package queue_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// Scenario "No pass": a hold without a pass is refused and the fan is sent to the waiting room.
func TestGateNoPass(t *testing.T) {
	env := onsaletest.Setup(t)
	sale := env.Env.NewSale(t, 100, 100)
	s := env.NewSale(t, sale, env.Open())
	_, _, err := env.Svc.CreateOrder(context.Background(), salestest.Buyer(), sale.Request(salestestGA(1)), "")
	if code(err) != "EPS-PASS-REQUIRED" {
		t.Fatalf("want EPS-PASS-REQUIRED, got %v", err)
	}
	if p := problemData(err); p["saleId"] != s.ID {
		t.Fatalf("the answer names sale %v, want %s (the waiting room to send the fan to)", p["saleId"], s.ID)
	}
	if used := sale.Used(t); len(used) != 0 {
		t.Fatalf("stock taken: %v", used)
	}
}

// Scenario "Pass from another session": a valid pass replayed from another session is refused,
// and so is one presented by another account.
func TestGatePassFromAnotherSession(t *testing.T) {
	env := onsaletest.Setup(t)
	sale := env.Env.NewSale(t, 100, 100)
	s := env.NewSale(t, sale, env.Open())
	f := env.Admitted(t, s, onsaletest.NewFan())
	ctx := context.Background()
	other := f.Buyer
	other.Session = "another-session-secret-0000000000"
	if _, _, err := env.Svc.CreateOrder(ctx, other, sale.Request(salestestGA(1)), ""); code(err) != "EPS-PASS-REQUIRED" {
		t.Fatalf("replayed from another session: %v", err)
	}
	thief := salestest.Buyer()
	thief.Pass, thief.Session = f.Pass, f.Session
	if _, _, err := env.Svc.CreateOrder(ctx, thief, sale.Request(salestestGA(1)), ""); code(err) != "EPS-PASS-REQUIRED" {
		t.Fatalf("another account with the pass and its session: %v", err)
	}
	// The pass's own session still works.
	sale.Hold(t, f.Buyer, salestestGA(1))
}

// Scenario "Forged pass": an edited payload or a pass signed with another key is refused.
func TestGateForgedPass(t *testing.T) {
	env := onsaletest.Setup(t)
	sale := env.Env.NewSale(t, 100, 100)
	s := env.NewSale(t, sale, env.Open())
	f := env.Admitted(t, s, onsaletest.NewFan())
	ctx := context.Background()
	parts := strings.Split(f.Pass, ".")
	body, _ := base64.RawURLEncoding.DecodeString(parts[1])
	edited := strings.Replace(string(body), `"exp":`, `"exp":9`, 1)
	forged := f.Buyer
	forged.Pass = parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(edited)) + "." + parts[2]
	if _, _, err := env.Svc.CreateOrder(ctx, forged, sale.Request(salestestGA(1)), ""); code(err) != "EPS-PASS-REQUIRED" {
		t.Fatalf("edited payload: %v", err)
	}
	_, otherKey, _ := ed25519.GenerateKey(rand.Reader)
	c, _ := env.Engine.Verifier().Verify(f.Pass, queue.KindPass, env.Clock.Now(), false)
	forged.Pass = queue.NewSigner(otherKey).Sign(*c)
	if _, _, err := env.Svc.CreateOrder(ctx, forged, sale.Request(salestestGA(1)), ""); code(err) != "EPS-PASS-REQUIRED" {
		t.Fatalf("signed with another key: %v", err)
	}
	// A queue token is not a pass.
	forged.Pass = f.Token
	if _, _, err := env.Svc.CreateOrder(ctx, forged, sale.Request(salestestGA(1)), ""); code(err) != "EPS-PASS-REQUIRED" {
		t.Fatalf("a queue token as the pass: %v", err)
	}
}

// D-11: the payment needs the order's own pass from the same session too.
func TestGatePaymentNeedsThePass(t *testing.T) {
	env := onsaletest.Setup(t)
	sale := env.Env.NewSale(t, 100, 100)
	s := env.NewSale(t, sale, env.Open())
	f := env.Admitted(t, s, onsaletest.NewFan())
	o := sale.Hold(t, f.Buyer, salestestGA(1))
	ctx := context.Background()
	if err := env.Gate.BeforePay(ctx, o, f.Buyer, false); err != nil {
		t.Fatalf("own pass: %v", err)
	}
	noPass := f.Buyer
	noPass.Pass = ""
	if err := env.Gate.BeforePay(ctx, o, noPass, false); code(err) != "EPS-PASS-REQUIRED" {
		t.Fatalf("payment without the pass: %v", err)
	}
	if err := env.Gate.BeforePay(ctx, o, noPass, true); err != nil {
		t.Fatalf("a signed order link pays without the pass: %v", err)
	}
}
