package checkout

import (
	"context"
	"errors"

	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Usage answers step 3's stock questions (availability and the publish gate) from the inventory:
// units sold plus held per pool instance, and units sold per product.
type Usage struct {
	Svc *Service
	// HeldBy reads units held (not sold), for the shop's honest sold out (step 5); nil: unknown.
	HeldBy HeldReader
}

// HeldReader is an inventory that tells held units from sold ones.
type HeldReader interface {
	Held(ctx context.Context, org, event string, instances []string) (map[string]int64, error)
}

var _ snapshot.HeldUsage = Usage{}

// PoolHeld is the units held by live holds per pool instance.
func (u Usage) PoolHeld(ctx context.Context, org, eventID string) (map[string]int64, error) {
	if u.HeldBy == nil {
		return nil, nil
	}
	live, err := u.Svc.Snaps.Load(ctx, eventID)
	if errors.Is(err, snapshot.ErrNotFound) {
		return map[string]int64{}, nil
	}
	if err != nil {
		return nil, err
	}
	held, err := u.HeldBy.Held(ctx, org, eventID, instancesOf(live))
	if errors.Is(err, ErrNotReady) {
		return map[string]int64{}, nil
	}
	return held, err
}

var _ snapshot.Usage = Usage{}

func (u Usage) PoolUsage(ctx context.Context, org, eventID string) (map[string]int64, error) {
	live, err := u.Svc.Snaps.Load(ctx, eventID)
	if errors.Is(err, snapshot.ErrNotFound) {
		return map[string]int64{}, nil
	}
	if err != nil {
		return nil, err
	}
	insts := instancesOf(live)
	used, err := u.Svc.Inv.Used(ctx, org, eventID, insts)
	if errors.Is(err, ErrNotReady) {
		if err := u.Svc.Rebuild(ctx, org, eventID); err != nil {
			return nil, err
		}
		used, err = u.Svc.Inv.Used(ctx, org, eventID, insts)
	}
	return used, err
}

func (u Usage) ProductSold(ctx context.Context, org, eventID string) (map[string]int64, error) {
	return u.Svc.ProductSold(ctx, org, eventID)
}
