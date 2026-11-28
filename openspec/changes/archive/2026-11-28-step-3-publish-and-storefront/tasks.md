# Tasks

## 1. Snapshot contract and storage

- [x] 1.1 Create branch `step-3-publish-and-storefront`; write `api/schemas/sale-snapshot.v1.json`, TS types in `apps/web/src/lib/snapshot/types.ts` and the Go struct in `services/internal/snapshot`; verify `services/internal/snapshot/schema_test.go` validates a hand-written fixture and rejects one with a missing field
- [x] 1.2 Add the control migration for `sale_snapshots` (unique event and version, immutability trigger), `events.live_snapshot_version`, and the `SECURITY DEFINER` functions `live_snapshot` and `public_storefront` with `EXECUTE` for `eps_core` and the storefront path; verify `services/internal/snapshot/immutable_test.go` and `services/internal/tenancy/schema_test.go`
- [x] 1.3 Add `activate`, `pool-usage` and public `availability` operations to `api/openapi.yaml` and regenerate; verify the contract drift check and `services/internal/api/contract_test.go`

## 2. Compiler and publish gate

- [x] 2.1 Implement the compiler in `apps/web/src/lib/snapshot/compile.ts` (sold events from pages, pool instances by event/day/slot with stable ids, price steps with precomputed amounts, instalments, rules, hash, skip when unchanged); verify `apps/web/tests/unit/snapshot-compile.test.ts` (first publish, design-only, restore uses current prices, day and slot instances, GA amounts)
- [x] 2.2 Wire compile into step 2's publish transaction with readiness and pool-usage gates and version locking per event; verify `apps/web/tests/unit/snapshot-publish-gate.test.ts` (not ready, capacity below usage, core unreachable) and the concurrent-publish scenario
- [x] 2.3 After commit: call `activate` per event and `revalidateTag` for the brand and events; show live version, time and "Changes not published" in Studio; verify `apps/web/tests/e2e/publish.spec.ts` locally

## 3. Go snapshot reader and availability

- [x] 3.1 Implement `snapshot.Load` (Redis `eps:{org}:<event>:snapshot:live`, DB fallback through `live_snapshot`, hash check, 5 s in-process copy, earlier versions by number) and `activate` with organiser scoping; verify `services/internal/snapshot/store_test.go` with testcontainers (draft change ignored, hash mismatch, earlier version, activation, lost activation within 10 s, foreign event 404)
- [x] 3.2 Implement `pool-usage` (zeros until sales tables exist, read from the sales schema when they do) and public `availability` (status, price step, all-in, remaining under threshold, hidden products, 2 s cache); verify `services/internal/snapshot/availability_test.go`

## 4. Storefront

- [x] 4.1 Build routes under `apps/web/src/app/s/[brand]/[locale]/` (home, `e/[event]`, `[page]`, checkout placeholder) with the language redirect and same 404 for unknown and unpublished brands; extend middleware with host routing through `public_storefront` and the link helper; verify `apps/web/tests/unit/storefront-routing.test.ts`
- [x] 4.2 Render published versions with the shared renderer in published mode reading live snapshots, cache tags per brand and event; verify `apps/web/tests/unit/storefront-cache.test.ts` (second request makes no query)
- [x] 4.3 Build `<AllInPrice>`, "from" prices, fee line, ladder notes and instalment summaries, plus the lint rule banning other money formatting in storefront code; verify `apps/web/tests/unit/storefront-prices.test.tsx`
- [x] 4.4 Build the availability island (statuses, few-left counts, paused buying on failure) and the ticket picker (limits, add-on requirements, day and slot choice, running total, hand-off URL); verify `apps/web/tests/unit/ticket-picker.test.ts`
- [x] 4.5 Add the sample-data banner and noindex for sample organisers, phone layout and language switch; verify `apps/web/tests/e2e/storefront.spec.ts` and `apps/web/tests/e2e/storefront-prices.spec.ts` locally against a local core

## 5. Step checks (CLAUDE.md loop)

- [x] 5.1 Run the step's tests locally (Vitest, Go with testcontainers, the local Playwright specs), then the full suite once; verify all pass
- [x] 5.2 Run `npx openspec validate step-3-publish-and-storefront --strict`; verify it passes
- [x] 5.3 Open the PR to `main`; verify CI is green, the preview is seeded and published, and the preview core serves availability for Galaxia 2027
- [x] 5.4 Run the three adversarial reviewers in parallel (correctness, including publish races and cache staleness; security and data, including the published-only reads and host routing; spec and scale, including cache hit behaviour under load), fix every confirmed finding or record its rejection, re-run affected tests; verify no open confirmed finding
- [x] 5.5 Run Playwright against the deployed preview URL (publish then shop refresh, all-in prices, picker, availability failure, host-routing unit coverage, desktop and phone, RO and EN, no console errors, no horizontal scroll) plus API checks on the preview core (`availability`, `activate` without and with the wrong organiser); verify all pass
- [x] 5.6 Squash-merge when green; verify the production deploy and the Playwright smoke test (NIBIRU shop shows 552.48 RON for GA) pass

## Workflow follow-up

- Archive with `/opsx:archive`: merge the deltas into `openspec/specs/`, add `## Status` to the proposal and tick step 3 in `docs/planning/build-plan.md`.
- Verify the archived specs validate.
