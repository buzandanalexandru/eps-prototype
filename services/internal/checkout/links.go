package checkout

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

// Link purposes: a resume link reopens the payment step until the hold deadline (D-8); a view
// link opens a guest's order status page for 30 days (D-9).
const (
	LinkResume = "resume"
	LinkView   = "view"
)

// ViewLinkLife is how long a guest's order link works.
const ViewLinkLife = 30 * 24 * time.Hour

// Links signs and checks order links with a key the core holds (ORDER_LINK_KEY).
type Links struct {
	Key []byte
	Now func() time.Time
}

// ErrLinkInvalid means the token was changed, is for another order or purpose, or is malformed.
var ErrLinkInvalid = errors.New("checkout: link not valid")

// ErrLinkExpired means the token is authentic but its time is over.
var ErrLinkExpired = errors.New("checkout: link expired")

func (l *Links) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func (l *Links) mac(body string) string {
	m := hmac.New(sha256.New, append([]byte("eps-order-link:"), l.Key...))
	m.Write([]byte(body))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// Sign makes a token for an order and purpose valid until `until`.
func (l *Links) Sign(orderID, purpose string, until time.Time) string {
	nonce := make([]byte, 6)
	_, _ = rand.Read(nonce)
	body := orderID + "|" + purpose + "|" + strconv.FormatInt(until.Unix(), 10) + "|" + hex.EncodeToString(nonce)
	enc := base64.RawURLEncoding.EncodeToString([]byte(body))
	return enc + "." + l.mac(enc)
}

// Check verifies a token for an order and purpose and returns its expiry.
func (l *Links) Check(token, orderID, purpose string) (time.Time, error) {
	enc, sig, ok := strings.Cut(token, ".")
	if !ok || len(token) > 400 {
		return time.Time{}, ErrLinkInvalid
	}
	if !hmac.Equal([]byte(sig), []byte(l.mac(enc))) {
		return time.Time{}, ErrLinkInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(enc)
	if err != nil {
		return time.Time{}, ErrLinkInvalid
	}
	parts := strings.Split(string(raw), "|")
	if len(parts) != 4 || parts[0] != orderID || parts[1] != purpose {
		return time.Time{}, ErrLinkInvalid
	}
	exp, err := strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return time.Time{}, ErrLinkInvalid
	}
	until := time.Unix(exp, 0)
	if !l.now().Before(until) {
		return until, ErrLinkExpired
	}
	return until, nil
}

// ResumeLink is the signed resume token of an order (valid until its hold deadline).
func (l *Links) ResumeLink(o *Order) string { return l.Sign(o.ID, LinkResume, o.HoldExpiresAt) }

// ViewLink is the signed order-status token of an order (30 days).
func (l *Links) ViewLink(o *Order) string {
	return l.Sign(o.ID, LinkView, o.CreatedAt.Add(ViewLinkLife))
}
