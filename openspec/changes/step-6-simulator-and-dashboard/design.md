# Design

## Context

See proposal.md (Why). Steps 4 and 5 give the checkout, the fake providers with live knobs, the waiting room, passes, bot scoring, caps, OTP and sharded stock, each already exposing some Prometheus metrics and outbox events on Redis Streams (P3). Everything Go runs on DOKS fra1 in one namespace per environment; the Studio is on Vercel. The cluster is small (a demo budget), so the simulator must be tunable and must not need Vercel to carry its traffic.

## Goals / Non-Goals

**Goals:**
- Real HTTP traffic through the real gateway and core, with the same rules as human fans (R3).
- A dashboard updated every second from the system's own numbers (R2).
- Simulated traffic never touches real stock or real reports.
- An honest busy mode (E11).

**Non-Goals:**
- A claim of a real 100k-client internet test: the simulator runs in the cluster; the published throughput numbers come from k6 in step 12.
- Grafana, alerting, tracing (named in the vision).
- Load on the Vercel storefront (only a small sample of fans loads real pages).

## Decisions

### D-1 Rehearsal sales

Studio's "Create rehearsal" copies a published sale (same snapshot version, settings and pools) into a new sale with `rehearsal = true` and its own stock. The shop shows it at `/s/<brand>/<event>?sale=<id>` with a "Rehearsal: simulated fans" banner; a human can join it too (the presenter queues among the 100k in the video). Orders carry `rehearsal = true` and `sim_run_id`, are excluded from the sales dashboard (G1) and from caps on the real sales, and can be cleared with "Reset rehearsal". Simulator credentials work only on rehearsal sales.

### D-2 Simulation run lifecycle

