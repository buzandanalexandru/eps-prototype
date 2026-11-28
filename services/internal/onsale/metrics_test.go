package onsale_test

import (
	"context"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/checkout"
	"github.com/buzandanalexandru/eps-prototype/services/internal/inventory"
	"github.com/buzandanalexandru/eps-prototype/services/internal/metrics"
	"github.com/buzandanalexandru/eps-prototype/services/internal/onsaletest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/queue"
	"github.com/buzandanalexandru/eps-prototype/services/internal/salestest"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

// Task 6.1: the series step 6 builds on are on /metrics after a small on-sale.
func TestOnSaleMetricsSeries(t *testing.T) {
	env := onsaletest.Setup(t)
	ctx := context.Background()
	sale := env.Env.NewSale(t, 100, 100)
	st := env.Open()
	st.OTPOn, st.OTPThreshold, st.BotDefence = true, 1, true
	s := env.NewSale(t, sale, st)
	f := env.Admitted(t, s, onsaletest.NewFan())
	o := sale.Hold(t, f.Buyer, checkout.Item{ProductID: salestest.ProductGA, Qty: 2})
	if _, err := env.Gate.SendOTP(ctx, o); err != nil {
		t.Fatal(err)
	}
	_ = env.Gate.VerifyOTP(ctx, o, "000000")
	_, _, _ = env.Svc.CreateOrder(ctx, salestest.Buyer(), sale.Request(checkout.Item{ProductID: salestest.ProductGA, Qty: 1}), "")
	(&queue.Admitter{Engine: env.Engine}).RunOnce(ctx)
	rec := &inventory.Reconciler{Inv: env.Svc.Inv.(*inventory.Sharded), Pool: env.Pool}
	_, _ = rec.RunOnce(ctx)
	w := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	body, _ := io.ReadAll(w.Body)
	for _, want := range []string{
		`eps_queue_length{sale="` + s.ID + `"}`,
		`eps_queue_prequeue_size{sale="` + s.ID + `"}`,
		`eps_queue_admitted{sale="` + s.ID + `"}`,
		`eps_queue_active_sessions{sale="` + s.ID + `"}`,
		"eps_queue_joins_total", "eps_queue_admitted_total", "eps_queue_passes_issued_total",
		"eps_bot_suspects_total",
		`eps_otp_total{outcome="sent"} `, `eps_otp_total{outcome="wrong"} `,
		`eps_pass_rejections_total{reason="missing"} `,
		"eps_holds_created_total",
		`eps_inventory_free{organiser="` + sale.Org + `",event="` + sale.Event + `",instance="` + salestest.PoolArena + `"} 98`,
		`eps_inventory_held{organiser="` + sale.Org + `",event="` + sale.Event + `",instance="` + salestest.PoolArena + `"} 2`,
		`eps_inventory_corrections_total{kind="shard_invariant"} `,
		"eps_inventory_reconcile_rounds_total",
	} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q", want)
		}
	}
}
