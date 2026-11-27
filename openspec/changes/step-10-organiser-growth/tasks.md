# Tasks

## 1. Data model and contract

- [ ] 1.1 Add control-schema migrations (promoters, codes, tracked links, code sets, channels, webhook endpoints, payment accounts) and sales-schema migrations (order attribution and channel columns, hourly roll-ups, processed-event set, webhook deliveries), each with `organiser_id` and RLS; verify with the RLS isolation test that a second organiser reads zero rows from every new table
- [ ] 1.2 Create the default channel for every existing event in the migration; verify existing seed orders show the default shop
- [ ] 1.3 Extend `api/openapi.yaml` (sales summary, code validation, channel and attribution fields on holds and orders, `codes.published` and the enriched `order.paid` payload) and regenerate TS types and Go stubs; verify CI's contract check passes

## 2. Sales channels (B11)

- [ ] 2.1 Studio Shops screen: add a shop with path, product subset and optional access-code requirement; compile shops into the sale snapshot on publish; verify `services/core/internal/channels/channels_test.go` (shared pools, access-only refusal) passes
- [ ] 2.2 Storefront route `/s/<brand>/<shop>` showing only that shop's products; record `channel_id` on holds and orders; verify `apps/web/tests/e2e/sales-channels.spec.ts` passes

## 3. Codes and attribution (C6 core path)

- [ ] 3.1 Promo, influencer and access codes in the control schema with the code-set compile and the `codes.published` outbox event; core caches the code set in Redis; verify `services/core/internal/codes/codes_test.go` (unlock, used up, expired, 50 concurrent uses of a 10-use code) passes
- [ ] 3.2 Tracked links: `?ref=` read by the storefront, 7-day last-click cookie, sent with the hold; core resolves code-over-link attribution onto hold and order and puts it on `order.paid`; verify `services/core/internal/codes/attribution_test.go` passes
- [ ] 3.3 Cart shows the discounted all-in price as soon as a code is applied; code checks rate limited per session and fed to bot scoring; verify the discount scenario in `apps/web/tests/e2e/promoter-hub.spec.ts`
- [ ] 3.4 Simulator option to send a share of virtual fans through given tracked links; verify a simulator run records attributed orders in the sales schema

## 4. Roll-ups and the sales dashboard (G1)

- [ ] 4.1 Worker `growth-rollups` consumer group: idempotent per event ID, Redis live counters per event, product, day, shop and promoter, hourly Postgres roll-up, nightly recompute; verify `services/worker/internal/rollup/rollup_test.go` and `promoter_test.go` (redelivery counted once, expired orders ignored) pass
- [ ] 4.2 Core sales summary endpoint (paid only, other states as counts, fee from order snapshot, minor units per currency, Europe/Bucharest days, simulated flag, organiser and brand scope); verify `services/core/internal/sales/summary_test.go` passes, including the reconciliation against a direct sum of orders
- [ ] 4.3 Studio Sales screen with per-product, per-day and per-shop tables, Simulated filter, sample label and SSE live updates throttled to 1/s; verify `apps/web/tests/e2e/sales-dashboard.spec.ts` passes with a simulator run

## 5. Promoter Hub screens (C6)

- [ ] 5.1 Promoters screen: create promoters with commission, codes and copyable links, live leaderboard over SSE; verify the create and leaderboard scenarios in `apps/web/tests/e2e/promoter-hub.spec.ts`
- [ ] 5.2 Payouts list (commission × attributed net, mark paid as a record) and the revocable read-only promoter stats link; verify `apps/web/tests/unit/promoter-stats-link.test.ts` passes and the payouts list renders on the dev server

## 6. Audience (G3)

- [ ] 6.1 Audience screen over paid orders with consent flags, per organiser; CSV export (consent-only default, owner-only all-buyers export logged, formula-prefix escaping, deleted users excluded); verify `apps/web/tests/unit/audience-export.test.ts` and `apps/web/tests/e2e/audience.spec.ts` pass

## 7. Webhooks (G5)

- [ ] 7.1 Webhooks screen: add endpoint (HTTPS only, secret shown once, stored encrypted), choose event types; verify the add scenario in `apps/web/tests/e2e/webhooks.spec.ts`
- [ ] 7.2 Worker `webhooks` consumer group: SSRF target check before each send, signed POST, 5 s timeout, 6 attempts with backoff and jitter, failed state, auto-disable after 20 consecutive failures; verify `services/worker/internal/webhooks/target_test.go` and `deliver_test.go` pass
- [ ] 7.3 Delivery log with Redeliver and Send test event, plus the sample-only demo receiver at `/api/dev/webhook-sink/<endpoint>`; verify on the dev server that a test event shows as delivered with a verified signature

## 8. Marketplace (A4) and payouts (A5)

- [ ] 8.1 `/marketplace` page (RO and EN) over published, listed snapshots, cached until `snapshot.published`, sample label, listing switch in organiser settings; verify `apps/web/tests/e2e/marketplace.spec.ts` passes
- [ ] 8.2 Payment account screen (simulated connect or EPS as merchant, owner-only changes, no-real-payments notice) and weekly simulated payouts list; verify `apps/web/tests/e2e/payouts.spec.ts` passes
- [ ] 8.3 Add all new Studio and fan text to the RO and EN message files with correct diacritics; verify the i18n key check passes

## 9. Delivery loop (CLAUDE.md)

- [ ] 9.1 Run the step's Go, Vitest and Playwright tests locally, then the full suite once; verify all green
- [ ] 9.2 Open the PR `step-10-organiser-growth` to `main`; verify CI is green and the Vercel preview and DOKS preview images are deployed
- [ ] 9.3 Run the three adversarial reviewers in parallel (correctness: attribution races, redelivery, code limits; security and data: RLS on every new table, webhook SSRF and secrets, CSV export consent and injection, code brute force; spec and scale: every scenario tested, roll-up and SSE behaviour under 100k-fan load); verify each returns a ranked findings list
- [ ] 9.4 Fix every confirmed finding or record the rejection reason in the PR, then re-run the affected tests; verify green
- [ ] 9.5 Run Playwright against the deployed preview URL (desktop and phone, RO and EN on the marketplace and storefront shops, no console errors, no horizontal scroll) plus API checks against the deployed core (sales summary, code validation); verify all pass
- [ ] 9.6 Squash-merge once CI, reviews and deployed tests are green; verify the production deploy with the Playwright smoke test (Sales screen, leaderboard and marketplace load)

## Workflow follow-up

- Archive with `/opsx:archive step-10-organiser-growth`: merge the deltas into `openspec/specs/`, add `## Status` to the proposal, tick step 10 in `docs/planning/build-plan.md`.
