package checkout_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
)

func links(now *time.Time) *checkout.Links {
	return &checkout.Links{Key: []byte("k"), Now: func() time.Time { return *now }}
}

func TestResumeLinkValidUntilTheDeadline(t *testing.T) {
	now := time.Now()
	l := links(&now)
	o := &checkout.Order{ID: "4a4a4a4a-0000-4000-8000-000000000001", HoldExpiresAt: now.Add(5 * time.Minute)}
	tok := l.ResumeLink(o)
	if _, err := l.Check(tok, o.ID, checkout.LinkResume); err != nil {
		t.Fatalf("fresh link: %v", err)
	}
	// A view token is not a resume token.
	if _, err := l.Check(l.ViewLink(&checkout.Order{ID: o.ID, CreatedAt: now}), o.ID, checkout.LinkResume); !errors.Is(err, checkout.ErrLinkInvalid) {
		t.Fatalf("view token used to resume: %v", err)
	}
}

// Scenario "Expired link": after the hold deadline the link is authentic but over (the page shows
// the order's true state and offers a new order, never payment).
func TestResumeLinkExpired(t *testing.T) {
	now := time.Now()
	l := links(&now)
	o := &checkout.Order{ID: "4a4a4a4a-0000-4000-8000-000000000002", HoldExpiresAt: now.Add(time.Minute)}
	tok := l.ResumeLink(o)
	now = now.Add(2 * time.Minute)
	if _, err := l.Check(tok, o.ID, checkout.LinkResume); !errors.Is(err, checkout.ErrLinkExpired) {
		t.Fatalf("want expired, got %v", err)
	}
}

// Scenario "Tampered link": a changed order id or deadline fails the signature check.
func TestResumeLinkTampered(t *testing.T) {
	now := time.Now()
	l := links(&now)
	o := &checkout.Order{ID: "4a4a4a4a-0000-4000-8000-000000000003", HoldExpiresAt: now.Add(time.Minute)}
	tok := l.ResumeLink(o)
	if _, err := l.Check(tok, "4a4a4a4a-0000-4000-8000-000000000004", checkout.LinkResume); !errors.Is(err, checkout.ErrLinkInvalid) {
		t.Fatalf("other order: %v", err)
	}
	// Re-sign a longer deadline with another key: the core's signature check fails.
	forged := (&checkout.Links{Key: []byte("attacker")}).Sign(o.ID, checkout.LinkResume, now.Add(time.Hour))
	if _, err := l.Check(forged, o.ID, checkout.LinkResume); !errors.Is(err, checkout.ErrLinkInvalid) {
		t.Fatalf("forged deadline: %v", err)
	}
	body, sig, _ := strings.Cut(tok, ".")
	flipped := body[:len(body)-1] + string("AB"[0]^body[len(body)-1]^'B') + "." + sig
	if _, err := l.Check(flipped, o.ID, checkout.LinkResume); !errors.Is(err, checkout.ErrLinkInvalid) {
		t.Fatalf("edited body: %v", err)
	}
}
