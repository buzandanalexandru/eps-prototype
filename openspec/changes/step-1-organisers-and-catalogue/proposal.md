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
- CI gains a seed job after migrations on previews and production.
- `zod` for input validation (small utility).
