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
