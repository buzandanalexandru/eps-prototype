# Tasks

## 1. Branch, contract and schema

- [x] 1.1 Create branch `step-6-simulator-and-dashboard` from `main` and verify steps 4 and 5 are merged on it
- [x] 1.2 Add the rehearsal, simulation run, degrade and live stream operations and the `rehearsal`/`degraded` fields to `api/openapi.yaml`; verify the Go stubs and TypeScript types regenerate and compile (`scripts/check-contract-drift.sh`)
- [x] 1.3 Add migration 00004 (rehearsal and degrade columns on sales, `rehearsal`/`sim_run_id` on orders, `merchant_event` on attempts, `simulation_runs` with row-level security, the SECURITY DEFINER functions owned by `eps_sales_index`); verify it applies on testcontainers Postgres and the RLS test shows organiser B reads none of A's runs

## 2. Observability

- [x] 2.1 Add the D-5 series to core, gateway, worker and simulator (no personal data in labels); verify `services/internal/metrics/metrics_test.go` (series present after a paid order, no email, pass or session in labels or logs) and `services/internal/health/health_test.go`
- [x] 2.2 Add the Prometheus server to the chart (2 s job for gateway, core and worker, 15 s otherwise, 6 h retention, namespace Role reading pods only); verify `services/internal/metrics/chart_test.go` and `helm lint`/`helm template`

## 3. Rehearsals and runs

- [x] 3.1 Implement rehearsal create and reset (own event id, the rehearsal snapshot source, copied settings, the merchant and breaker per rehearsal, `rehearsal`/`sim_run_id` on orders, automatic Redis clean-up); verify `services/internal/simulation/rehearsal_test.go`
- [x] 3.2 Implement the sim token issuer rules in the core and gateway (only the named rehearsal, only checkout operations); verify `run_test.go` (token refused on a real sale)
- [x] 3.3 Implement runs: knob validation and caps, one active run per sale, organiser and environment, the internal claim and report endpoints on the core's internal port, states, Stop, the lost-simulator rule and the provider A schedule; verify `services/internal/simulation/run_test.go`
- [x] 3.4 Replace the simulator Job with a Deployment in the chart, add the internal port, `SIM_CONTROL_TOKEN`, the `SIM_TOKEN_*` pair (`runtime-secret.sh`) and `SIM_MAX_FANS`; verify `helm template` and the chart rules in CI

## 4. Simulator

- [x] 4.1 Implement the claim loop and the virtual fan journey over real HTTP (join with proof of work, jittered polling, pass, hold, details, OTP, payment by method with the hosted page, abandonment, status follow-up, synthetic addresses, bounded in-flight requests, stop); verify `services/internal/simulation/fan_test.go` against a local stack
- [x] 4.2 Implement the bot strategies (refresh spammer, account farm, headless, queue skipper, cap breaker) and the truth report and summary; verify `services/internal/simulation/bots_test.go`

## 5. Aggregator and live stream

- [x] 5.1 Implement the worker aggregator (watched sales, exact values, rates from frame history, system figures from Prometheus, timeline, truth, the degrade suggestion, 900-frame history, pub/sub); verify `services/internal/live/aggregator_test.go`
- [x] 5.2 Implement the gateway's SSE endpoint (organiser check, CORS, history and latest frame, heartbeat, limits); verify `services/internal/live/stream_test.go`

## 6. Graceful degradation

- [x] 6.1 Implement the degrade switch in the core (EPS-BUSY on new holds, held orders payable), the gateway (no admissions while degraded, status flag) and the availability answer; verify `services/internal/onsale/degrade_test.go`

## 7. Studio and shop screens

- [x] 7.1 Build the simulator panel (rehearsal create, reset and waiting-room link; knobs with defaults and limits; Start, Stop, state, summary; honest load panel) with links from the sale moments page; verify `apps/web/tests/e2e/simulator-panel.spec.ts`
- [x] 7.2 Build the live dashboard (tiles, SVG sparklines with failover markers, provider lights, stock bars, bots panel with truth, timeline, system strip, stale badge, rate dial, pause, degrade switch and suggestion); verify `apps/web/tests/e2e/onsale-dashboard.spec.ts`
- [x] 7.3 Show the busy banner on the event page, waiting room and checkout and the rehearsal label in the waiting room and checkout, RO and EN; verify `apps/web/tests/e2e/degrade.spec.ts`
- [x] 7.4 Write the deployed run test (a small run with an A outage: failover on the dashboard, summary afterwards); verify `apps/web/tests/e2e/simulator-run.spec.ts`

## 8. Local checks

- [x] 8.1 Run the step's Go tests (`go test ./internal/metrics/... ./internal/simulation/... ./internal/live/... ./internal/onsale/...`) and its Playwright specs; verify all pass
- [x] 8.2 Run the full suite once (Go with testcontainers, Vitest with Postgres, Playwright against `next start`, tsc, eslint, next build, helm lint, actionlint, contract drift) and `npx openspec validate --all --strict`; verify all pass

## 9. Review and release loop (CLAUDE.md)

- [ ] 9.1 Open a PR from `step-6-simulator-and-dashboard` to `main`; verify CI is green and the preview (Vercel + Neon branch + per-PR DOKS namespace with Prometheus) is deployed, including the deployed-preview e2e job
- [ ] 9.2 Run the three adversarial reviewers in parallel (correctness: run lifecycle, stop, frame accuracy, reconnect; security and data: sim token scope, simulated address trust, rehearsal isolation, internal port, no personal data in metrics; spec and scale: every scenario tested, run bounds, resource limits, operability); verify each report is attached to the PR
- [ ] 9.3 Fix every confirmed finding or record in the PR why it is rejected, then re-run the affected tests; verify CI is green again
- [ ] 9.4 On the deployed preview run a 2,000-fan rehearsal with an A outage, then Playwright (desktop and phone, RO and EN where text changed, no console errors, no horizontal scroll) on the simulator panel, dashboard and degrade mode, plus API checks (run refused on a real sale, stream refused for another organiser); verify all pass
- [ ] 9.5 Squash-merge to `main`; verify the merge commit is on `main`
- [ ] 9.6 Run the production smoke test: a rehearsal of the sample sale with 10% bots and an A outage at the production cap, with Playwright checking the dashboard updates, shows the failover and ends with a summary; record the run's measured numbers for the video (not a 100k claim)

## Workflow follow-up

- Archive with `/opsx:archive`: merge the deltas into `openspec/specs/`, add `## Status` to the proposal (what was built, deviations, checks with results and the run's measured numbers) and tick step 6 in `docs/planning/build-plan.md`.
- Verify the archived specs validate with `npx openspec validate --all --strict`.
