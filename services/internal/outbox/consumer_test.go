package outbox_test

import (
	"context"
	"testing"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/outbox"
)

// Scenario "Consumer deduplicates": the same event id twice is applied once.
func TestConsumerAppliesAnIDOnce(t *testing.T) {
	_, rdb := setup(t)
	ctx := context.Background()
	n := 0
	c := &outbox.Consumer{Redis: rdb, Topic: outbox.TopicOrders, Group: "issuer", Name: "w1", Handle: func(context.Context, outbox.Envelope) error { n++; return nil }}
	e := outbox.Envelope{ID: "0c6f8f0e-1111-4111-8111-000000000001", Type: "order.paid", OrganiserID: orgA, OccurredAt: time.Now()}
	for i := 0; i < 2; i++ {
		if _, err := c.Apply(ctx, e); err != nil {
			t.Fatal(err)
		}
	}
	if n != 1 {
		t.Fatalf("applied %d times", n)
	}
}

// Scenario "Other organiser": organiser B never sees A's events, through the API read or a
// consumer limited to B.
func TestEventsStayWithinTheirOrganiser(t *testing.T) {
	pool, rdb := setup(t)
	ctx := context.Background()
	writeN(t, ctx, pool, orgA, 2)
	got, err := outbox.List(ctx, pool, orgB, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("organiser B read %d of A's events", len(got))
	}
	mine, _ := outbox.List(ctx, pool, orgA, 100)
	if len(mine) != 2 {
		t.Fatalf("organiser A read %d of its own events", len(mine))
	}
	if _, err := (&outbox.Relay{Pool: pool, Redis: rdb}).RunOnce(ctx); err != nil {
		t.Fatal(err)
	}
	seen := 0
	c := &outbox.Consumer{Redis: rdb, Topic: outbox.TopicOrders, Group: "webhooks-b", Name: "w", Organiser: orgB, Handle: func(context.Context, outbox.Envelope) error { seen++; return nil }}
	if err := c.Ensure(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReadOnce(ctx, 100); err != nil {
		t.Fatal(err)
	}
	if seen != 0 {
		t.Fatalf("B's consumer handled %d of A's events", seen)
	}
}
