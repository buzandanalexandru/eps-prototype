# Tasks

## 1. Chart and autoscaling (I2, R1)

- [ ] 1.1 Finish the sub-chart values for gateway, core, worker, fakepsp, simulator and Redis: resources, `/readyz` and `/healthz` probes, PDB `minAvailable: 1` for gateway and core, topology spread; verify `helm lint` and `helm template` pass in CI
- [ ] 1.2 Install Prometheus Adapter (pinned chart version) exposing `http_requests_per_second` per pod; verify `kubectl get --raw` on the custom metrics API returns the metric for gateway and core
- [ ] 1.3 Add `autoscaling/v2` HPAs (gateway 2–12, core 2–8, CPU 60% and requests per second, scale-up and scale-down behaviour) with placeholder rps targets; verify `kubectl describe hpa` shows both metrics reading values on the preview
- [ ] 1.4 Enable DOKS node pool autoscaling (2–5 nodes) and write the pre-scaling runbook step (minReplicas raised before opening); verify the node pool settings with the DigitalOcean API and the runbook in `deploy/helm/eps/README.md`
- [ ] 1.5 Fresh-install check into an empty namespace and a node drain during `load/smoke.js`; verify all pods ready and errors under 1%, recorded in the PR

## 2. k6 scenarios and measurement (I3)

- [ ] 2.1 Write the shared k6 helper (queue join, pass handling, idempotency keys) and `load/smoke.js`; verify smoke passes against the preview and runs as a CI post-deploy step
- [ ] 2.2 Write `load/queue-join.js` and `load/checkout.js` with stepped rates and thresholds, plus the in-cluster Job manifest `load/k8s/k6-job.yaml` with anti-affinity; verify a short dry run completes in namespace `eps-load`
- [ ] 2.3 Write `load/failover.js` (provider A to 100% failure at minute 2, charges equal paid orders, zero oversold) and `load/autoscale.js` (records replicas every 15 s); verify both pass on the preview
- [ ] 2.4 Add the manual GitHub Actions workflow that installs `eps-load` with HPA off and 1 replica, runs a scenario, uploads the summary and metadata as an artefact and tears down; verify one run end to end
- [ ] 2.5 Run the four scenarios for real, commit the results to `load/results/` with metadata, then set the HPA rps targets to 60% of the measured per-pod maxima; verify the result files and the updated values

## 3. Scale page and maths

- [ ] 3.1 Script that builds `apps/web/content/scale-results.json` from `load/results/`; verify `apps/web/tests/unit/scale-results.test.ts` (page numbers equal stored values)
- [ ] 3.2 The scaling maths function (orders per second, pods needed with 0.6 headroom, queue clearing time, binding shared limit, bot share on the gateway only); verify `apps/web/tests/unit/scale-maths.test.ts` with the spec's worked examples
- [ ] 3.3 `/scale` page in RO and EN: method statement, Measured / Simulated / Calculated labels, setup and date per result, the maths with inputs, the binding limit and the scale-up path, cluster cost; verify `apps/web/tests/e2e/scale-page.spec.ts`

## 4. Roadmap page (G6 as vision)

- [ ] 4.1 `/roadmap` page in RO and EN from one content file: Next (G6 refunds and cancellation, NATS or Kafka, Redis Cluster, real providers), Cut on purpose (B8, B9, D10, F9, G7, H4), Out of scope (J1–J4), no screen links; verify `apps/web/tests/e2e/roadmap.spec.ts`

## 5. Test roll-up (R4)

- [ ] 5.1 `apps/web/tests/unit/spec-coverage.test.ts` checking every scenario's `Test:` line in `openspec/specs/` and in-flight changes; verify it fails on a planted missing path and passes on the real tree
- [ ] 5.2 CI step writing `apps/web/content/test-rollup.json` (scenarios, automated, manual, passing suites) and the roll-up block on `/scale`; verify the roll-up scenario in `apps/web/tests/e2e/scale-page.spec.ts`
- [ ] 5.3 Fill any scenario test gaps the check finds in earlier steps' specs by adding the missing tests (not by editing specs); verify the coverage check is green on `main`

## 6. Delivery loop (CLAUDE.md)

- [ ] 6.1 Run the step's Vitest and Playwright tests, `helm lint` and `load/smoke.js` locally, then the full suite once; verify all green
- [ ] 6.2 Open the PR `step-12-scale-proof` to `main`; verify CI is green and the preview (Vercel and DOKS) is deployed with the new chart
- [ ] 6.3 Run the three adversarial reviewers in parallel (correctness: k6 thresholds, maths, result provenance; security and data: load mode not exposed in production, secrets in the chart, namespace isolation; spec and scale: every scenario tested, HPA behaviour, binding limits, honesty of every claim on `/scale`); verify each returns a ranked findings list
- [ ] 6.4 Fix every confirmed finding or record the rejection in the PR, then re-run the affected tests and, if a number changed, the affected k6 scenario; verify green
- [ ] 6.5 Run Playwright against the deployed preview (`/scale` and `/roadmap`, desktop and phone, RO and EN, no console errors, no horizontal scroll) plus `load/smoke.js` against the deployed gateway and core; verify all pass
- [ ] 6.6 Squash-merge once CI, reviews and deployed tests are green; verify production with the Playwright smoke test and `kubectl get hpa` on the production namespace

## Workflow follow-up

- Archive with `/opsx:archive step-12-scale-proof`: merge the delta into `openspec/specs/`, add `## Status` to the proposal (with the measured numbers), tick step 12 in `docs/planning/build-plan.md`.
