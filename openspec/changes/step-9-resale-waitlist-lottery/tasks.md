# Tasks

## 1. Schema, contract and settings

- [ ] 1.1 Add sales-schema migrations for resale policies, listings, transfers, waitlist entries, lottery moments, registrations, access codes and upgrade paths, each with `organiser_id` and row-level security, and a `face_value` column on tickets; verify the migration applies on testcontainers Postgres and an RLS test shows another organiser cannot read any of them.
- [ ] 1.2 Add the resale, transfer, waitlist, lottery and upgrade paths to `api/openapi.yaml` and regenerate types and stubs; verify both builds pass with a clean generated diff.
- [ ] 1.3 Compile the resale policy, lottery moments and upgrade paths into the sale snapshot on publish (or the versioned `sale_extras` record) and add their EPS Studio settings screens; verify `services/internal/resale/policy_test.go` (off, closed) and a manual check of the settings on the preview.

## 2. Face-value resale and transfer (Real)

- [ ] 2.1 Store face value on issue and carry it across reissues; implement listing with the cap, the eligibility rules, delist, and auto-close on a door entry; verify `services/internal/resale/listing_test.go`.
- [ ] 2.2 Implement buying a listing (hold on the listing, resale order, reissue with cleared personalisation on `order.paid`, `resale.sold`), with refund and relist if the reissue fails; verify `services/internal/resale/sale_test.go` (completes, declined, two buyers).
- [ ] 2.3 Post the seller payout to the EPS Wallet ledger in the same transaction and add a reconciliation check; verify `services/internal/resale/payout_test.go` (entries sum to zero, no payout on failed reissue).
- [ ] 2.4 Implement transfer by email or link (pending, cancel, accept as user or guest, reissue); verify `services/internal/resale/transfer_test.go`.
- [ ] 2.5 Build Sell and Transfer on the live ticket (cap, payout preview, sold and transferred states), the transfer accept page, and the storefront resale section with face-value labels, in RO and EN; verify `apps/web/tests/e2e/resale.spec.ts` (payout preview, storefront section, old code rejected and new code accepted on the door page).

## 3. Waitlist (Thin)

- [ ] 3.1 Add authorise, capture and void to the fake providers (simulated, no card data) behind the step 4 adapter; verify a fakepsp unit test for each.
- [ ] 3.2 Implement join (pre-authorisation, cap, position), leave (void) and bot-flagged entries behind others; verify `services/internal/waitlist/join_test.go`.
- [ ] 3.3 Implement allocation in the worker for new listings and returned tickets (hold for the head, capture, reissue or issue, next on failure); verify `services/internal/waitlist/allocate_test.go`.
- [ ] 3.4 Build the waitlist join and leave screen on sold-out products with the fan's position, in RO and EN; verify by manual check on the preview.

## 4. Lottery (Thin)

- [ ] 4.1 Implement lottery moments with seed generation and the published commitment, and registration (one per person, window, cap); verify `services/internal/lottery/register_test.go`.
- [ ] 4.2 Implement the draw (HMAC ranking, bot-flagged last, idempotent, published seed and order) and "Draw next" for unused codes; verify `services/internal/lottery/draw_test.go` including a recompute from published data.
- [ ] 4.3 Issue single-use account-bound access codes through the step 5 access-code mechanism; verify `services/internal/lottery/codes_test.go`.
- [ ] 4.4 Build the lottery page (register, commitment, result), the organiser's draw screen and the winner and non-winner notices, in RO and EN; verify by manual check on the preview with seeded registrations.

## 5. Upgrades (Thin)

- [ ] 5.1 Implement upgrade pricing, target hold, payment and reissue with original stock returned and face value updated; verify `services/internal/upgrade/upgrade_test.go` (GA to VIP, VIP sold out, blocked tickets).
- [ ] 5.2 Build the Upgrade dialog on the live ticket with the all-in price, in RO and EN; verify by manual check on the preview.

## 6. Ship (CLAUDE.md loop)

- [ ] 6.1 Run the step's Go tests and web tests, then the full suite once; verify all pass locally.
- [ ] 6.2 Open a PR from `step-9-resale-waitlist-lottery` to `main`; verify CI is green and the Vercel, Neon and DOKS previews are deployed.
- [ ] 6.3 Run the three adversarial reviewers in parallel (correctness: listing and delist races, reissue versus payment failure, waitlist capture failures, idempotent draw; security and data: cap bypass through the API, payout and ledger integrity, transfer link guessing, lottery seed handling, organiser isolation; spec and scale: every scenario tested, allocation and draw at 100k registrations); verify each report is in the PR.
- [ ] 6.4 Fix every confirmed finding or record in the PR why it is rejected, and re-run the affected tests; verify the PR lists each finding with its outcome.
- [ ] 6.5 Run Playwright against the deployed preview URL on desktop and phone, RO and EN, no console errors and no horizontal scroll: resale at face value end to end, and on a phone viewport the door page admitting the buyer's new rotating QR while rejecting the seller's old code and a screenshot of it; plus API checks (listing above face value refused) against the deployed Go core; verify the run passes and its report is linked in the PR.
- [ ] 6.6 Squash-merge when CI is green, findings are fixed and the deployed tests pass; verify the merge commit on `main`.
- [ ] 6.7 Run the Playwright smoke test against production (list a sample ticket at face value, see it on the storefront, delist it); verify it passes.

## Workflow follow-up

- Archive with `/opsx:archive`: merge the deltas into `openspec/specs/`, add `## Status` to proposal.md (what was built, deviations, checks with results) and tick step 9 in `docs/planning/build-plan.md`.
- Verify the archived specs validate with `npx openspec validate --all --strict`.
