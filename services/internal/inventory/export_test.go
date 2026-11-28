package inventory

import (
	"context"
	"time"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
)

// StartShard exposes a hold's first shard to the tests.
func StartShard(holdID string, n int) int { return startShard(holdID, n) }

// ReservePartial is a reserve cut short: the record and the units of the named instances only, as
// if the core stopped after its first pool (the expiry scenario).
func (s *Sharded) ReservePartial(ctx context.Context, h checkout.Hold, deadline time.Time, only ...string) error {
	k, _ := s.keysFor(h.OrganiserID, h.EventID)
	if err := masterBegin.Run(ctx, s.Redis, []string{k.hold(h.ID), k.homeExp(homeOf(h.ID))}, deadline.UnixMilli(), h.ID).Err(); err != nil {
		return err
	}
	for _, in := range only {
		n, _ := s.Redis.HGet(ctx, k.meta(part(in)), "n").Int()
		need := h.Draws[in]
		for i := 0; i < n && need > 0; i++ {
			got, err := s.takeShard(ctx, k, i, part(in), h.ID, need, deadline)
			if err != nil {
				return err
			}
			need -= got
		}
	}
	s.index(ctx, h.OrganiserID, h.EventID)
	return nil
}

// HoldKey names a hold's record (tests).
func HoldKey(org, event, holdID string) string { return keys{org, event}.hold(holdID) }
