package tickets_test

import (
	"context"
	"strings"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
)

// Scenarios "Guest opens the signed link" and "Altered link": the link opens only its own order's
// tickets with codes; a changed order or signature is "Link not valid"; a claimed order moved.
func TestSignedGuestLink(t *testing.T) {
	x := setup(t)
	ctx := context.Background()
	g := salestest.Guest()
	o, list := x.issued(t, g, ga(1))
	o, _ = x.Svc.GetIn(ctx, o.OrganiserID, o.ID)
	tok := x.svc.LinkFor(ctx, o)
	if tok == "" {
		t.Fatal("no link for an issued guest order")
	}
	ev, err := x.svc.LinkTrip(ctx, tok)
	if err != nil || len(ev.Items) != 1 || ev.Items[0].ID != list[0].ID {
		t.Fatalf("link trip: %+v %v", ev, err)
	}
	if c, err := x.svc.LinkCodes(ctx, tok, list[0].ID); err != nil || len(c.Codes) != 40 {
		t.Fatalf("codes through the link: %v", err)
	}
	// Another order's ticket through this link.
	_, other := x.issued(t, salestest.Guest(), ga(1))
	if _, err := x.svc.LinkCodes(ctx, tok, other[0].ID); code(err) != "EPS-TICKET-NOT-FOUND" {
		t.Fatalf("another order's ticket: %v", err)
	}
	// Altered: the signature, or the order inside it.
	enc, sig, _ := strings.Cut(tok, ".")
	for _, bad := range []string{enc + "." + sig[:len(sig)-2] + "AA", strings.Replace(enc, enc[2:6], "AAAA", 1) + "." + sig, "garbage"} {
		if _, err := x.svc.LinkTrip(ctx, bad); code(err) != "EPS-LINK-NOT-VALID" {
			t.Fatalf("altered link %q: %v", bad, err)
		}
	}
	// No link for an account's order.
	ao, _ := x.issued(t, salestest.Buyer(), ga(1))
	ao, _ = x.Svc.GetIn(ctx, ao.OrganiserID, ao.ID)
	if x.svc.LinkFor(ctx, ao) != "" || x.svc.LinkFor(ctx, &checkout.Order{BuyerKind: "guest", State: checkout.Paid}) != "" {
		t.Fatal("a link for an account's or an unissued order")
	}
}
