// Package tenancy keeps organisers apart (I1, U2): the schema rule every table must follow, and the
// API scoping that takes the organiser only from the caller's token.
package tenancy

// PlatformTables are the reviewed tables that hold no organiser data and therefore have no
// organiser_id or row-level security. Adding a table here needs a review: everything else in the
// control and sales schemas must carry organiser_id NOT NULL with RLS enabled and forced.
var PlatformTables = map[string]string{
	"control.user":             "Better Auth users (people, not organiser data)",
	"control.session":          "Better Auth sessions",
	"control.account":          "Better Auth linked sign-in methods",
	"control.verification":     "Better Auth one-time tokens (magic links)",
	"control.jwks":             "core-token signing keys (the core reads only the public columns)",
	"control.rateLimit":        "Better Auth rate-limit counters",
	"control.organisers":       "the organiser registry itself",
	"control.demo_mailbox":     "demo-domain magic links shown on screen (no real addresses)",
	"control.platform_fee":     "the published platform fee (one public value per currency, written only by migrations)",
	"control.map_tiles":        "OpenStreetMap tiles for storefront map sections, keyed by position (no organiser data)",
	"control.goose_db_version": "migration history",
	"sales.goose_db_version":   "migration history",
}
