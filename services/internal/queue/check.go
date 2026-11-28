package queue

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
)

// Why a pass was refused.
const (
	RejectMissing = "missing"
	RejectForged  = "forged"
	RejectExpired = "expired"
	RejectSale    = "other_sale"
	RejectSession = "other_session"
	RejectState   = "ended"
)

var rejections = map[string]*metrics.Counter{}

func init() {
	for _, r := range []string{RejectMissing, RejectForged, RejectExpired, RejectSale, RejectSession, RejectState} {
		rejections[r] = metrics.NewCounter("eps_pass_rejections_total", "Checkout requests refused for their pass, by reason.", "reason", r)
	}
}

// Rejected is a refused pass (the claims are set when the signature was good).
type Rejected struct {
	Reason string
	Claims *Claims
}

func (r *Rejected) Error() string { return "queue: pass refused (" + r.Reason + ")" }

// PassCheck is the core's side of the gate (D-4): signature (public key only), expiry, organiser,
// event and sale against the request, the session, and the pass's state in Redis.
type PassCheck struct {
	Verifier *Verifier
	Redis    *redis.Client
}

// Check verifies a pass for a request of sub with session secret on (org, event, sale). With
// ignoreExpiry a pass that ended by time still serves the order it already holds.
func (p *PassCheck) Check(ctx context.Context, tok, session, sub, org, event, sale string, now time.Time, ignoreExpiry bool) (*Claims, error) {
	reject := func(reason string, c *Claims) error {
		rejections[reason].Inc()
		return &Rejected{Reason: reason, Claims: c}
	}
	if tok == "" {
		return nil, reject(RejectMissing, nil)
	}
	c, err := p.Verifier.Verify(tok, KindPass, now, ignoreExpiry)
	switch {
	case errors.Is(err, ErrExpired):
		return nil, reject(RejectExpired, c)
	case err != nil:
		return nil, reject(RejectForged, nil)
	}
	if c.Org != org || c.Evt != event || c.Sale != sale {
		return nil, reject(RejectSale, c)
	}
	if session == "" || c.Sid != SessionHash(session) || c.Sub != sub {
		return nil, reject(RejectSession, c)
	}
	k, err := KeysOf(org, event, sale)
	if err != nil {
		return nil, reject(RejectSale, c)
	}
	v, err := p.Redis.HMGet(ctx, k.Pass(c.QID), "state", "tok").Result()
	if err != nil {
		return nil, err
	}
	st, _ := v[0].(string)
	stored, _ := v[1].(string)
	if stored != tok || st != "active" {
		return nil, reject(RejectState, c)
	}
	return c, nil
}
