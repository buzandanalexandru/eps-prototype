# Tasks

## 1. Schema

- [x] 1.1 Create branch `step-1-organisers-and-catalogue`; add the control migration for `brands`, `brand_hostnames`, `invitations`, `fee_policies`, the platform table `platform_fee`, and new columns on `organisers` (`is_sample`, `onboarding_state`) and `memberships` (`scope`); forced RLS on every organiser table; verify `services/internal/tenancy/schema_test.go` passes with the allowlist extended only by `platform_fee`
- [x] 1.2 Add the catalogue migration (`venues`, `events`, `event_days`, `lineup_slots`, `products`, `capacity_pools`, `product_pools`, `price_steps`, `instalment_plans`) with composite foreign keys that include `organiser_id`; verify `schema_test.go` and `apps/web/tests/unit/catalogue-isolation.test.ts` (cross-organiser link refused) against the CI Postgres service
- [x] 1.3 Add the `SECURITY DEFINER` slug availability function and the global unique indexes; verify `apps/web/tests/unit/slugs.test.ts` (taken, reserved, malformed, no holder details)

## 2. Pricing module

- [x] 2.1 Implement `apps/web/src/lib/pricing/` (`feeBreakdown`, `activePriceStep`, `nextPriceStep`, money helpers in minor units) with `vectors.json`; verify `apps/web/tests/unit/pricing.test.ts` (pass on 539 → 552.48, split odd fee, absorb 25 → 24.37, free, date and sales ladders, before first threshold)
- [x] 2.2 Implement `instalmentSchedule`; verify `apps/web/tests/unit/instalments.test.ts` (10 RON + 9, remainder on last, past-the-event refusal with latest count, deposit too large)

## 3. Organisers

- [x] 3.1 Build onboarding (`/studio/onboarding`: organisation, first brand, template choice) with resume from `onboarding_state`; RO/EN messages; verify `apps/web/tests/e2e/organiser-onboarding.spec.ts` locally
- [x] 3.2 Build brands list and settings (name, slug, languages, currency, logo slot) and the active-organiser switcher that re-issues the core token; verify `apps/web/tests/e2e/brands.spec.ts` and `apps/web/tests/unit/active-organiser.test.ts`
- [x] 3.3 Build the team screen (members, roles, scopes, invitations with copyable link, owner-only guard, last-owner rule); verify `apps/web/tests/unit/roles.test.ts` and `apps/web/tests/e2e/team-roles.spec.ts`
- [x] 3.4 Build the public `/fees` page and the fee-policy settings with per-event override and "Fan pays / You receive"; verify `apps/web/tests/e2e/fees-page.spec.ts` and `apps/web/tests/unit/fee-policy.test.ts`

## 4. Catalogue

- [x] 4.1 Write the zod schemas (events, days, line-up, product discriminated union by type, pools, ladders, rules, plans) in `apps/web/src/lib/catalogue/schema.ts`; verify `apps/web/tests/unit/catalogue-products.test.ts` (each type valid and invalid, sales window, ladder ordering and mixing)
- [x] 4.2 Build venues and events screens (days, opening hours past midnight, line-up by day and stage); verify `apps/web/tests/unit/catalogue-events.test.ts`
- [x] 4.3 Build the products screen with type-specific forms and explanations, perks, named flag, sales window, visibility, max per order; verify the product tests above and a Playwright check that each type can be created (`apps/web/tests/e2e/catalogue.spec.ts`)
- [x] 4.4 Build the pools screen (pool bars with the products that draw on each, scope event/day/slot, many-to-many links, available = min of remaining); verify `apps/web/tests/unit/catalogue-pools.test.ts`
- [x] 4.5 Build the price ladder editor (mode, steps, current and next step preview, fixed public note) and the instalment panel with the schedule preview; verify `catalogue.spec.ts` covers both panels
- [x] 4.6 Build the buying rules panel (min age, consent age, max per account, max per order); verify `apps/web/tests/unit/catalogue-rules.test.ts`
- [x] 4.7 Implement copy as new edition (one transaction, date offset, `edition_of`); verify `apps/web/tests/unit/catalogue-editions.test.ts`
- [x] 4.8 Implement `eventReadiness` and the checklist with links; verify `apps/web/tests/unit/catalogue-readiness.test.ts`

## 5. Sample data

- [x] 5.1 Write the seed definitions in `apps/web/db/seed/` (NIBIRU: two brands, Galaxia 2027 passes, pools, camping, instalments; resort: Beer Garden tables, Promenada ladder and Any-Day, Center Stage voucher, padel slots, stand-up; Sunwave), stable v5 ids, assumptions marked; verify `apps/web/tests/unit/seed-nibiru.test.ts` (prices, pools, ladder, special types, all events ready, run twice, real organiser untouched)
- [ ] 5.2 Add the "Sample data" / "Date de exemplu" label for sample organisers; add a `seed` job after migrations in `preview.yml` and `deploy.yml`; verify `apps/web/tests/e2e/sample-data.spec.ts` locally and that the preview job logs a successful seed

## 6. Step checks (CLAUDE.md loop)

- [x] 6.1 Run the step's tests locally, then the full suite once; verify all pass
- [x] 6.2 Run `npx openspec validate step-1-organisers-and-catalogue --strict`; verify it passes
- [ ] 6.3 Open the PR to `main`; verify CI is green and the preview is seeded
- [ ] 6.4 Run the three adversarial reviewers in parallel (correctness; security and data; spec and scale), fix every confirmed finding or record its rejection in the PR, re-run affected tests; verify no open confirmed finding
- [ ] 6.5 Run Playwright against the deployed preview URL (onboarding, brands, team, fees page, catalogue, sample data; desktop and phone, RO and EN, no console errors, no horizontal scroll); verify all pass
- [ ] 6.6 Squash-merge when green; verify the production deploy, the production seed and the Playwright smoke test pass

## Workflow follow-up

- Archive with `/opsx:archive`: merge the deltas into `openspec/specs/`, add `## Status` to the proposal and tick step 1 in `docs/planning/build-plan.md`.
- Verify the archived specs validate.
