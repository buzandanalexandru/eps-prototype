package snapshot_test

import (
	"context"
	"strings"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

// Scenario "Attempted change": every database role, the migrating owner included, is refused
// an update, a delete or a truncate of a stored snapshot.
func TestSnapshotsAreImmutableForEveryRole(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	w := newWorld(t, pool)
	id := w.publish(t, 1, 53900)
	stmts := []string{
		"update control.sale_snapshots set document_text = document_text where id = '" + id + "'",
		"update control.sale_snapshots set published_at = now() where id = '" + id + "'",
		"delete from control.sale_snapshots where id = '" + id + "'",
		"truncate control.sale_snapshots cascade",
	}
	for _, role := range []string{"", "eps_web", "eps_core"} {
		for _, q := range stmts {
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if role != "" {
				_, _ = tx.Exec(ctx, "set local role "+role)
				_, _ = tx.Exec(ctx, "select set_config('app.organiser_id', $1, true)", w.org)
			}
			_, err = tx.Exec(ctx, q)
			_ = tx.Rollback(ctx)
			if err == nil {
				t.Fatalf("role %q: %s was allowed", role, q)
			}
			if !strings.Contains(err.Error(), "immutable") && !strings.Contains(err.Error(), "permission denied") {
				t.Fatalf("role %q: %s: unexpected error %v", role, q, err)
			}
		}
	}
	var n int
	if err := pool.QueryRow(ctx, "select count(*) from control.sale_snapshots where id = $1", id).Scan(&n); err != nil || n != 1 {
		t.Fatalf("snapshot gone: %d %v", n, err)
	}
	// The stored hash must match the stored bytes.
	_, err := pool.Exec(ctx, `insert into control.sale_snapshots (id, organiser_id, event_id, version, schema_version, document_text, sha256, terms_sha256)
		values ($1, $2, $3, 2, 1, $4, repeat('0', 64), repeat('0', 64))`, id[:35]+"f", w.org, w.event, w.doc(t, id[:35]+"f", 2, 1))
	if err == nil {
		t.Fatal("a snapshot with a wrong hash was stored")
	}
}
