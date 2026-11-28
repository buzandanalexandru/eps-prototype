# Proposal

Step 6: Simulator and dashboard

**Demo act:** Act 2 (the live on-sale), completing it: the organiser presses "Start" in Studio on a rehearsal of a published sale, a bounded crowd of simulated fans and a share of bots hit the real waiting room and checkout, provider A goes down mid-sale, and the live dashboard shows the queue, sales, failovers, bots caught and stock left as they happen. The rehearsal labels itself as simulated and shows its measured numbers; the 100,000-fan figure appears only as arithmetic with its assumptions next to them.

**Features covered:** E7 on-sale simulator, E8 live on-sale dashboard, E11 graceful degradation (Thin), I4 basic monitoring.
**Decisions covered:** R2 metrics and the live dashboard, R3 the simulator sends real traffic. Builds on O1–O4 (queue, stock, payments, fake providers) and P3 (event streams).

## Why

Act 2's claim is that the platform handles a crowd, bots and a failing provider. That claim is only honest if real HTTP traffic goes through the real gateway and checkout, the dashboard reads the system's own numbers rather than animated counters (R3), and the run's size is what was actually sent (no claim of a real 100k-client test; step 12 measures throughput with k6). It also gives the organiser what Ticketmaster's TM1 Demand gives (live on-sale performance) and the rehearsal that festival-tech pattern 9 calls for, plus an honest busy mode instead of a crash (pattern 3, static fallbacks).

## What Changes

