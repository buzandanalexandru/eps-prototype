package botdefence_test

import (
	"context"
	"errors"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/apperr"
	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/sales"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

func problem(err error) (string, map[string]any) {
	var p *apperr.Problem
	if errors.As(err, &p) {
		return p.Code, p.Data
	}
	if err != nil {
		return err.Error(), nil
	}
	return "", nil
}

func ga(n int) checkout.Item { return checkout.Item{ProductID: salestest.ProductGA, Qty: n} }

type salesSettings = sales.Settings

func keysOf(env *onsaletest.Env, o *checkout.Order) (queue.Keys, error) {
	return queue.KeysOf(o.OrganiserID, o.EventID, *o.SaleID)
}

func used(t *testing.T, env *onsaletest.Env, o *checkout.Order) int64 {
	t.Helper()
	u, err := env.Svc.Inv.Used(context.Background(), o.OrganiserID, o.EventID, []string{salestest.PoolArena})
	if err != nil {
		t.Fatal(err)
	}
	return u[salestest.PoolArena]
}
