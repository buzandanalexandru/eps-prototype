package tenancy_test

import (
	"context"
	"strings"
	"testing"

	"github.com/buzandanalexandru/eps-prototype/services/internal/tenancy"
	"github.com/buzandanalexandru/eps-prototype/services/internal/testdb"
)

// The real schemas pass the organiser rule.
func TestMigratedSchemasFollowTheOrganiserRule(t *testing.T) {
	pool, _ := testdb.Migrated(t)
	vs, err := tenancy.CheckSchema(context.Background(), pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) > 0 {
		t.Fatalf("tables breaking the organiser rule:%s", tenancy.FormatViolations(vs))
	}
}

func expectViolation(t *testing.T, vs []tenancy.Violation, table, reason string) {
	t.Helper()
	for _, v := range vs {
		if v.Table == table && strings.Contains(v.Reason, reason) {
			return
		}
	}
	t.Fatalf("expected %s to be named (%s); got:%s", table, reason, tenancy.FormatViolations(vs))
}

// Scenario "New table without organiser id".
func TestSchemaCheckNamesTableWithoutOrganiserID(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	if _, err := pool.Exec(ctx, "create table sales.test_no_org (id int primary key)"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "create table control.test_nullable_org (id int primary key, organiser_id uuid)"); err != nil {
		t.Fatal(err)
	}
	vs, err := tenancy.CheckSchema(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	expectViolation(t, vs, "sales.test_no_org", "organiser_id")
	expectViolation(t, vs, "control.test_nullable_org", "organiser_id")
}

// Scenario "Table without forced row-level security".
func TestSchemaCheckNamesTableWithoutForcedRLS(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.Migrated(t)
	for _, sql := range []string{
		"create table sales.test_not_forced (id int primary key, organiser_id uuid not null)",
		"alter table sales.test_not_forced enable row level security",
		"create policy p on sales.test_not_forced using (true)",
		"create table sales.test_no_rls (id int primary key, organiser_id uuid not null)",
		"create table sales.test_no_policy (id int primary key, organiser_id uuid not null)",
		"alter table sales.test_no_policy enable row level security",
		"alter table sales.test_no_policy force row level security",
	} {
		if _, err := pool.Exec(ctx, sql); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	vs, err := tenancy.CheckSchema(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	expectViolation(t, vs, "sales.test_not_forced", "not forced")
	expectViolation(t, vs, "sales.test_no_rls", "not enabled")
	expectViolation(t, vs, "sales.test_no_policy", "no row-level security policy")
}
