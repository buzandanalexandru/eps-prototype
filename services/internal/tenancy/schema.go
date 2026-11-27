package tenancy

import (
	"context"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
)

// Violation is a table that breaks the organiser rule.
type Violation struct {
	Table  string
	Reason string
}

func (v Violation) String() string { return v.Table + ": " + v.Reason }

// Queryer is satisfied by pools, connections and transactions.
type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// CheckSchema lists every table in the control and sales schemas that is not on the platform
// allowlist and lacks organiser_id NOT NULL, row-level security enabled, row-level security
// forced, or a policy.
func CheckSchema(ctx context.Context, q Queryer) ([]Violation, error) {
	rows, err := q.Query(ctx, `
		select n.nspname || '.' || c.relname,
		       c.relrowsecurity, c.relforcerowsecurity,
		       exists (select 1 from pg_attribute a
		               where a.attrelid = c.oid and a.attname = 'organiser_id'
		                 and a.attnotnull and not a.attisdropped),
		       exists (select 1 from pg_policy p where p.polrelid = c.oid)
		from pg_class c join pg_namespace n on n.oid = c.relnamespace
		where n.nspname in ('control', 'sales') and c.relkind in ('r', 'p')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Violation
	for rows.Next() {
		var name string
		var rls, force, hasOrg, hasPolicy bool
		if err := rows.Scan(&name, &rls, &force, &hasOrg, &hasPolicy); err != nil {
			return nil, err
		}
		if _, ok := PlatformTables[name]; ok {
			continue
		}
		switch {
		case !hasOrg:
			out = append(out, Violation{name, "no organiser_id NOT NULL column"})
		case !rls:
			out = append(out, Violation{name, "row-level security not enabled"})
		case !force:
			out = append(out, Violation{name, "row-level security enabled but not forced"})
		case !hasPolicy:
			out = append(out, Violation{name, "no row-level security policy"})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Table < out[j].Table })
	return out, rows.Err()
}

// FormatViolations renders violations for a test failure.
func FormatViolations(vs []Violation) string {
	s := ""
	for _, v := range vs {
		s += fmt.Sprintf("\n  - %s", v)
	}
	return s
}
