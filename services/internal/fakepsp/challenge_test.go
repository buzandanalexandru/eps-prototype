package fakepsp_test

import (
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
)

func pageHTML(t *testing.T, r *rig, id string) string {
	t.Helper()
	resp, err := http.Get(r.srv.URL + "/a/pay/" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

var codeRe = regexp.MustCompile(`data-testid="psp-bank-code">(\d{6})<`)

func submitCode(t *testing.T, r *rig, id, code string) {
	t.Helper()
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.PostForm(r.srv.URL+"/a/pay/"+id+"/challenge", url.Values{"code": {code}})
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

// Scenario "Challenge passed": the fan enters the code shown and the charge succeeds (webhook).
func TestChallengePassed(t *testing.T) {
	r := newRig(t, true)
	r.knobs(t, "m1", fakepsp.Knobs{ChallengeRate: 1})
	_, ch := r.charge(t, "challenge-01", fakepsp.HostedCard, nil)
	id := ch["charge_id"].(string)
	approveOnPage(t, r, id, "approve")
	html := pageHTML(t, r, id)
	m := codeRe.FindStringSubmatch(html)
	if m == nil {
		t.Fatalf("no simulated bank code on the page:\n%s", html)
	}
	submitCode(t, r, id, m[1])
	_, got := r.do(t, "GET", "/v1/charges/"+id, "api-key", nil)
	if got["status"] != "succeeded" {
		t.Fatalf("status %v", got["status"])
	}
	waitHooks(t, r, 1)
	if !strings.Contains(string(r.hooks.bodies[0]), `"charge.succeeded"`) {
		t.Fatalf("webhook %s", r.hooks.bodies[0])
	}
}

// Scenario "Challenge failed": three wrong codes decline the charge with 3ds_failed (the core maps
// it to EPS-PAY-3DS-FAILED; services/internal/payments covers the order's side).
func TestChallengeFailed(t *testing.T) {
	r := newRig(t, true)
	r.knobs(t, "m1", fakepsp.Knobs{ChallengeRate: 1})
	_, ch := r.charge(t, "challenge-02", fakepsp.HostedCard, nil)
	id := ch["charge_id"].(string)
	approveOnPage(t, r, id, "approve")
	for i := 0; i < 3; i++ {
		submitCode(t, r, id, "000000x"[:6])
	}
	_, got := r.do(t, "GET", "/v1/charges/"+id, "api-key", nil)
	if got["status"] != "declined" || got["decline_reason"] != "3ds_failed" {
		t.Fatalf("after three wrong codes: %v", got)
	}
}

// Scenario "Hosted test outcomes" (page check): the provider page offers the test outcomes and no
// input that could take a card number; Apple Pay and Google Pay open a simulated sheet; RO and EN.
func TestHostedPageHasNoCardField(t *testing.T) {
	r := newRig(t, true)
	r.knobs(t, "m1", fakepsp.Knobs{ChallengeRate: 1})
	_, ch := r.charge(t, "page-0001", fakepsp.HostedCard, map[string]any{"locale": "en"})
	html := pageHTML(t, r, ch["charge_id"].(string))
	for _, want := range []string{"Test card: approve", "Decline: insufficient funds", "Decline: 3-D Secure failed", "Cancel at bank", "RON 552.48", "no card data"} {
		if !strings.Contains(html, want) {
			t.Fatalf("page lacks %q", want)
		}
	}
	if strings.Contains(html, "<input") {
		t.Fatal("the outcome page has an input")
	}
	approveOnPage(t, r, ch["charge_id"].(string), "approve")
	challenge := pageHTML(t, r, ch["charge_id"].(string))
	inputs := regexp.MustCompile(`<input[^>]*>`).FindAllString(challenge, -1)
	if len(inputs) != 1 || !strings.Contains(inputs[0], `maxlength="6"`) || strings.Contains(challenge, "cc-number") {
		t.Fatalf("challenge inputs %v: only a 6-digit code may be typed", inputs)
	}
	_, ro := r.charge(t, "page-0002", fakepsp.ApplePay, map[string]any{"locale": "ro"})
	roHTML := pageHTML(t, r, ro["charge_id"].(string))
	if !strings.Contains(roHTML, "Plătește cu Apple Pay (simulat)") || !strings.Contains(roHTML, "552,48 RON") || strings.Contains(roHTML, "<input") {
		t.Fatalf("Romanian Apple Pay sheet:\n%s", roHTML)
	}
	_, gp := r.charge(t, "page-0003", fakepsp.GooglePay, map[string]any{"locale": "en"})
	if !strings.Contains(pageHTML(t, r, gp["charge_id"].(string)), "Pay with Google Pay (simulated)") {
		t.Fatal("no Google Pay sheet")
	}
}
