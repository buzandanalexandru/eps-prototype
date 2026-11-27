# Design: step 12, scale proof

## Context

See `proposal.md` for why. From step 0 there is a Helm umbrella chart `deploy/helm/eps` with one sub-chart per Go service and Redis, deployed by GitHub Actions to DOKS fra1; step 6 added Prometheus metrics, the SSE aggregator and the simulator Job (R2, R3); step 5 added the gateway's pre-queue, shuffle, signed passes and admission rate (O1) and step 4 the payment state machine with failover (O3) and sharded stock (O2).

## Goals / Non-Goals

**Goals:**
- Per-instance throughput numbers that someone else could reproduce from the repo.
- Maths that turns those numbers into replica counts for 100k fans in 10 minutes, with every assumption visible.
- A chart that a fresh cluster can install with one command.

**Non-Goals:**
- A real 100k-client test, multi-region, Redis Cluster, Kafka or NATS (all named in the vision as the scale-up path), Argo CD, a Grafana stack.

## Decisions

### D1 · Autoscaling (R1)
- `autoscaling/v2` HPA for **gateway** (min 2, max 12) and **core** (min 2, max 8), two metrics each: CPU at 60% of requests, and a Pods metric `http_requests_per_second` (from the services' Prometheus counters via Prometheus Adapter) with targets set to **60% of the measured per-pod maximum** from D3 (written into values after the first run; a placeholder until then).
- Behaviour: scale up with no stabilisation, up to +100% or +4 pods per 15 s; scale down after 300 s stabilisation, at most −1 pod per 60 s (no flapping during a sale).
- Gateway and core: requests 250m CPU / 256Mi, limits 1 CPU / 512Mi; readiness on `/readyz` (Redis and Postgres reachable), liveness on `/healthz`; PodDisruptionBudget `minAvailable: 1`; topology spread across nodes. Worker 1–2 replicas fixed, fakepsp 1 per provider, simulator as a Job, Redis one StatefulSet (named as the single-node limit).
- DOKS node pool autoscaling 2–5 nodes of the cheapest size that fits; the expected maximum cost is stated on the scale page.
- Pre-scaling before an on-sale is a values change (`minReplicas`) applied by CI from the sale settings' opening time, documented as the runbook step; HPA alone reacts in 30–60 s, too slow for a T0 spike.

### D2 · k6 scenarios
All in `load/`, sharing a helper for signed passes and idempotency keys:
- `queue-join.js`: `ramping-arrival-rate` of joins plus position polls every 20 s against the gateway, stepped 250 → 4,000 req/s in 1-minute steps.
- `checkout.js`: virtual fans join with the admission rate at maximum, get a pass, take a hold and pay through fake provider A at 0% failure and 0 ms latency (the core's own limit), stepped like above in orders per second.
- `failover.js`: steady 50 orders/s; at minute 2 provider A goes to 100% failure; checks no order is charged twice (charges equal paid orders, read from the core API) and p95 during the switch.
- `autoscale.js`: HPA on, ramps load to 3× one pod's maximum and records replica counts each 15 s from the Kubernetes API.
- `smoke.js`: 1 minute at low rate, used in CI after deploy.
Thresholds: p95 < 300 ms (gateway < 150 ms), errors < 1%, zero double charges, zero oversold units (stock check after the run).

### D3 · How results are measured and recorded
- k6 runs as a Kubernetes Job (`load/k8s/k6-job.yaml`) on a node with no gateway or core pods (anti-affinity), so the laptop's network is not measured. For per-instance runs the chart is installed with HPA off and 1 replica of the service under test.
- **Per-instance maximum** = the highest step whose thresholds all pass.
- Each run writes `load/results/<date>-<scenario>.json`: k6's summary export plus metadata (commit SHA, chart version, replicas, node size, provider knobs, step that passed, Redis ops/s and Postgres write rate from Prometheus at that step). Results are committed by hand from the workflow's artefact; a script builds `apps/web/content/scale-results.json` from them. Numbers on the page are never typed by hand.

### D4 · The scaling maths
Shown on `/scale`, computed by one tested function from the results file:
- Orders: 100,000 fans in 600 s = **167 orders/s** average; planning peak ×2 = 333 orders/s.
- Core requests: requests per order measured by `checkout.js` (hold, pay, status polls); core replicas = ceil(peak orders/s × requests per order ÷ (per-pod max × 0.6)).
- Gateway: all 100k waiting and polling every 20 s = 5,000 req/s, plus the T0 join burst (100k joins over the first 30 s ≈ 3,333 req/s); gateway replicas = ceil(peak req/s ÷ (per-pod max × 0.6)).
- Admission: the rate dial is set to the core's safe orders/s, so the queue absorbs the rest; time to clear = fans ÷ admission rate (shown in minutes).
- Shared limits: Redis ops/s at the measured step versus a single Redis node's measured ceiling; Postgres paid-order writes per second; payment providers (two, failover). The page states which of these binds first and the next step for each (Redis Cluster, a write queue, more providers).
- Bots: the share of traffic from bots (from the simulator settings) multiplies the gateway load only, because silent scoring sends them to the back before checkout.

### D5 · The honest load story
Every number on `/scale` carries one label: **Measured** (k6, with date, commit and setup), **Simulated** (the in-system simulator, which drives real HTTP through the real services at the size the cluster allows) or **Calculated** (the maths). A fixed sentence says: "We did not run 100,000 real clients; we measured one instance and calculated the rest." The video and vision use the same labels and numbers.

### D6 · Roadmap page
`/roadmap` (RO and EN) has three static sections from one content file: **Next** (G6 refunds and cancellation, with why it waits for the payment flow; plus NATS or Kafka, Redis Cluster, real providers behind the adapter), **Cut on purpose** (B8, B9, D10, F9, G7, H4 with the reason "cut in review") and **Out of scope** (J1–J4). No item links to a screen.

### D7 · Test roll-up (R4)
`apps/web/tests/unit/spec-coverage.test.ts` parses every `openspec/specs/*/spec.md` (and in-flight changes), checks each scenario ends with a `Test:` line whose path exists (or is a `manual check on the deployed preview (...)`), and fails listing the missing ones. A CI step writes counts (scenarios, automated, manual, tests passing per suite from the CI reports) to `apps/web/content/test-rollup.json`, shown on `/scale`.

## Risks / Trade-offs

- [Per-instance numbers are low on small DOKS nodes] → they are what they are; the page shows the node size and the maths scales with it. Honest beats impressive.
- [Measuring a shared Redis with one pod hides Redis limits] → record Redis ops/s at each step and show the ceiling separately.
- [Prometheus Adapter misconfigured, HPA stuck on CPU] → the CPU metric still scales; `autoscale.js` checks replicas actually grow.
- [Load test cost and cluster stability] → load runs only by hand, in a separate namespace, and the chart is restored to normal values after.

## Migration Plan

Chart changes ship through the normal CI `helm upgrade`; rollback is `helm rollback eps <revision>`. Load tests run in namespace `eps-load` with their own release and are deleted afterwards.
