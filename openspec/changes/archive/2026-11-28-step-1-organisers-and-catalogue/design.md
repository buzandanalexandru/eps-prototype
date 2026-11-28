# Design

## Context

Step 0 delivered the control and sales schemas, row-level security keyed on `organiser_id`, the transaction helper (`SET LOCAL ROLE eps_web` + `app.organiser_id`), Better Auth with core tokens carrying `org` and `role`, `control.organisers` and `control.memberships`, and the demo organiser path. This step adds the organiser's own data. The catalogue lives only in the control schema; the Go core never reads it (P1): step 3 compiles it into sale snapshots. Teardown facts used by the seed are in `docs/research/nibiru-eps-teardown.md` §1, §3, §5 and §10.

## Goals / Non-Goals

**Goals:**
- A catalogue model general enough that NIBIRU's resort is data, not code.
- One money and pricing module (fee policy, all-in price, ladder evaluation, instalment schedule) used by Studio now and by the snapshot compiler in step 3, so prices can never disagree between screens.
- Isolation of every new table from the first migration.

**Non-Goals:**
- Storefront design and the draft shop itself (step 2), publishing and snapshots (step 3), enforcing per-account caps (step 5), selling anything (step 4).
- Payment accounts (A5, step 10), promo codes (C6, step 10), upgrades (C7, step 9), seat maps (J2, out of scope).
- Real email for invitations: an invitation is a pending row (token stored hashed) and a copyable link `/studio/invite/<organiser>/<token>`, shown once.

## Decisions

### D1.1 Studio structure
- Studio is under `/studio`; the active organiser is held in the session (a Better Auth session field `activeOrganiserId`, always re-checked against memberships; switcher in the header), not in the URL, so Playwright paths are stable. Changing it re-issues the core token with the new `org` (the membership cache is keyed by user and active organiser). The core token's `role` claim carries the exact membership role (`owner`, `admin`, `box_office`, `finance`; step 0's `staff` stays valid); the Go verifier accepts exactly these.
- Writes go through Next.js server actions that validate with zod schemas in `apps/web/src/lib/catalogue/schema.ts`, the same schemas the seed and (step 11) the AI copilot use. Each action runs inside the step 0 transaction helper.
- Text that fans see (event and product names, descriptions, perks) is stored as `{ ro, en }` objects (jsonb); the brand's languages (step 2, B10) decide which are required.

### D1.2 Slugs and addresses
Organiser and brand slugs: 3 to 40 characters, lowercase letters, digits and hyphens, globally unique across organisers (organiser slugs among organisers, brand slugs among brands; a platform-level unique index outside RLS, checked through a `SECURITY DEFINER` function that answers only "taken or free"), with a reserved list (`studio`, `api`, `s`, `status`, `fees`, `sign-in`, `admin`, `eps`, `www`). `brand_hostnames` holds hostnames entered only by configuration or seed (no screen; B8 is cut) and is read by step 3's host routing.

### D1.2b Organiser registry
`control.organisers` (a platform table) has forced row-level security: `eps_web` reads the organisers it is scoped to or a member of, inserts only the organiser its transaction is scoped to (onboarding, serialised per user so a double submit creates one organisation), updates only `name` and `onboarding_state`, and never deletes; `eps_core` reads all.

### D1.3 Money and fees
- Money is integer minor units (bani, cents) plus a currency per brand (`RON` or `EUR`); no floats anywhere.
- The platform fee lives in one platform table (`control.platform_fee`: percent in basis points, fixed minor units, effective from) published at `/fees`. Initial value: 2.5% and 0 fixed, matching what NIBIRU's checkout charges today.
- Fee policy (`absorb`, `pass_on`, `split`) is set per organiser with an optional per-event override. For a face price `p` and fee `f = round_half_up(p × bps / 10000) + fixed`: absorb → fan pays `p`, organiser gets `p − f`; pass on → fan pays `p + f`, organiser gets `p`; split → fan pays `p + floor(f / 2)`, organiser gets `p − ceil(f / 2)`. The fan-facing number is always the all-in total. Computed per unit, so a cart total is a sum of unit all-in prices (no cart-level rounding surprises).
- `apps/web/src/lib/pricing/` exports `feeBreakdown`, `activePriceStep`, `nextPriceStep` and `instalmentSchedule` as pure functions with shared test vectors in `apps/web/src/lib/pricing/vectors.json`; step 3's snapshot stores the computed results so Go never recomputes fees.

