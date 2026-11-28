package payments_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/fakepsp"
	"github.com/buzandanalexandru/eps-prototype/services/internal/payments"
)

func post(t *testing.T, url, sig string, body []byte) int {
	t.Helper()
	req, _ := http.NewRequest("POST", url, bytes.NewReader(body))
	req.Header.Set("EPS-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func successBody(r *rig, attemptID, chargeID string, amount int64) []byte {
	b, _ := json.Marshal(map[string]any{"id": "evt_test_" + attemptID[:8], "type": "charge.succeeded", "charge": map[string]any{
		"charge_id": chargeID, "provider": "a", "merchant": r.sale.Org, "amount_minor": amount, "currency": "RON", "idempotency_key": attemptID,
		"method": "hosted_card", "status": "succeeded", "metadata": map[string]string{"organiser_id": r.sale.Org, "attempt_id": attemptID}}})
	return b
}

// Scenario "Forged webhook": a wrong signature is refused with 401 and nothing changes.
func TestForgedWebhook(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	res, err := r.press(t, b, o, payments.HostedCard, "press-forged-1")
	if err != nil {
		t.Fatal(err)
	}
	body := successBody(r, res.Attempt.ID, "ch_forged", o.TotalMinor)
	url := r.core.URL + "/v1/webhooks/psp/a"
	if st := post(t, url, fakepsp.Sign([]byte("not-the-secret"), time.Now(), body), body); st != 401 {
		t.Fatalf("wrong secret: %d", st)
	}
	if st := post(t, url, fakepsp.Sign([]byte("whsec-a"), time.Now().Add(-6*time.Minute), body), body); st != 401 {
		t.Fatalf("older than 5 minutes: %d", st)
	}
	if st := post(t, url, "", body); st != 401 {
		t.Fatalf("no signature: %d", st)
	}
	if st := post(t, r.core.URL+"/v1/webhooks/psp/b", fakepsp.Sign([]byte("whsec-a"), time.Now(), body), body); st != 401 {
		t.Fatalf("A's secret on B's endpoint: %d", st)
	}
	if got := r.get(t, o.ID); got.State != checkout.Paying {
		t.Fatalf("order %s", got.State)
	}
	if as := r.attempts(t, o.ID); as[0].State != payments.Submitted {
		t.Fatalf("attempt %s", as[0].State)
	}
}

// Scenario "Duplicate webhook": three deliveries pay the order once with one event.
func TestDuplicateWebhook(t *testing.T) {
	r := newRig(t)
	b, o := r.order(t)
	r.knobs(t, "a", fakepsp.Knobs{WebhookDropRate: 1}) // the provider's own webhook is dropped
	res, err := r.press(t, b, o, payments.HostedCard, "press-dup-1")
	if err != nil {
		t.Fatal(err)
	}
	r.pagePost(t, res.ActionURL, "approve")
	as, _ := r.svc.GetAttempt(t.Context(), r.sale.Org, res.Attempt.ID)
	body := successBody(r, res.Attempt.ID, *as.ChargeID, o.TotalMinor)
	for i := 0; i < 3; i++ {
		if st := post(t, r.core.URL+"/v1/webhooks/psp/a", fakepsp.Sign([]byte("whsec-a"), time.Now(), body), body); st != 204 {
			t.Fatalf("delivery %d: %d", i+1, st)
		}
	}
	r.waitState(t, o.ID, checkout.Paid)
	if n := r.events(t, "order.paid"); n != 1 {
		t.Fatalf("%d order.paid events, want 1", n)
	}
}
