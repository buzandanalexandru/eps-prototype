package checkout

import (
	"context"
	"errors"

	"github.com/buzandanalexandru/eps-prototype/services/internal/snapshot"
)

// Usage answers step 3's stock questions (availability and the publish gate) from the inventory:
// units sold plus held per pool instance, and units sold per product.
type Usage struct{ Svc *Service }

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
