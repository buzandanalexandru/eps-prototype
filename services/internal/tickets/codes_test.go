package tickets_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/tickets"
)

func code(err error) string {
	var p *apperr.Problem
	if errors.As(err, &p) {
		return p.Code
	}
	if err != nil {
		return err.Error()
	}
	return ""
}

// Scenarios "Another user asks for codes", "Buffer is at most 10 minutes" and "Old holder after a
// reissue".
func TestCodesOnlyForTheHolder(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	b := salestest.Buyer()
	_, list := x.issued(t, b, ga(1))
	tk := list[0]

	c, err := x.svc.CodesFor(ctx, holder(b), tk.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Codes) != tickets.BufferWindows || c.Codes[0].Window != tickets.WindowOf(x.now) || c.Codes[39].Window != tickets.WindowOf(x.now)+39 {
		t.Fatalf("%d codes from window %d, want the current window and the next 39", len(c.Codes), c.Codes[0].Window)
	}
	if span := c.Codes[39].ExpireAt.Sub(c.Codes[0].ValidAt); span > 10*time.Minute {
		t.Fatalf("buffer spans %v, more than 10 minutes", span)
	}
	for i, cd := range c.Codes {
		tok, err := tickets.Verify(cd.Code, pubKeys(x.svc.Keys))
		if err != nil || tok.TicketID != tk.ID || tok.EventID != tk.EventID || tok.Generation != 1 || tok.Window != cd.Window {
			t.Fatalf("code %d: %+v %v", i, tok, err)
		}
	}
	// A stranger: not found, the same as a ticket that does not exist.
	stranger := salestest.Buyer()
	if _, err := x.svc.CodesFor(ctx, holder(stranger), tk.ID); code(err) != tickets.CodeNotFound {
		t.Fatalf("stranger: %v", err)
	}
	if _, err := x.svc.CodesFor(ctx, holder(stranger), "11111111-2222-4333-8444-555555555555"); code(err) != tickets.CodeNotFound {
		t.Fatalf("missing ticket: %v", err)
	}
	// After a reissue: the old holder is told it moved and gets no code; the new holder's codes
	// carry the new generation.
	nb := salestest.Buyer()
	if _, err := x.svc.Reissue(ctx, x.sale.Org, tk.ID, 1, tickets.Change{Reason: "transferred", HolderKind: "account", HolderID: nb.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := x.svc.CodesFor(ctx, holder(b), tk.ID); code(err) != tickets.CodeMoved {
		t.Fatalf("old holder: %v, want %s", err, tickets.CodeMoved)
	}
	c2, err := x.svc.CodesFor(ctx, holder(nb), tk.ID)
	if err != nil || c2.Generation != 2 {
		t.Fatalf("new holder: %+v %v", c2, err)
	}
	tok, _ := tickets.Verify(c2.Codes[0].Code, pubKeys(x.svc.Keys))
	if tok.Generation != 2 {
		t.Fatalf("new holder's code at generation %d", tok.Generation)
	}
}

// Code requests are limited per holder (each signs 40 codes).
func TestCodesRateLimited(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	x.svc.CodesPerMinute = 3
	b := salestest.Buyer()
	_, list := x.issued(t, b, ga(1))
	for i := 0; i < 3; i++ {
		if _, err := x.svc.CodesFor(ctx, holder(b), list[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := x.svc.CodesFor(ctx, holder(b), list[0].ID); code(err) != tickets.CodeRateLimit {
		t.Fatalf("4th request in a minute: %v", err)
	}
}
