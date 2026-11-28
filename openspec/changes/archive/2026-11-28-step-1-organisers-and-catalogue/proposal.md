# Proposal

Step 1: Organisers and catalogue

**Demo act:** Act 1 (an organiser signs up, holds two brands, sets the fee policy and builds the catalogue), and the data every later act sells.
**Covers:** features A1, A2, A3 (Thin), A6, C1, C2, C3 (Thin), C4, C5, C8 (Thin), C9 (Thin), C10; decisions U2 (applied to the new tables) and P1 (the catalogue is the draft side that step 3 compiles).

## Why

The platform is open to every organiser, so the first real screen is self-serve: an organiser signs up, names the organisation, adds brands and describes what they sell, without code and without talking to sales. NIBIRU's 2027 catalogue (passes by zone, tables by category, a price ladder, a drink voucher, padel slots, camping, instalments) is rebuilt as organiser #1 through the same model, which proves the model is general and gives the demo real data.

## What Changes

- Organiser sign-up and onboarding in EPS Studio: organisation name and address slug, first brand, a template choice (stored here, turned into a draft shop by step 2), owner membership, active-organiser switcher.
- Several brands per organiser, each with its own slug (its shop address `/s/<brand>`), languages, currency and optional hostnames set by configuration.
- Team roles screen (Thin): owner, admin, box office, finance, with a scope; only owner versus everyone else is enforced.
- A published platform fee and a fee-policy switch (absorb, pass on, split) per organiser with a per-event override, and one all-in price calculation the whole platform uses.
- Catalogue: venues, events with days and line-ups, copy as a new edition; product types (pass by zone, day ticket, add-on, and NIBIRU's types: spend voucher, table by category, time slot, flexible day); capacity pools shared many-to-many with day and slot scopes; price ladders by date or by sales with a public note; buying rules (age, parental consent, per-order and per-account maxima); instalment settings with a generated schedule.
- A readiness check that lists what would stop an event from being published (used by step 3).
- An idempotent seed that builds the NIBIRU 2027 sample organiser (two brands) and a small second sample organiser, clearly labelled as sample data, through the same validation as Studio.

## Capabilities

### New Capabilities
- `organisers`: sign-up and onboarding, brands, active organiser, team roles, published platform fee and fee policy with the all-in price calculation.
- `catalogue`: venues, events, days, line-ups, editions, product types, capacity pools, price ladders, buying rules, instalment settings, readiness check.
- `sample-data`: the NIBIRU 2027 sample organiser and a second sample organiser, seeded idempotently and labelled.

### Modified Capabilities
None.

## Impact

- New control-schema tables (all with `organiser_id` and forced row-level security except the platform fee table): `brands`, `brand_hostnames`, `fee_policies`, `venues`, `events`, `event_days`, `lineup_slots`, `products`, `capacity_pools`, `product_pools`, `price_steps`, `instalment_plans`, `invitations`; `organisers` and `memberships` from step 0 gain columns.
- EPS Studio screens under `/studio`, a public `/fees` page, a shared money and pricing module in `apps/web/src/lib/pricing/` that steps 3 and 4 reuse.
- Previews are seeded after migrations; production is seeded only on request (`seed.yml`).
- `zod` for input validation (small utility).

## Status

Built and merged as PR #2 (squash f612670) on 28 Nov 2026.

**What was built:**
- Organiser sign-up with resumable onboarding, plus brands with an active-organiser switch that the core token follows.
- Team: roles, scopes and invitation links. Only owner versus everyone else is enforced; the screen labels scopes as not enforced in the prototype.
- Fee policy (absorb, pass or split, with a per-event override) and all-in prices in integer minor units, with vectors shared with Go.
- Venues, and events with days and line-up.
- Seven product types, with capacity pools scoped by event, day or slot.
- Price ladders, instalment plans capped per buyer in the event's local dates, and buying rules.
- Copy-as-new-edition and a readiness checklist.
- A public `/fees` page.
- NIBIRU and Sunwave sample data, labelled "Date de exemplu / Sample data", loaded by a manual `seed.yml` workflow.

**Deviations, recorded in design.md:**
- An empty day means the buyer picks the day.
- The instalment limit is computed per buyer from the purchase date. The NIBIRU sample is "10 RON + up to 7 monthly".
- Brand slugs and organiser slugs are separate namespaces.
- The core token carries the exact role: owner, admin, box_office, finance or staff.
- Invite URLs contain the organiser id.
- `tsx` was added as a dev dependency.
- New columns were added for later steps: personalisation and check-in deadlines, a late check-in fee and deposit_bps.
- Seeding moved out of deploy.yml into seed.yml.
- Migrations 00005 and 00006 were edited in place before reaching production.

**Checks run:**
- Vitest 201/201 against the database; tsc, eslint and next build pass.
- Go vet and tests pass; contract drift, actionlint and `openspec validate --all --strict` pass.
- Local Playwright: 31 passed, 9 skipped (specs that need the core, and phone runs of the flows that create data).
- All 17 PR checks are green; the deployed e2e had 37 passed and 9 skipped.
- An independent Playwright check of the preview, on desktop and phone: the demo sign-in opens Studio with NIBIRU labelled "Date de exemplu"; no horizontal scroll.
- Three adversarial reviews found 2 high and 3 medium security issues, 2 major and 6 minor correctness defects, and 4 high, 6 medium and 4 low spec/scale issues. All confirmed findings were fixed: prototype-safe form parsing with zod, RLS and narrowed grants on `control.organisers`, update-only edits with server-generated ids, sample slugs reserved, a safe idempotent seed, pool scopes enforced, composite foreign keys including event_id, indexes, and Studio home queries bounded and paginated.
- Production deploy run 4 is green. The production seed (`seed.yml`, target production) ran after it, (run 46088607056, green), followed by a production Playwright smoke check on desktop and phone: the pages render with no console errors and no horizontal scroll, and the demo sign-in opens Studio with NIBIRU labelled "Date de exemplu".
