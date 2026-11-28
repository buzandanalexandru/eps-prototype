package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/httpx"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
)

// WebhookMaxAge is the oldest signature accepted (D-5).
const WebhookMaxAge = 5 * time.Minute

var (
	webhooksOK       = metrics.NewCounter("eps_webhooks_total", "Provider webhooks applied.")
	webhooksRejected = metrics.NewCounter("eps_webhooks_rejected_total", "Provider webhooks refused (signature, age or body).")
)

// ErrBadSignature means the EPS-Signature header is missing, wrong or too old.
var ErrBadSignature = errors.New("payments: webhook signature not valid")

// VerifySignature checks `EPS-Signature: t=<unix>,v1=<hex HMAC-SHA256(secret, t + "." + body)>`.
func VerifySignature(secret []byte, header string, body []byte, now time.Time) error {
	if len(secret) == 0 {
		return ErrBadSignature
	}
	var ts, sig string
	for _, part := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts = v
		case "v1":
			sig = v
		}
	}
	unix, err := strconv.ParseInt(ts, 10, 64)
	if err != nil || sig == "" {
		return ErrBadSignature
	}
	at := time.Unix(unix, 0)
	if now.Sub(at) > WebhookMaxAge || at.Sub(now) > time.Minute {
		return ErrBadSignature
	}
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(ts + "." + string(body)))
	want := hex.EncodeToString(m.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return ErrBadSignature
	}
	return nil
}

// Webhooks receives provider results on POST /v1/webhooks/psp/{provider} (outside the public
// contract: only the providers call it, and only with a valid signature).
type Webhooks struct {
	Svc     *Service
	Secrets map[string][]byte
	Log     *slog.Logger
	Now     func() time.Time
}

type webhookBody struct {
	ID     string `json:"id"`
	Type   string `json:"type"`
	Charge Charge `json:"charge"`
}

// Handle is the HTTP handler.
func (h *Webhooks) Handle(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	secret, ok := h.Secrets[provider]
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	now := time.Now()
	if h.Now != nil {
		now = h.Now()
	}
	if !ok || err != nil || VerifySignature(secret, r.Header.Get("EPS-Signature"), body, now) != nil {
		webhooksRejected.Inc()
		httpx.WriteProblem(w, r, http.StatusUnauthorized, "invalid_signature", "")
		return
	}
	var ev webhookBody
	if err := json.Unmarshal(body, &ev); err != nil || ev.ID == "" || len(ev.ID) > 100 {
		webhooksRejected.Inc()
		httpx.WriteProblem(w, r, http.StatusBadRequest, "invalid_request", "")
		return
	}
	if err := h.Apply(r.Context(), provider, ev); err != nil {
		if h.Log != nil {
			h.Log.Warn("webhook not applied", "provider", provider, "event", ev.ID, "error", err.Error())
		}
		httpx.WriteProblem(w, r, http.StatusInternalServerError, "internal", "") // the provider retries
		return
	}
	webhooksOK.Inc()
	w.WriteHeader(http.StatusNoContent)
}

// Apply applies a verified webhook once: a repeated event id has no further effect.
func (h *Webhooks) Apply(ctx context.Context, provider string, ev webhookBody) error {
	org := ev.Charge.Metadata["organiser_id"]
	attemptID := ev.Charge.Metadata["attempt_id"]
	if _, err := uuid.Parse(org); err != nil {
		return errors.New("webhook without an organiser")
	}
	if _, err := uuid.Parse(attemptID); err != nil {
		return errors.New("webhook without an attempt")
	}
	var dup bool
	err := db.WithTx(ctx, h.Svc.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		var n int
		if err := tx.QueryRow(ctx, "select count(*) from sales.webhook_events where provider = $1 and event_id = $2", provider, ev.ID).Scan(&n); err != nil {
			return err
		}
		dup = n > 0
		return nil
	})
	if err != nil || dup {
		return err
	}
	a, err := h.Svc.GetAttempt(ctx, org, attemptID)
	if err != nil {
		return err
	}
	if a.Provider != provider || a.ProviderKey != ev.Charge.IdempotencyKey {
		return errors.New("webhook for another provider or charge")
	}
	if err := h.Svc.Apply(ctx, org, attemptID, ev.Charge); err != nil {
		return err
	}
	return db.WithTx(ctx, h.Svc.Pool, db.RoleCore, org, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "insert into sales.webhook_events (organiser_id, provider, event_id) values ($1, $2, $3) on conflict do nothing", org, provider, ev.ID)
		return err
	})
}
