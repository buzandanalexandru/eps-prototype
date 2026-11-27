# Tasks

## 1. Branch, contract and schema

- [ ] 1.1 Create branch `step-6-simulator-and-dashboard` from `main` and verify steps 4 and 5 are merged on it
- [ ] 1.2 Add the rehearsal, simulation run, live stream and degrade paths to `api/openapi.yaml`; verify the Go stubs and TypeScript types regenerate and compile
- [ ] 1.3 Add migrations for `simulation_runs`, the `rehearsal` flag and `sim_run_id` on sales and orders, and `degrade_state`, with row-level security; verify they apply on a testcontainers Postgres and an RLS test shows organiser B reads none of A's runs

## 2. Observability

- [ ] 2.1 Add Prometheus metrics, `/healthz` and `/readyz` and JSON logs with request ids to gateway, core, worker, fake providers and simulator, using the series in design D-5; verify `services/internal/metrics/metrics_test.go` (series present, no personal data) and `health_test.go`
- [ ] 2.2 Add the Prometheus server to the Helm chart (2 s scrape for on-sale pods, 6 h retention, scrape annotations); verify `helm template` renders it and, on a local kind cluster or the preview, every pod is an up target

## 3. Rehearsal sales and run lifecycle

- [ ] 3.1 Implement "Create rehearsal" and "Reset rehearsal", the rehearsal flag on orders and its exclusion from reports and real caps; verify `services/internal/simulation/rehearsal_test.go`
- [ ] 3.2 Implement run creation with knob validation, the `sim` run token, the Job launcher (Indexed Job from a fixed template, scoped service account), run states, one active run per sale and organiser, and Stop; verify `run_test.go` with a fake Kubernetes client
- [ ] 3.3 Add the simulator Job template and its RBAC to the chart; verify `helm template` shows the service account can only manage `app=eps-simulator` Jobs
- [ ] 3.4 Implement scheduled provider outage or failure-rate changes through the fake providers' knobs during a run; verify a test sees the knob change at the planned time and revert after its length

## 4. Simulator

- [ ] 4.1 Implement the virtual fan state machine over real HTTP (join with proof of work, jittered polling, pass, hold, OTP, payment by method, abandonment, status follow-up), synthetic client addresses and sharding of fans across pod indexes; verify `services/internal/simulation/fan_test.go` against a local stack
- [ ] 4.2 Implement the bot strategies (refresh spammer, account farm, headless, queue skipper, cap breaker) shared with step 5's crowd test; verify `bots_test.go`
- [ ] 4.3 Implement simulator metrics and the run summary written at the end; verify the summary fields in `bots_test.go`
- [ ] 4.4 Honour the simulated client address and `sim` token only on rehearsal sales in the gateway and core; verify a test that the header is ignored on a real sale

## 5. Aggregator and live stream

- [ ] 5.1 Implement the worker `aggregator` role (lease per sale, exact values from Redis and Postgres, rates from PromQL, timeline from the streams, simulator truth, 900-frame history) publishing frames on pub/sub; verify `services/internal/live/aggregator_test.go`
- [ ] 5.2 Implement the gateway SSE endpoint (organiser check, CORS for Studio, heartbeat, latest frame on reconnect); verify `stream_test.go` (30 frames in 60 s, other organiser refused)

## 6. Studio screens

- [ ] 6.1 Build the simulator panel (rehearsal create and reset, knobs with defaults, start, stop, state, summary); verify `apps/web/tests/e2e/simulator-panel.spec.ts`
- [ ] 6.2 Build the live dashboard (tiles, sparklines, provider lights with failover markers, stock bars, bots panel with truth, timeline, stale badge, rate dial, pause, degrade switch); verify `apps/web/tests/e2e/onsale-dashboard.spec.ts`
- [ ] 6.3 Write the end-to-end run test (small run of 2,000 fans with an A outage: failover on the dashboard, summary afterwards); verify `apps/web/tests/e2e/simulator-run.spec.ts` on the local stack

## 7. Graceful degradation

- [ ] 7.1 Implement the degrade state in the core and gateway (`EPS-BUSY` for new holds, held orders still payable, queue untouched) and the dashboard suggestion rule; verify `services/internal/degrade/degrade_test.go`
- [ ] 7.2 Implement the storefront's static event-page variant and busy message in RO and EN, revalidated when the switch changes; verify `apps/web/tests/e2e/degrade.spec.ts`

## 8. Local checks

- [ ] 8.1 Run the step's Go tests (`go test ./internal/metrics/... ./internal/simulation/... ./internal/live/... ./internal/degrade/...`) and its Playwright specs; verify all pass
- [ ] 8.2 Run the full suite once and `npx openspec validate step-6-simulator-and-dashboard --strict`; verify both pass

## 9. Review and release loop (CLAUDE.md)

- [ ] 9.1 Open a PR from `step-6-simulator-and-dashboard` to `main`; verify CI is green and the preview (Vercel + Neon branch + per-PR DOKS namespace with Prometheus) is deployed
- [ ] 9.2 Run the three adversarial reviewers in parallel (correctness: run lifecycle, stop, frame accuracy, reconnect; security and data: sim token scope, simulated address trust, RBAC of the Job launcher, rehearsal isolation, no personal data in metrics; spec and scale: every scenario tested, a 100k run on the cluster, resource limits, operability); verify each report is attached to the PR
- [ ] 9.3 Fix every confirmed finding or record in the PR why it is rejected, then re-run the affected tests; verify CI is green again
- [ ] 9.4 On the deployed preview run a 10,000-fan rehearsal with an A outage, then Playwright (desktop and phone, RO and EN where text changed, no console errors, no horizontal scroll) on the simulator panel, dashboard and degrade mode, plus API checks (run refused on a real sale, stream refused for another organiser); verify all pass
- [ ] 9.5 Squash-merge to `main`; verify the merge commit is on `main`
- [ ] 9.6 Run the production smoke test: a 100,000-fan rehearsal with 10% bots and an A outage, with Playwright checking the dashboard updates, shows the failover and ends with a summary; verify it passes and record the run's numbers for the video

## Workflow follow-up

- Archive with `/opsx:archive`: merge the deltas into `openspec/specs/`, add `## Status` to the proposal (what was built, deviations, checks with results and the 100k run's numbers) and tick step 6 in `docs/planning/build-plan.md`.
- Verify the archived specs validate with `npx openspec validate --all --strict`.
