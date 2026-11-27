# Proposal

Step 6: Simulator and dashboard

**Demo act:** Act 2 (the live on-sale), completing it: the organiser presses "Start" in Studio, 100,000 simulated fans and a share of bots hit the real waiting room and checkout, provider A goes down mid-sale, and the live dashboard shows the queue, sales, failovers, bots caught and stock left as they happen.

**Features covered:** E7 on-sale simulator, E8 live on-sale dashboard, E11 graceful degradation (Thin), I4 basic monitoring.
**Decisions covered:** R2 metrics and the live dashboard, R3 the simulator sends real traffic. Builds on O1–O4 (queue, stock, payments, fake providers) and P3 (event streams).

## Why

Act 2's claim is that the platform survives 100k fans, bots and a failing provider. That claim is only honest if real HTTP traffic goes through the real gateway and checkout and the dashboard reads the system's own numbers, not animated counters (R3). It also gives the organiser what Ticketmaster's TM1 Demand gives (live on-sale performance) and the rehearsal that festival-tech pattern 9 calls for, plus an honest busy mode instead of a crash (pattern 3, static fallbacks).

## What Changes

- A simulator control panel in Studio that starts a simulation run against a rehearsal sale, with knobs for fan count (default 100,000), bot share, arrival pattern, purchase mix, payment-method mix, abandonment, and a timed outage or failure rate on provider A.
- A simulator that runs as a Kubernetes Job of several pods and drives virtual fans and bots as real HTTP clients through the gateway and core: joining the pre-queue, polling, getting admitted, holding, answering OTPs, paying through the fake providers, abandoning, and (for bots) misbehaving.
- Rehearsal sales: a copy of a published sale with its own stock, clearly labelled, so simulated fans never sell real stock and their orders are kept out of real reports.
- A live on-sale dashboard for the organiser: queue length, admitted per minute, tickets sold, payment failures and failovers with provider status, bots caught, stock left per pool, plus the admission dial and the degrade switch, updated every second.
- Prometheus metrics from every Go service, a Prometheus server in the cluster, and an aggregator that turns metrics and exact Redis counts into one live stream to the dashboard over server-sent events.
- A graceful degradation switch: static event pages and an honest busy message in the shop, and the gateway turning away new checkouts politely while the queue keeps everyone's place.

## Capabilities

### New Capabilities
- `onsale-simulator`: rehearsal sales, the control panel and its knobs, the run lifecycle, and the behaviour of virtual fans and bots.
- `onsale-dashboard`: the live numbers, their freshness and the controls on the dashboard screen.
- `observability`: the metrics each service exposes, their collection, health and structured logs.
- `graceful-degradation`: the switch, static event pages and the honest busy message (Thin).

### Modified Capabilities
None.

## Impact

- **Go:** `cmd/simulator` implemented (virtual fan engine, bot behaviours, run reporting); `cmd/core` gains simulation run and degrade admin endpoints and a Kubernetes Job launcher; `cmd/worker` gains the `aggregator` role; `cmd/gateway` gains the SSE fan-out and the degrade response; all services gain Prometheus metrics.
- **Sales schema:** `simulation_runs`, `rehearsal` flag and `sim_run_id` on sales and orders, `degrade_state`.
- **Redis:** pub/sub channel per sale for live frames, run state keys.
- **API contract:** simulation, live stream and degrade paths.
- **Next.js:** Studio simulator panel, live dashboard, degrade switch; static event-page variant and busy message on storefronts; rehearsal label on the shop.
- **Helm:** Prometheus server (small, 6 h retention), simulator Job template with a service account allowed only to create and delete Jobs in its namespace, aggregator deployment, scrape annotations.
- **No new libraries** beyond the Prometheus Go client and the Prometheus server image (part of the K8s story in R2).