### D1.4 Catalogue model
- `venues` (per organiser, reusable), `events` (brand, venue, `{ro,en}` name/description, slug unique per brand, timezone default `Europe/Bucharest`, start and end, status `draft|published|archived` where only step 3 sets `published`, `edition_of` for copies), `event_days` (date, open and close local times; a night past midnight belongs to its start date), `lineup_slots` (artist name, day, stage, start time, order).
- `products`: `type` in `pass | day_ticket | flex_day | add_on | voucher | table | time_slot`, plus type-specific settings in a validated jsonb `config`: pass → `zone`, `days[]`; day ticket → `day` (or null: the buyer picks any event day when buying, used by Promenada's pick-a-day); flex day → `days[]` it may be used on, day picked later; add-on → `requires` (product ids or "any ticket"); voucher → `spend_value`; table → `category`, `seats`; time slot → `slot_minutes`, `first_start`, `last_start`, `max_consecutive_slots`. Common: `{ro,en}` name, perks, `named` (needs personalisation), `sales_from/until`, `visible`, `max_per_order`, sort order.
- `capacity_pools` (name, capacity, `scope` in `event | day | slot`) and `product_pools` (many-to-many, carrying `event_id`, so a product and its pools share an event by foreign key; line-up slots likewise reference a day of their own event). A day- or slot-scoped pool means one counter per day or slot instance with that capacity; step 3 expands the instances. A product is sellable only while every pool it draws on has room (pretix quota semantics). Passes, flexible days and add-ons draw on event pools only; day pools serve day tickets (and other single-day products such as vouchers or tables); slot pools serve time slots only, with one time grid per slot pool; slot instance ids use the event's local calendar date of the slot start (a slot after midnight belongs to the next local date). The readiness check enforces these.
- `price_steps` (product, mode `by_date` or `by_sales`, ordered; `from_at` or `after_sold`, unit face price). Exactly one mode per product; the first step has no threshold. "Sold" counts that product's paid units, supplied at sale time by the core (step 4); in Studio it is shown as 0 until sales exist. The public note text is fixed platform copy, not organiser-editable.
- Buying rules as columns on events (`min_age`, `consent_under_age`) and products (`max_per_order`) and an event-level `max_per_account` (stored here, enforced in step 5).
- `instalment_plans` per product: the deposit as `deposit_minor` or `deposit_bps` (exactly one), `monthly_count` (the most monthly payments), `day_of_month`. Each buyer's number of payments is computed from their purchase date in the event's time zone: payment k falls on `day_of_month` of the k-th local month after the purchase, and the last must fall at least 7 local days before the event's start date (`maxMonthlyPayments`); the plan is capped per buyer. Studio refuses a count that does not fit for the next buyer (now, or the first sales day when later), shows that buyer's schedule and the dates from which buyers get each number; readiness flags a plan only when no payment fits any more.
- `products` also store `personalise_deadline`, `checkin_deadline` and `late_checkin_fee_minor` (nullable, used by step 7).
- Edits are `UPDATE … WHERE id` (no row: not found); new rows always get server-generated ids. Brand currency is locked once the brand has events, an event moves only between brands of the same currency, and a brand with published events cannot be deleted (`ON DELETE RESTRICT` from events).
- Copy as new edition: one transaction copying the event, days, line-up, products, pools, ladder and plans with every date shifted by a chosen number of days (default: to the same weekday next year), status `draft`, `edition_of` set.

### D1.5 Readiness check
`eventReadiness(eventId)` returns a list of problems with codes (e.g. `product_without_pool`, `pool_capacity_zero`, `missing_translation`, `ladder_unordered`, `instalments_end_after_event`, `addon_requires_missing`). Studio shows it as a checklist; step 3 refuses to publish while it is non-empty.

### D1.6 Roles (Thin)
`memberships.role` in `owner | admin | box_office | finance` with `scope` (`organiser`, or a brand or event id). Enforcement in this prototype: owner-only actions are fee policy, team changes and deleting a brand; every other Studio action is allowed to any member. The last owner cannot be removed or demoted.

### D1.7 Seed
`apps/web/db/seed/` holds TypeScript seed definitions run by `npm run seed`: `preview.yml` runs it after migrations on every preview, and production is seeded only on request through `seed.yml` (workflow_dispatch, target preview or production). The registry row and the demo user are written as the connecting owner (the sample flag is not writable by `eps_web`); the catalogue goes through `withTx` as `eps_web`. The seed never deletes or changes a published event or anything that belongs to it, removes only unpublished extras a visitor added, and batches the per-day inserts. The sample organisers' and brands' slugs are reserved for everyone but the seed, and Studio cannot change a sample brand's address. Plan counts are stored capped to what fits for a buyer at seed time (NIBIRU Galaxia: 10 RON now and up to 7 monthly payments in early December 2026). It upserts by stable UUIDs (v5 from a seed namespace), goes through the same zod schemas and server-side domain functions as Studio (no raw inserts that skip validation), and only touches organisers flagged `is_sample`. Re-running restores the sample catalogue to its defined state. The demo organiser user from step 0 is made owner of the NIBIRU sample organiser. Every sample organiser carries `is_sample = true`; Studio and (step 3) storefronts show a "Sample data" label for it. Values the teardown does not state (pool sizes, intermediate ladder steps, court count) are seed assumptions, marked as such in the seed file.

## Risks / Trade-offs

- [jsonb `config` per product type could drift from the zod schema] → a discriminated-union schema validates every write and a test round-trips every seeded product.
- [Global slug uniqueness is a cross-organiser check under RLS] → one `SECURITY DEFINER` function returning a boolean; no other cross-organiser read.
- [Split fee with an odd fee in minor units] → the fan's half is floored and the organiser's ceiled, documented and covered by vectors, so totals always add up.
- [Seed running against production on every deploy] → idempotent, sample organisers only, and a test that runs it twice and compares.

## Open Questions

None that change the specs: seed assumptions (pool sizes, ladder dates, number of padel courts) are tuned in the seed file only.
