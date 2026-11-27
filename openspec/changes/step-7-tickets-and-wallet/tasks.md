# Tasks

## 1. Schema, contract and keys

- [ ] 1.1 Add sales-schema migrations for tickets, ticket revocations, door devices, scans and scan conflicts, each with `organiser_id` and row-level security; verify the migration applies on a fresh testcontainers Postgres and an RLS test shows organiser B cannot read organiser A's tickets.
- [ ] 1.2 Add the ticket, codes, keys, revocations, door-sync, scans, personalise, check-in, pick-day and `/v1/me/trip` paths to `api/openapi.yaml` and regenerate Go stubs and TypeScript types; verify the step 0 code-generation command leaves a clean diff and both builds pass.
- [ ] 1.3 Add CI steps that generate the Ed25519 ticket key and key ID per environment into the core API's Kubernetes secret (never into Vercel) and wire it through Helm values; verify a preview deploy has the secret and the core API fails health without it (`services/internal/tickets/keys_test.go`).

## 2. Token signing and verification

- [ ] 2.1 Implement the 104-byte token (design D2) with Ed25519 signing in Go and fixed test vectors; verify `services/internal/tickets/token_test.go` (valid, altered byte, unknown key ID).
- [ ] 2.2 Implement the TypeScript verifier (WebCrypto Ed25519 with the library fallback) checked against the Go vectors; verify `apps/web/tests/unit/ticket-token.test.ts`.
- [ ] 2.3 Serve `GET /v1/ticket-keys` with key ID, public key and validity; verify `keys_test.go` and `apps/web/tests/unit/no-ticket-secret.test.ts` (no private key in the web build or env).

## 3. Issuing and reissuing

- [ ] 3.1 Add the worker consumer for `order.paid` that issues one ticket per unit with the unique unit key and writes `ticket.issued` to the outbox; verify `services/internal/tickets/issue_test.go` (paid, declined, duplicate and concurrent delivery).
- [ ] 3.2 Implement the reissue primitive (generation bump plus revocation row in one transaction, conflict on concurrent reissue) for later steps to call; verify `services/internal/tickets/reissue_test.go`.
- [ ] 3.3 Implement suspension and its lifting from instalment plan state; verify `services/internal/tickets/suspend_test.go`.

## 4. Codes for the holder and the live ticket page

- [ ] 4.1 Implement `GET /v1/tickets/{id}/codes` (holder only, current generation, at most 40 windows, not found for anyone else); verify `services/internal/tickets/codes_test.go`.
- [ ] 4.2 Build `/[locale]/wallet/tickets/[id]` with the animated QR, countdown, in-memory buffer, clock offset, refetch below 5 minutes and the "Reconnect to show your code" state; verify `apps/web/tests/e2e/tickets.spec.ts` (three codes in 45 s, offline after the buffer).
- [ ] 4.3 Build the wallet pass preview dialog (Apple and Google, brand tokens, signing note, RO and EN); verify by manual check on the preview and a screenshot in the PR.

## 5. Revocation feed and door page

- [ ] 5.1 Implement `GET /v1/events/{id}/door-sync?since=` (revocations, entries, manifest, server time) for the organiser's door roles only; verify `services/internal/tickets/revocations_test.go` and `services/internal/door/access_test.go`.
- [ ] 5.2 Implement `POST /v1/events/{id}/scans` (idempotent by scan ID, first entry wins by device time, conflicts recorded); verify `services/internal/door/sync_test.go`.
- [ ] 5.3 Implement the door verification module (signature, event, window ±1 with clock offset, revocations, suspensions, single entry, stale-sync state); verify `apps/web/tests/unit/door-verify.test.ts` covering every door-scanning scenario.
- [ ] 5.4 Build `/door/[event]`: Prepare this device, service worker shell cache, IndexedDB store, camera scan (BarcodeDetector or the decoder fallback) plus a scan-from-image option, full-screen RO/EN feedback, 30-second sync, header with last sync; verify `apps/web/tests/e2e/door-scanning.spec.ts` (prepare, offline reload, valid scan in airplane mode, rejected screenshot, Romanian reason).

## 6. Trip wallet and fan account

- [ ] 6.1 Implement `GET /v1/me/trip` (all organisers, holder from the JWT, product type, dates, personalisation, check-in and plan state); verify `services/internal/tickets/trip_test.go` (reissued ticket moves between wallets).
- [ ] 6.2 Implement the timeline grouping and the next-action rule list in TypeScript; verify `apps/web/tests/unit/trip-timeline.test.ts` and `apps/web/tests/unit/trip-next-action.test.ts`.
- [ ] 6.3 Build `/[locale]/wallet` with brand-styled event groups, the header action, the "Issuing" state with auto-refresh and the empty state, in RO and EN; verify `apps/web/tests/e2e/trip-wallet.spec.ts`.
- [ ] 6.4 Build `/[locale]/account` with orders, tickets and instalment schedules with Pay now; verify `apps/web/tests/e2e/fan-account.spec.ts` (claimed guest order, pay next instalment).

## 7. Thin fan actions

- [ ] 7.1 Implement personalisation with deadline and the "Name this ticket" gate; verify `services/internal/tickets/personalise_test.go` and a manual check on the preview.
- [ ] 7.2 Implement online check-in and the paid late check-in through the step 4 payment flow; verify `services/internal/tickets/checkin_test.go`.
- [ ] 7.3 Implement pick-the-day-later as a reissue for the chosen day; verify by manual check on the preview (stock for the day decreases by one).
- [ ] 7.4 Implement the signed guest link `/t/<order>.<signature>` and the printable ticket without a static code; verify `apps/web/tests/unit/signed-link.test.ts` and the guest case in `tickets.spec.ts`.
- [ ] 7.5 Seed NIBIRU sample tickets (pass, camping, table, padel slot, bus, flexible day ticket, one overdue plan) labelled as sample data; verify the trip wallet shows them on a fresh preview database.

## 8. Ship (CLAUDE.md loop)

- [ ] 8.1 Run the step's Go tests and web unit tests, then the full suite once; verify all pass locally.
- [ ] 8.2 Open a PR from `step-7-tickets-and-wallet` to `main`; verify CI is green and the Vercel preview, Neon branch and DOKS preview images are deployed.
- [ ] 8.3 Run the three adversarial reviewers in parallel (correctness: windows, skew, reissue races, redelivery, offline sync; security and data: key handling, holder checks, organiser isolation of door-sync, personal data in codes; spec and scale: every scenario tested, issuing lag at 100k fans, sync load from many doors); verify each report is in the PR.
- [ ] 8.4 Fix every confirmed finding or record in the PR why it is rejected, and re-run the affected tests; verify the PR lists each finding with its outcome.
- [ ] 8.5 Run Playwright against the deployed preview URL on desktop and phone, RO and EN, no console errors and no horizontal scroll: the trip wallet, the live ticket, and on a phone viewport the door page admitting a rotating QR and rejecting a screenshot of it taken a minute earlier; plus API checks of keys, codes and door-sync against the deployed Go core; verify the run passes and its report is linked in the PR.
- [ ] 8.6 Squash-merge when CI is green, findings are fixed and the deployed tests pass; verify the merge commit on `main`.
- [ ] 8.7 Run the Playwright smoke test against production (open the wallet, show a live code, verify a scan on the door page); verify it passes.

## Workflow follow-up

- Archive with `/opsx:archive`: merge the deltas into `openspec/specs/`, add `## Status` to proposal.md (what was built, deviations, checks with results) and tick step 7 in `docs/planning/build-plan.md`.
- Verify the archived specs validate with `npx openspec validate --all --strict`.
