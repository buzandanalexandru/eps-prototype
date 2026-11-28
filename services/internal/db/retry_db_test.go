package db_test

import (
	"context"
	"database/sql"
	"io/fs"
	"os"
	"testing"
	"testing/fstest"
	"time"

	dbfiles "github.com/buzandanalexandru/eps-prototype/services/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/db"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

// The step 7 migration against a live system: a writer holds a lock on sales.orders past the
// migration's 5 s lock timeout; the first try fails with SQLSTATE 55P03, rolls back, and the
// retry applies it once the writer is done.
func TestMigrationRetriesOnLockTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out a lock timeout")
	}
	ctx := context.Background()
	dsn := testdb.NewDatabase(t)
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer sqlDB.Close()
	// Everything before step 7 first.
	upTo4 := fstest.MapFS{}
	entries, _ := fs.ReadDir(dbfiles.Sales(), ".")
	for _, e := range entries {
		if e.Name() < "00005" {
			b, _ := fs.ReadFile(dbfiles.Sales(), e.Name())
			upTo4[e.Name()] = &fstest.MapFile{Data: b}
		}
	}
	control := os.DirFS(testdb.ControlMigrations(t))
	if err := db.MigrateAll(ctx, sqlDB, control, upTo4); err != nil {
		t.Fatal(err)
	}
	// A live writer holds sales.orders for 7 s.
	writer, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(ctx, "begin; lock table sales.orders in row exclusive mode"); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(7 * time.Second)
		_, _ = writer.ExecContext(context.Background(), "commit")
		_ = writer.Close()
	}()
	var retries int
	start := time.Now()
	err = db.MigrateAllRetry(ctx, sqlDB, control, dbfiles.Sales(), 5, func(int, error, time.Duration) { retries++ })
	if err != nil {
		t.Fatalf("migrate with retry: %v", err)
	}
	if retries < 1 || time.Since(start) < 7*time.Second {
		t.Fatalf("retries %d after %v, want the first try to time out and a retry to apply it", retries, time.Since(start))
	}
	var v int64
	if err := sqlDB.QueryRowContext(ctx, "select max(version_id) from sales.goose_db_version where is_applied").Scan(&v); err != nil || v < 5 {
		t.Fatalf("sales version %d (%v), want 5", v, err)
	}
}