`POST /v1/admin/simulations` (organiser JWT; Studio panel) validates knobs, writes `simulation_runs` (state `pending`), and creates an Indexed Kubernetes Job `sim-<run>` with `parallelism = completions = pods` from a fixed template (the core's service account may only create, read and delete Jobs with label `app=eps-simulator` in its namespace). States: `pending → starting → running → finishing → done | failed | stopped`. "Stop" deletes the Job (`stopped`). A run ends at `duration` or when stock is gone and every fan has finished. One run per sale at a time; at most one running run per organiser.

Knobs (defaults): `fans` 100,000 (1–200,000); `bot_share` 0.10; `pods` 10 (1–40); `prequeue_share` 0.6 (fans who arrive before opening); `arrival` `spike` (late arrivals follow an exponential decay over 10 minutes) or `flat`; `opens_in` 2 minutes (the run moves the rehearsal's opening time); `qty_mix` 1:35%, 2:40%, 3:10%, 4:15%; `product_mix` weights from the snapshot's products; `payment_mix` card 55%, wallet 15%, Apple Pay 10%, Google Pay 10%, saved card 10%; `abandon_rate` 0.08; `decline_rate` 0.03 (set on both providers); `poll_interval_s` 10; `provider_a_outage` `{at: "+3m", for: "90s"}` or `provider_a_failure_rate`; `duration` 15 minutes. Outage and failure knobs are applied by the core calling the fake providers' admin knobs at the planned times (step 4), so the failover is real.

### D-3 How virtual fans work

Each pod runs `fans / pods` virtual fans as goroutines (10,000 per pod at default), each a state machine over real HTTP/1.1 keep-alive connections to the gateway's and core's in-cluster services, with the same API the browser uses:
`arrive → join (proof of work solved for real) → poll status (interval ± jitter) → admitted (pass) → hold (quantity from the mix) → OTP if asked (reads the simulated code from the response, as a person reads the screen; 2% type it wrong once) → pay (method from the mix; card payments drive the fake provider's hosted-page outcome endpoint instead of a browser) → follow order status → done`, with abandonment at hold or payment.
- **Identity:** each fan is a guest with a generated email `fan-<run>-<n>@sim.invalid`; wallet fans get a simulated balance topped up before opening through the normal top-up path.
- **Authentication:** the core issues a run-scoped token (Ed25519, kind `sim`, run id, sale id, 1-hour life) passed to the Job as a secret; the gateway and core accept guest sessions under it only for that rehearsal sale.
- **Client addresses:** each fan has a synthetic client IP (residential-looking spread for fans; a few /24 ranges and the configured data-centre list for bots) sent in `X-EPS-Sim-Client`, honoured only with a valid `sim` token on a rehearsal sale. Bot scoring therefore sees realistic address signals.
- **Ground truth:** the simulator knows which clients are bots; the gateway does not. The simulator reports its own metrics (fans by state, bots by state, bots admitted, bot purchases) so the dashboard can show how well scoring did without the system cheating.
- **Sample of real pages:** 0.1% of fans also fetch the storefront page from Vercel, so the static path is exercised without loading Vercel with the crowd.
- **Budget:** at default knobs the gateway sees about 10,000 status polls per second at the peak plus joins; the gateway HPA scales on requests per second. `poll_interval_s` and `fans` are the levers if the cluster is smaller.

### D-4 How bots behave

Bot share is split across behaviours: refresh spammers (poll every 200–500 ms, no jitter), multi-account farms (20–200 places per /24, shared fingerprint), headless clients (automation user agent, skipped or fake proof of work), queue skippers (call checkout without a pass, with a forged pass or with a pass copied from another session), and cap breakers (try 8 tickets, then several guest emails with `+tags`). When admitted, bots buy the maximum allowed and pay by card. Each behaviour is a strategy in the simulator, so the crowd test in step 5 and the simulator share them.

### D-5 Metrics (I4, R2)

Every Go service exposes `/metrics` (Prometheus client) on a separate port and `/healthz`, `/readyz`. Labels always include `organiser`, `sale` where they apply; never personal data. Core series:

- Gateway: `eps_queue_waiting`, `eps_queue_prequeue`, `eps_queue_admitted_total`, `eps_queue_active_sessions`, `eps_queue_rate_setting`, `eps_bot_flagged_total{level}`, `eps_bot_sent_back_total`, `eps_pass_rejected_total{reason}`, `eps_http_requests_total{route,code}`, `eps_http_request_duration_seconds{route}`.
- Core: `eps_holds_total{result}`, `eps_orders_total{state}`, `eps_tickets_sold_total`, `eps_revenue_minor_total{currency}`, `eps_payment_attempts_total{provider,method,result}`, `eps_payment_failover_total{from,to}`, `eps_payment_breaker_state{provider}` (0 closed, 1 half-open, 2 open), `eps_payment_latency_seconds{provider}`, `eps_otp_total{result}`, `eps_cap_refused_total`, `eps_stock_left{pool}`, `eps_stock_held{pool}`.
- Worker: `eps_outbox_lag_seconds`, `eps_inventory_corrections_total`, `eps_holds_expired_total`.
- Fake providers: `fakepsp_charges_total{provider,outcome}`, `fakepsp_knob{provider,knob}`.
- Simulator: `sim_fans{state}`, `sim_bots{state}`, `sim_bot_purchases_total`, `sim_request_duration_seconds{step}`.

A Prometheus server in the chart scrapes pods by annotation every 2 s for the on-sale services (15 s otherwise), 6-hour retention. Logs are JSON lines on stdout with a request id.

### D-6 Aggregator and live stream (E8, R2)

The worker binary runs a single `aggregator` role (lease lock per sale). Every second, for each sale with a dashboard open or a run active, it builds one frame:
- **Exact values from Redis and Postgres:** queue waiting and pre-queue size, active sessions, rate setting, stock left and held per pool (sum of shards), breaker states, degrade state.
- **Rates from Prometheus** (instant PromQL over a 10 s window, minute rates scaled): admitted per minute, tickets sold per minute and total, payment attempts, failures per minute, success rate, failovers total, bots flagged and sent back, OTP challenges and failures, gateway requests per second, core p95, pods per service.
- **Events from the streams** (`payments`, `orders`): the last 20 notable events (failover, breaker opened or closed, sold out, oversell risk, degrade on) for the timeline.
- **Simulator ground truth:** fans and bots by state, bots admitted, bot purchases.

Frame JSON: `{ts, sale, queue:{waiting, prequeue, admitted_total, admitted_per_min, active, rate}, sales:{sold, sold_per_min, orders_paid, revenue_minor, currency}, payments:{attempts_per_min, success_rate, failures_per_min, failovers_total, providers:{a:{state, error_rate, p95_ms}, b:{...}}}, bots:{flagged, sent_back, otp_challenged, otp_failed, passes_rejected, truth:{bots, bots_admitted, bot_tickets}}, stock:[{pool, capacity, left, held}], system:{rps, core_p95_ms, pods:{gateway, core}}, degraded, timeline:[...]}`.
Frames are published on Redis pub/sub `live:{o:e:s}`; every gateway instance subscribes for the sales its clients watch and serves `GET /v1/live/{sale}` as server-sent events (organiser JWT checked; CORS for the Studio origin), with `id` = frame timestamp, a 15 s heartbeat comment and `retry: 2000`. The browser connects directly to the gateway, so the stream does not run through Vercel functions. Reconnect resends the latest frame. Sparklines keep the last 15 minutes in the browser; on reload the aggregator's last 900 frames in a Redis list fill them.

### D-7 Dashboard screen

Studio `/studio/sales/<sale>/live`: number tiles (in queue, admitted per minute, sold of capacity, revenue, payment success rate, failovers, bots caught), sparklines for queue length, admissions per minute and sales per minute, provider A and B status lights with breaker state and a marker on the chart for each failover, stock bars per pool (left, held, sold), a bots panel (flagged, sent back, OTP, rejected passes, and for simulator runs the ground truth "caught N of M bots, K real fans flagged"), the event timeline, the admission dial and pause (step 5's live controls), and the degrade switch. A "stale" badge appears if no frame arrives for 5 s.

### D-8 Graceful degradation (E11, Thin)

One switch per sale in Studio (and on the dashboard), set by Studio through the core (sales schema and Redis `degrade:{o:e:s}`), with Next.js keeping its own copy in the control schema (P2: each side writes its own schema) so the storefront can render without calling Go:
- **Event pages:** the storefront's event page is ISR-cached per published version (U3); turning the switch on revalidates the brand's tag and renders the static variant (no live widgets, prices and line-up only, a banner "We are very busy. The shop will reopen shortly; tickets are not lost by waiting").
- **Shop:** the buy button is replaced by the honest message; fans already in the queue keep their place and the waiting room keeps working.
- **Gateway and core:** new holds return 503 `EPS-BUSY` with `Retry-After: 30` and the same message; orders already held keep their timers and can still pay.
- Turning it off reverses all three within 10 s. The dashboard suggests the switch (it never flips it) when core p95 is over 2 s or the error rate over 5% for 30 s.

## Risks / Trade-offs

- [A small cluster cannot carry 100k fans at a 10 s poll] → knobs for poll interval, fans and pods; the HPA; the video states the run's real numbers.
- [Simulator in the same cluster competes for CPU with the system it tests] → pods with their own resource limits and a node selector for a separate pool where available; the dashboard shows pod counts.
- [Prometheus rates lag by a few seconds] → exact counts come from Redis; only rates come from Prometheus.
- [Simulated client addresses could be abused on real sales] → honoured only with a `sim` token on a rehearsal sale.
- [Rehearsal orders leak into reports] → `rehearsal` flag filtered in every report query, tested.

## Migration Plan

Additive tables and columns; Prometheus, aggregator and simulator Job template as new chart components. Rollback: revert the release; sales work without the dashboard.

## Open Questions

- Cluster size for the demo recording (nodes in fra1) decides the default poll interval; it can be set by the knobs without changing the specs.

## Deviation: fewer Vercel deployments (owner's request during step 6)

Vercel Hobby has a daily deployment limit, so Vercel now skips building a commit that changes nothing the web app uses. `apps/web/vercel.json` has an `ignoreCommand` (`deploy/scripts/vercel-ignore.sh`) that compares the commit with the branch's last deployed commit (`VERCEL_GIT_PREVIOUS_SHA`; on production without it, `HEAD^`; a preview without it always builds, because its first deployment is what the preview workflow binds to the pull request's database). A build is needed when a path matches `apps/web/**`, `api/**`, `package.json`, `package-lock.json` or `.nvmrc` (`deploy/scripts/web-paths.cjs`; there is no other shared package); docs, specs, `services/**`, `deploy/**`, `load/**` and `.github/**` alone skip; anything unclear builds. The `wip/**` rule stays.

The CI jobs that wait for Vercel (`preview.yml` web, `deploy.yml` web-smoke) use the same rule through `.github/scripts/vercel-deployment.cjs`: they find the newest ancestor with a ready Vercel deployment in the same environment and, when the files changed since it need no web build, use this commit's deployment if Vercel made one anyway, else that ancestor's (logged as `FALLBACK` with both commits). `EXPECT_WEB_COMMIT` follows the deployment actually used. When the web app did change, or no such ancestor exists, the strict wait for this commit's own deployment applies unchanged, as do the `vercel[bot]` and host checks. Tests: `deploy/scripts/web-paths.test.cjs`, `deploy/scripts/vercel-ignore.test.sh` and `.github/scripts/vercel-deployment.test.cjs` (CI job "slug").
