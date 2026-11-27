# Tasks

## 1. Branch, contract and schema

- [ ] 1.1 Create branch `step-5-onsale-engine` from `main` and verify step 4 is merged on it
- [ ] 1.2 Add the sale settings, queue (join, status), pass and OTP paths to `api/openapi.yaml`; verify the Go stubs and TypeScript types regenerate and compile
- [ ] 1.3 Add sales-schema migrations (sales, sale_settings versions, queue_entries, cap_ledger, otp_challenges, inventory_adjustments) with `organiser_id` and row-level security; verify they apply on a testcontainers Postgres and an RLS test shows organiser B reads none of A's rows
- [ ] 1.4 Add the Ed25519 pass signing key to CI secret generation and the Helm chart (private key to the gateway only, public key to the core); verify `helm template` mounts them as designed

## 2. Sale settings

- [ ] 2.1 Implement sale moments and versioned settings with range validation, locked structural fields, live fields cached in Redis and organiser checks; verify `services/internal/sales/settings_test.go`
- [ ] 2.2 Build the Studio sale settings screen with the live rate dial, active limit and pause; verify `apps/web/tests/e2e/sale-settings.spec.ts`

## 3. Sharded inventory

- [ ] 3.1 Implement shard allocation and the per-shard Lua scripts (reserve, extend, commit, release, sweep, snapshot) behind step 4's inventory interface; verify `services/internal/inventory/shards_test.go`, `reserve_test.go` and `commit_test.go`
- [ ] 3.2 Implement multi-pool reserve with compensation and the shard expiry sweeper; verify `reserve_race_test.go` (20,000 concurrent holds on 5,000) and `expiry_test.go` (crash after a partial hold)
- [ ] 3.3 Switch the core to the sharded engine and re-run step 4's hold and checkout tests unchanged; verify `services/internal/checkout/...` passes
- [ ] 3.4 Implement held-versus-sold-out status for the shop; verify `soldout_test.go`
- [ ] 3.5 Implement the reconciler (two Postgres reads, two-round confirmation, repairs, freeze and alert events); verify `reconciler_test.go`
- [ ] 3.6 Write the k6 inventory script; verify `load/inventory.js` reaches 1,000 holds per second with p95 under 50 ms against a local core and reports no oversell

## 4. Waiting room

- [ ] 4.1 Implement join (proof of work check, session binding, one place per account or session, pre-queue window, queue-off immediate pass) and rate limits; verify `services/internal/queue/join_test.go`
- [ ] 4.2 Implement the shuffle job (lock, crypto seed stored, batched writes, latecomers behind); verify `shuffle_test.go` (uniformity over 200 runs, latecomer position)
- [ ] 4.3 Implement status with position, estimate and `next_poll_ms`; verify `status_test.go`
- [ ] 4.4 Implement the admitter (lease lock, token bucket, active limit, pause) and pass issue, revoke on paid or cancelled; verify `admitter_test.go` and `pass_test.go`
- [ ] 4.5 Implement the checkout gate in the core (signature, expiry, sale, session, Redis state) in the order of design D-11; verify `gate_test.go`
- [ ] 4.6 Build the waiting room page (static shell, proof of work, jittered polling, pre-queue countdown, position and estimate, pause message, RO and EN); verify `apps/web/tests/e2e/waiting-room.spec.ts`
- [ ] 4.7 Write the k6 queue script; verify `load/queue.js` meets p95 under 200 ms and errors under 0.1% at the stated rate against one gateway instance

## 5. Bot defence

- [ ] 5.1 Implement signal collection and scoring with the design weights; verify `services/internal/botdefence/score_test.go`
- [ ] 5.2 Implement the post-shuffle penalty and the slow lane, with no visible difference for suspects; verify `penalty_test.go`
- [ ] 5.3 Implement caps per person (identity normalisation, Lua check at hold, Postgres check on paid) and the per-order cap; verify `caps_test.go`
- [ ] 5.4 Implement risk-based OTP (trigger rule, code life, tries, resends, cancel on exhaustion) and its checkout step with the simulated SMS panel in RO and EN; verify `otp_test.go` and `apps/web/tests/e2e/otp.spec.ts`
- [ ] 5.5 Write the crowd test with human-like fans and scripted bots; verify `crowd_test.go` (under 1% fans flagged, over 90% bots flagged)

## 6. Metrics for step 6

- [ ] 6.1 Expose Prometheus metrics for queue length, pre-queue size, admitted, active sessions, suspects, OTP outcomes, pass rejections, holds, stock left and reconciler corrections; verify a test scrapes `/metrics` and finds each series

## 7. Local checks

- [ ] 7.1 Run the step's Go tests (`go test ./internal/sales/... ./internal/queue/... ./internal/botdefence/... ./internal/inventory/... ./internal/checkout/...`) and its Playwright specs; verify all pass
- [ ] 7.2 Run the full suite once and `npx openspec validate step-5-onsale-engine --strict`; verify both pass

## 8. Review and release loop (CLAUDE.md)

- [ ] 8.1 Open a PR from `step-5-onsale-engine` to `main`; verify CI is green and the preview (Vercel + Neon branch + per-PR DOKS namespace) is deployed
- [ ] 8.2 Run the three adversarial reviewers in parallel (correctness: shuffle fairness, shard races, expiry, cap races; security and data: pass forgery and replay, organiser isolation of keys, abuse paths, real fans not blocked; spec and scale: every scenario tested, opening spike, Redis load, operability); verify each report is attached to the PR
- [ ] 8.3 Fix every confirmed finding or record in the PR why it is rejected, then re-run the affected tests; verify CI is green again
- [ ] 8.4 Run Playwright against the deployed preview URL (desktop and phone, RO and EN, no console errors, no horizontal scroll) for sale settings, waiting room and OTP, plus API checks against the deployed gateway and core (pass required, forged pass refused, rate dial live) and both k6 scripts at reduced rate; verify all pass
- [ ] 8.5 Squash-merge to `main`; verify the merge commit is on `main`
- [ ] 8.6 Run the Playwright production smoke test (join a queued sale, get admitted, buy with a pass); verify it passes on production

## Workflow follow-up

- Archive with `/opsx:archive`: merge the deltas into `openspec/specs/`, add `## Status` to the proposal (what was built, deviations, checks with results) and tick step 5 in `docs/planning/build-plan.md`.
- Verify the archived specs validate with `npx openspec validate --all --strict`.
