# Tasks

## 1. Schema, contract and settings

- [ ] 1.1 Add sales-schema migrations for `squads` and `squad_seats` (state, token hash, payer, child order, display name, deadline) with `organiser_id` and row-level security; verify the migration applies on testcontainers Postgres and an RLS test shows another organiser cannot read a squad.
- [ ] 1.2 Add squad paths (create, get, seats, replace link, release seat, join, pay share, cover, claim) to `api/openapi.yaml` and regenerate types and stubs; verify both builds pass with a clean generated diff.
- [ ] 1.3 Add the squad switch and squad window to sale settings and the sale snapshot (or confirm step 5 already has them); verify `services/internal/squad/create_test.go` (switched off, window from the snapshot).

## 2. Group hold and activation

- [ ] 2.1 Implement squad creation: N within per-order max and leader cap, queue pass check, all-or-nothing group hold, seats and token hashes, `squad.created`; verify `create_test.go` (squad of four, not enough stock, over cap, no pass).
- [ ] 2.2 Implement activation on the leader's `order.paid` (extend the group hold to the squad deadline) and expiry of a forming squad; verify `services/internal/squad/lifecycle_test.go`.

## 3. Seats and paying a share

- [ ] 3.1 Implement seat links (128-bit tokens stored hashed, replace, release early, rate-limited lookups); verify `services/internal/squad/seats_test.go`.
- [ ] 3.2 Implement joining and paying a share: compare-and-set seat lock, unit moved from the group hold to a one-unit child order at the locked price, payer cap check, idempotency key per seat attempt, instalment option, decline back to open; verify `services/internal/squad/pay_test.go` (full, instalments, declined, over cap, two payers, double click, price step-up).
- [ ] 3.3 Mark seats paid from `order.paid` of the child order and write `squad.seat_paid` and `squad.completed`; verify the completion case in `lifecycle_test.go`.

## 4. Deadline, cover and claim

- [ ] 4.1 Implement the deadline scheduler (Redis sorted set plus Postgres sweep), release of open seats, 2-minute grace for paying seats, `squad.expired`, and automatic refund of late confirmations; verify `services/internal/squad/expiry_test.go` (two of four, in-flight, late confirmation, worker down, no double release).
- [ ] 4.2 Implement leader cover (one order for chosen unpaid seats, tickets to the leader) and claim through the step 7 reissue primitive; verify `services/internal/squad/cover_test.go` (cover, claim, cover racing a friend's payment).
- [ ] 4.3 Implement the squad status endpoint with the privacy rules and a Redis cache per squad; verify `services/internal/squad/status_test.go`.

## 5. Screens

- [ ] 5.1 Add "Buy as a squad" with a size picker to the storefront product and checkout (only when switched on); verify by manual check on the preview with squad buy on and off.
- [ ] 5.2 Build the leader page `/s/[brand]/squad/[id]` (seats, share and copy buttons, replace link, release, cover, countdown, 3-second polling) in RO and EN; verify the leader part of `apps/web/tests/e2e/squad-buy.spec.ts`.
- [ ] 5.3 Build the friend page `/s/[brand]/squad/join/[token]` (share price, deadline, progress, pay my share or first instalment, expired and released states, claim for covered seats) in RO and EN; verify `squad-buy.spec.ts` with four browser contexts (leader on desktop, three friends on phone viewports: two pay, one is covered then claims) and the Romanian case.

## 6. Ship (CLAUDE.md loop)

- [ ] 6.1 Run the step's Go tests and web tests, then the full suite once; verify all pass locally.
- [ ] 6.2 Open a PR from `step-8-squad-buy` to `main`; verify CI is green and the Vercel, Neon and DOKS previews are deployed.
- [ ] 6.3 Run the three adversarial reviewers in parallel (correctness: seat races, deadline versus in-flight payments, late webhooks, hold moves, sweep idempotency; security and data: link guessing, cap gaming, refunds and ledger integrity, organiser isolation, what the leader can see; spec and scale: every scenario tested, stock held by squads during a 100k on-sale, expiry load); verify each report is in the PR.
- [ ] 6.4 Fix every confirmed finding or record in the PR why it is rejected, and re-run the affected tests; verify the PR lists each finding with its outcome.
- [ ] 6.5 Run Playwright against the deployed preview URL on desktop and phone, RO and EN, no console errors and no horizontal scroll: the full squad of four, each friend naming their ticket, and on a phone viewport the door page admitting a friend's rotating QR and rejecting a screenshot of it; plus API checks of squad endpoints against the deployed Go core; verify the run passes and its report is linked in the PR.
- [ ] 6.6 Squash-merge when CI is green, findings are fixed and the deployed tests pass; verify the merge commit on `main`.
- [ ] 6.7 Run the Playwright smoke test against production (start a squad of two, the friend pays from the link, both tickets in their wallets); verify it passes.

## Workflow follow-up

- Archive with `/opsx:archive`: merge the delta into `openspec/specs/`, add `## Status` to proposal.md (what was built, deviations, checks with results) and tick step 8 in `docs/planning/build-plan.md`.
- Verify the archived specs validate with `npx openspec validate --all --strict`.
