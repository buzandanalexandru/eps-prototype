package sales

import "context"

// StoreForTest writes a copy with a given generation (tests).
func (s *Service) StoreForTest(ctx context.Context, org, event string, list []Sale, gen int64) {
	s.store(ctx, org, event, list, gen)
}
