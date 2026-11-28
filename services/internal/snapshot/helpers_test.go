package snapshot_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

func TestMain(m *testing.M) { testdb.Main(m) }

func uuid() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	h := hex.EncodeToString(b)
	return h[0:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

type world struct {
	pool    *pgxpool.Pool
	org     string
	brand   string
	event   string
	product string
}

// newWorld makes an organiser with one brand, one event and one GA product priced 539.00 in the
// draft catalogue (written as the migrating owner, setup only).
func newWorld(t *testing.T, pool *pgxpool.Pool) world {
	t.Helper()
	ctx := context.Background()
	w := world{pool: pool, org: uuid(), brand: uuid(), event: uuid(), product: uuid()}
	slug := "t" + strings.ReplaceAll(w.org[:8], "-", "")
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{"insert into control.organisers (id, slug, name) values ($1, $2, 'Org')", []any{w.org, slug}},
		{"insert into control.brands (id, organiser_id, slug, name) values ($1, $2, $3, 'Brand')", []any{w.brand, w.org, slug + "-b"}},
		{"insert into control.events (id, organiser_id, brand_id, slug, starts_at, ends_at) values ($1, $2, $3, 'galaxia-2027', '2027-07-15T14:00Z', '2027-07-19T00:00Z')", []any{w.event, w.org, w.brand}},
		{"insert into control.products (id, organiser_id, event_id, type, name) values ($1, $2, $3, 'pass', '{\"ro\":\"GA\",\"en\":\"GA\"}')", []any{w.product, w.org, w.event}},
		{"insert into control.price_steps (organiser_id, product_id, position, mode, price_minor) values ($1, $2, 0, 'by_date', 53900)", []any{w.org, w.product}},
	} {
		if _, err := pool.Exec(ctx, q.sql, q.args...); err != nil {
			t.Fatalf("%s: %v", q.sql, err)
		}
	}
	return w
}

// doc returns the fixture rewritten for this world, version and GA face price.
func (w world) doc(t *testing.T, id string, version int, faceMinor int64) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(fixture(t), &m); err != nil {
		t.Fatal(err)
	}
	m["snapshotId"], m["organiserId"], m["brandId"], m["eventId"], m["version"] = id, w.org, w.brand, w.event, version
	ga := m["products"].([]any)[0].(map[string]any)
	ga["productId"] = w.product
	steps := ga["priceSteps"].([]any)
	fee := (faceMinor*250 + 5000) / 10000
	steps[0].(map[string]any)["faceMinor"] = faceMinor
	steps[0].(map[string]any)["fanFeeMinor"] = fee
	steps[0].(map[string]any)["allInMinor"] = faceMinor + fee
	steps[0].(map[string]any)["organiserNetMinor"] = faceMinor
	camping := m["products"].([]any)[1].(map[string]any)
	camping["requires"] = []any{w.product}
	b, _ := json.Marshal(m)
	return string(b)
}

// publish inserts snapshot `version` (GA at faceMinor) and makes it live, as the publish does.
func (w world) publish(t *testing.T, version int, faceMinor int64) string {
	t.Helper()
	ctx := context.Background()
	id := uuid()
	doc := w.doc(t, id, version, faceMinor)
	sum := sha256.Sum256([]byte(doc))
	if _, err := w.pool.Exec(ctx,
		`insert into control.sale_snapshots (id, organiser_id, event_id, version, schema_version, document_text, sha256, terms_sha256)
		 values ($1, $2, $3, $4, 1, $5, $6, $6)`, id, w.org, w.event, version, doc, hex.EncodeToString(sum[:])); err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
	if _, err := w.pool.Exec(ctx, "update control.events set live_snapshot_version = $2, status = 'published' where id = $1", w.event, version); err != nil {
		t.Fatal(err)
	}
	return id
}

// logBuffer collects log lines for assertions.
type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}
func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}
func newLog() (*slog.Logger, *logBuffer) {
	b := &logBuffer{}
	return slog.New(slog.NewTextHandler(b, nil)), b
}

var _ = fmt.Sprintf
