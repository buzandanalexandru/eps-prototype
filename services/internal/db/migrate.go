package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/pressly/goose/v3"

	_ "github.com/jackc/pgx/v5/stdlib" // database/sql driver "pgx" for goose
)

// Schema names a database schema with its own forward-only migration history.
type Schema string

const (
	Control Schema = "control"
	Sales   Schema = "sales"
)

// VersionTable is the goose version table of a schema, kept inside that schema.
func (s Schema) VersionTable() string { return string(s) + ".goose_db_version" }

// MinControlVersion is the oldest control schema version the Go services work with (they read the
// organiser registry and the auth key table). Raise it when the core starts depending on newer
// control migrations.
const MinControlVersion = 8

// Migrate applies the pending migrations of a schema from fsys (a directory of NNNNN_name.sql files).
// It creates the schema first so the version table can live in it. Down migrations are never run.
func Migrate(ctx context.Context, sqlDB *sql.DB, schema Schema, fsys fs.FS) ([]*goose.MigrationResult, error) {
	if _, err := sqlDB.ExecContext(ctx, "CREATE SCHEMA IF NOT EXISTS "+string(schema)); err != nil {
		return nil, fmt.Errorf("migrate %s: create schema: %w", schema, err)
	}
	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys,
		goose.WithTableName(schema.VersionTable()),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		return nil, fmt.Errorf("migrate %s: %w", schema, err)
	}
	res, err := p.Up(ctx)
	if err != nil {
		return res, fmt.Errorf("migrate %s: %w", schema, err)
	}
	return res, nil
}

// MigrateAll applies control first (it creates the roles and schemas), then sales.
func MigrateAll(ctx context.Context, sqlDB *sql.DB, control, sales fs.FS) error {
	if _, err := Migrate(ctx, sqlDB, Control, control); err != nil {
		return err
	}
	_, err := Migrate(ctx, sqlDB, Sales, sales)
	return err
}

var migrationName = regexp.MustCompile(`^(\d+)_.+\.sql$`)

// LatestVersion is the highest migration version in fsys.
func LatestVersion(fsys fs.FS) (int64, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return 0, err
	}
	var versions []int64
	for _, e := range entries {
		m := migrationName.FindStringSubmatch(e.Name())
		if m == nil {
			continue
		}
		v, _ := strconv.ParseInt(m[1], 10, 64)
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		return 0, fmt.Errorf("no migrations found")
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	return versions[len(versions)-1], nil
}

// RowQuerier is satisfied by pools, connections and transactions.
type RowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// SchemaVersion reads the applied version of a schema (0 when its version table does not exist).
func SchemaVersion(ctx context.Context, q RowQuerier, schema Schema) (int64, error) {
	var reg *string
	if err := q.QueryRow(ctx, "select to_regclass($1)::text", schema.VersionTable()).Scan(&reg); err != nil {
		return 0, err
	}
	if reg == nil {
		return 0, nil
	}
	var v int64
	err := q.QueryRow(ctx, "select coalesce(max(version_id), 0) from "+schema.VersionTable()+" where is_applied").Scan(&v)
	return v, err
}