- **Rehearsal sales:** a copy of a published sale moment under its own event id, with the source's prices and settings, its own stock, caps, queue, payment merchant and breaker, labelled "Rehearsal: simulated fans"; reset makes a fresh one; its Redis data is cleaned up automatically.
- **A simulator control panel** in Studio: knobs for fan count (default 1,000, capped per environment: 2,000 on previews, 10,000 in production), bot share, share arriving before opening, arrival pattern, quantity and payment mixes, abandonment, decline rate, polling floor, a timed outage or failure rate on provider A, and duration (default 5, at most 10 minutes); Start, Stop, state and summary; an honest load panel with the run's measured numbers, step 5's k6 numbers and the arithmetic for 100,000 fans.
- **A simulator service** that drives virtual fans and bots as real HTTP clients through the gateway, core and the providers' hosted pages, with its own rehearsal-only token issuer; one run at a time per environment, bounded in-flight requests, no database or Redis access.
- **A live on-sale dashboard** for the organiser: queue, admitted per minute, shopping, sold of capacity, revenue, payment success and failures, failovers with provider status, bots caught (against the simulator's truth during a run), stock per pool, timeline, system strip, the admission dial, pause and the degrade switch, updated every second over server-sent events straight from the gateway.
- **Observability:** added metrics on every Go service, a Prometheus server in the cluster (2 s scrape for the on-sale services, 6 h retention) and an aggregator in the worker that builds one frame per watched sale per second from exact Redis and Postgres values.
- **Graceful degradation:** a switch per sale: new holds refused with an honest `EPS-BUSY`, admissions held while the queue keeps everyone's place, held orders still payable, and the static event page showing a busy message in place of its buy controls.

## Capabilities

### New Capabilities
- `onsale-simulator`: rehearsal sales, the control panel and its knobs, the run lifecycle, and the behaviour of virtual fans and bots.
- `onsale-dashboard`: the live numbers, their freshness and the controls on the dashboard screen.
- `observability`: the metrics each service exposes, their collection, health and structured logs.
- `graceful-degradation`: the switch, static event pages and the honest busy message (Thin).

### Modified Capabilities
None.

## Impact

- **Go:** `cmd/simulator` implemented (claim loop, fan engine, bot strategies, reports); `cmd/core` gains rehearsal, simulation and degrade operations, an internal port for the simulator, the rehearsal snapshot source, the sim-token rules and the merchant per rehearsal; `cmd/worker` gains the aggregator and the rehearsal clean-up; `cmd/gateway` gains the live stream and the degrade rule in the admitter; added metrics.
- **Sales schema (migration 00004):** rehearsal and degrade columns on sales, `rehearsal` and `sim_run_id` on orders, `merchant_event` on payment attempts, `simulation_runs`, three SECURITY DEFINER functions owned by `eps_sales_index`.
- **Redis:** live frames (pub/sub and a 900-frame list per sale), the watched-sales set, run reports.
- **API contract:** rehearsal, simulation, degrade and live-stream operations; `degraded` and `rehearsal` on the queue status, availability and order views.
- **Next.js:** Studio simulator panel and live dashboard; links from the sale moments page; busy banner on the event page, waiting room and checkout; rehearsal label in the waiting room and checkout; RO and EN.
- **Helm:** Prometheus (namespace Role), the simulator as a Deployment, the internal port, `SIM_CONTROL_TOKEN` and the `SIM_TOKEN_*` pair from `eps-runtime`, `SIM_MAX_FANS` per environment.
- **No new libraries:** the standard-library metrics registry stays; Prometheus is a server image only; charts are plain SVG.

## Status

Done. Merged as `65acb45` (PR #8, squash).

**What was built:**
- **Rehearsal copies of a sale:** their own event id, stock, caps, queue, payment merchant (`rh-<event>`) and breaker. They never touch real money: no wallet, saved cards or instalments, and rehearsal orders are kept out of the real ledger.
- **A bounded simulator** (a one-replica Deployment):
  - drives simulated fans and five bot strategies through the real waiting room, checkout and simulated providers;
  - capped at 5,000 clients in production and 2,000 on previews, 1,500 requests a second, 10 minutes;
  - one run per cluster, 3 runs an hour per organiser, refused while any real sale has fans waiting;
  - stops itself if it loses the core, and closes its rehearsal after the run.
- **Ground-truth summaries** from the system's own records.
- **A live dashboard** streamed over SSE from the gateway, with exact per-second frames.
- **Observability:** Prometheus in the chart, new series and latency histograms, and NetworkPolicies on the internal ports.
- **Degraded mode:** new holds are told the system is busy, held orders still pay, every place is kept and pass expiry pauses. The fan-facing copy is honest in RO and EN.
- **Vercel skips builds that do not touch the web app**, and CI falls back to the branch's last ready deployment.

**Deviations (design, including "Review decisions"):**
- The simulator is a Deployment, not a Job.
- The busy message reaches the shop through the availability answer.
- Wallet and saved cards are left out of the payment mix.
- Run success is counted per order.
- The 100k figure is arithmetic from measured rates, not a run. The owner confirmed this on 28 Nov ("cap + honest maths"), and feature-scope E7 and Act 2 and architecture R3 were updated.
- The waiting-room "Opening spike" and inventory "Throughput" targets move to step 12.
- Rejected: tracking clean-up keys in a set, which would put a Redis write on the hot path. A bounded, resumable SCAN replaces it.

**Checks run:**
- Locally:
  - Go: vet clean, 24 packages pass.
  - Vitest: 328 tests.
  - Playwright: 109 passed, 27 skipped.
  - tsc, eslint, next build, helm, actionlint, contract drift, `openspec validate --all --strict` (25), and the Vercel ignore and node tests are clean.
- Three adversarial reviews, run in parallel:
  - correctness: 1 high, 4 medium and 7 low;
  - security and data: 3 medium and 7 low;
  - spec, scale and honesty: 1 high, 10 medium and 12 low.

  All findings were fixed or recorded. The PR body's "Review findings" table maps each to its commit.
- PR CI:
  - The first e2e run was blocked because the `VERCEL_TOKEN` secret was missing. The owner restored it.
  - The next deployed e2e failed: the preview core's 3-connection pool was starved during the outage, and the simulator panel showed "no rehearsal" on a slow read. Both are fixed: 8 connections on previews, and honest retry messages.
  - All 17 checks pass on `b338080`; deployed e2e had 105 passed and 31 skipped.
- Independent Playwright run on the preview: the simulator panel and live dashboard, EN and RO at 1440 px and 390 px. No console errors, no horizontal scroll, simulated and arithmetic labels present.
- Production:
  - Deploy run 14's first attempt failed in `migrate`: sales migration 4 deadlocked with the running worker. Goose rolled it back, and production kept serving step 5. The re-run of the failed jobs succeeded.
  - Follow-up for later migrations: take table locks up front, in a fixed order, with a `lock_timeout`, and retry in the deploy.
- Production rehearsal smoke on Promenada 2027 "General sale", run by the demo organiser at the default size: 1,000 clients (900 fans, 100 bots, 10%), with provider A down for 45 s from opening. The run ended `done`. Measured numbers for the video:
  - 2,072 tickets sold, not sold out;
  - 97.3% of orders where Pay was pressed were paid;
  - 308 provider switches, and 188 orders paid at B during the outage;
  - 22 of 100 bots flagged; bots bought 388 tickets;
  - 0 real fans flagged or asked for a code;
  - 44 queue-jump attempts refused;
  - 48.3 requests a second on average, 147 at the peak;
  - no console errors.
