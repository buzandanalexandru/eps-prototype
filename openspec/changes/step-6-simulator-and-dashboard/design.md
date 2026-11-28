# Design

## Context

See proposal.md (Why). Steps 3 to 5 built what this step drives and measures (their archived designs D-15/D-16 and D-12/D-13):

- **Checkout and payments (step 4):** holds on the live snapshot, orders and attempts in the sales schema, simulated providers A and B with knobs **per merchant** (each organiser is its own merchant) and a Redis circuit breaker per organiser and provider, failover A → B, hosted card pages at `pay.<env>.<base>/<a|b>/pay/<id>` (outcome `approve`, `decline_funds`, ...), Apple and Google Pay without a page, outbox events on Redis Streams.
- **On-sale engine (step 5):** sale moments in the sales schema with settings versions and a signed copy per event in Redis; the gateway (no Postgres) serves join and status, runs the draw and the admitter and signs Ed25519 passes; the core's checkout gate picks **the event's current sale** and checks pass, caps per order and per person and the OTP; sharded stock per pool instance under `{org:event:<shard>}`; bot scoring per place; per-sale gauges reported by every gateway (dashboards take the max, never the sum).
- **Metrics:** every Go binary serves Prometheus text on `:9090/metrics` from a small standard-library registry (counters, gauges read at scrape time, histograms such as `eps_http_request_seconds{service,route}`); `/healthz` and `/readyz` exist (readiness fails without Postgres or Redis); logs are JSON lines with a request id.
- **Measured throughput (step 5 k6, 4-core sandbox shared with k6 and Redis):** one gateway answered 1,000 joins/s and 2,000 polls/s at p95 23 ms and 13 ms and saturated near 4,000 requests/s; one core held 500 holds/s at p95 32 ms and saturated near 900 holds/s. The 5,000 joins/s, 10,000 polls/s and 1,000 holds/s targets were not reached there.
- **The cluster is small and shared:** DOKS ams3, s-2vcpu-4gb nodes autoscaling 2 to 4, one Redis (1 GiB in production, 192 MB on previews), one Neon compute, Vercel Hobby; every pull request has its own namespace on the same nodes.

So the honest scope is a **bounded rehearsal**: real traffic through the real services, modest by default, capped per environment, with the measured numbers shown as measured and any 100,000-fan figure shown only as arithmetic with its assumptions.

## Goals / Non-Goals

**Goals:**
- Real HTTP traffic through the real gateway, core and providers, with the same rules as human fans (R3); nothing written directly.
- Simulated fans can only ever touch rehearsal sales: their own event id, stock, caps, queue, merchant account and breaker.
- A dashboard updated every second from the system's own numbers (R2), shown only to the sale's organiser.
- Bounded load: a run cannot exhaust the shared Redis, database, cluster or Vercel; it ends by itself; its data is cleaned up automatically.
- An honest busy mode (E11).

**Non-Goals:**
- A claim of a real 100,000-client test. The simulator runs in the cluster at the sizes below; step 12 measures throughput on the cluster with k6 and publishes the scaling maths.
- Grafana, alerting, tracing (named in the vision).
- Load on Vercel: simulated fans never call the web app.

## Decisions

### D-1 Rehearsal sales: their own event id

The checkout gate picks an event's current sale (step 5, D-12), and stock, caps and queues are keyed by organiser and event. A rehearsal that shared the real event id would become the real event's current sale and share its stock. So a rehearsal is a **sale moment with its own event id** `R` (a fresh uuid), linked to the source: `sales.sales` gains `rehearsal_of_event`, `rehearsal_of_sale` and `rehearsal_version` (the source's live snapshot version when it was made).

- **Create** (`POST /v1/organiser/sales/{saleId}/rehearsal`, owner or admin): copies the source sale's settings (rate, shopping limit, hold, caps, bot defence, OTP) into a new sale on `R` named "Rehearsal: <name>", opening a year ahead (nobody can join until a run sets the opening). At most 3 live rehearsals per organiser.
- **Snapshot:** the core's snapshot source answers for `R` with the source event's pinned version, rewritten so `eventId = R`, `version = 1`, its hash recomputed (`sales.rehearsal_source(event)`, a SECURITY DEFINER function owned by `eps_sales_index`). Everything downstream (holds, prices, caps, availability) then works unchanged on `R` with **full stock of its own**. The source going off sale ends its rehearsals (no snapshot).
- **Orders** on a rehearsal carry `rehearsal = true` and the run id (`sim_run_id`, from the simulator's token); they are on event `R`, so the real event's figures, stock and caps never see them. A report that sums by organiser filters `rehearsal = false` (tested).
- **Payments:** a rehearsal order pays under its own merchant account at the providers (`rh-<R>`) and its own breaker (`eps:{org}:R:pay-breaker:<provider>`), so a rehearsed outage or decline rate never reaches the organiser's real checkout (`payment_attempts.merchant_event`).
- **Reset** (`POST /v1/organiser/sales/{saleId}/rehearsal/reset`): ends the rehearsal (soft delete) and makes a fresh one from the same source (a new `R`: full stock, an empty queue). Refused while a run is active.
- **Clean-up (automatic):** the worker deletes the Redis keys of every rehearsal that was reset, deleted or whose last run ended more than 2 hours ago (queue, stock, caps, places, breaker and knob state under its tags, by a bounded `SCAN` per rehearsal, 1,000 keys per step). Postgres keeps the rehearsal's orders as a record, marked rehearsal: they hold no personal data (simulated emails at `sim.invalid`).
- **Shop label:** the waiting room and the checkout of a rehearsal show "Rehearsal: simulated fans" (the queue status and the order carry `rehearsal: true`); a person may join a rehearsal during a run (the presenter in the video).

### D-2 Runs: one bounded run at a time

- `POST /v1/organiser/sales/{saleId}/simulations` (owner or admin; the sale must be a rehearsal) validates the knobs and writes `sales.simulation_runs` (state `pending`). Refused when the organiser or the sale already has an active run (naming that run), or when **any** run is active in the environment (a shared cluster: "Another rehearsal is running; try again in a few minutes", naming nothing of another organiser).
- **Where it runs:** a `simulator` Deployment (one replica, its own CPU and memory limits) asks the core for work every 2 s on the core's **internal port** (8089, not behind the ingress; bearer `SIM_CONTROL_TOKEN`, compared in constant time; without the token outside `EPS_ENV=dev` both refuse to start). Deviation from R3's "Kubernetes Job": a Deployment needs no Kubernetes API client and no permission to create pods, and stops within a second; the simulator has **no database and no Redis access**, only HTTP to the gateway, the core and the providers' hosted pages.
- **States:** `pending → starting` (claimed: the core moves the rehearsal's opening to `now + opens_in`, pre-queue 5 minutes so it is open at once, and sets the decline rate on both providers for the rehearsal merchant) `→ running` (first report) `→ finishing` (duration over or stock gone: no new arrivals, fans in flight finish) `→ done`; or `stopped` (Stop pressed) or `failed` (no report for 30 s, or an error). The simulator reports every second; the answer carries `stop: true` once Stop was pressed, and it cancels every fan at once, so traffic ends within 15 s. Every end restores the rehearsal merchant's knobs.
- **Knobs** (default; range): `fans` 1,000 (10 to `SIM_MAX_FANS`: 2,000 on previews, 5,000 in production); `botShare` 0.10 (0 to 0.5); `prequeueShare` 0.6 (0 to 1); `arrival` `spike` (late arrivals decay exponentially over a minute) or `flat` (evenly over the first half of the run); `opensInSeconds` 60 (30 to 600); `qtyMix` 1:35, 2:40, 3:10, 4:15 (weights); `paymentMix` card 60, Apple Pay 20, Google Pay 20 (weights); `abandonRate` 0.08 (0 to 0.5); `declineRate` 0.03 (0 to 0.5); `pollSeconds` 10 (5 to 30, a floor under the server's `nextPollMs`, ±30% jitter); `providerA` `{mode: none|outage|failure_rate, atSeconds 60, forSeconds 90, failureRate 0.5}`; `durationSeconds` 300 (60 to 600). Wallet and saved cards are not in the mix (a simulated fan has no wallet or saved card; topping up wallets would write ledger lines in the organiser's real wallet books).
- **Provider A schedule:** the core applies it on the simulator's reports (elapsed since opening ≥ `atSeconds` → A's knobs `outage` or `failure_rate` for the rehearsal merchant; ≥ `atSeconds + forSeconds` → back), recorded on the run (`outage_on_at`, `outage_off_at`). The fake providers also reset knobs 15 minutes after a change (step 4), a backstop if everything else dies.
- **Bounded:** at most `SIM_MAX_INFLIGHT` (default 200) requests in flight per simulator, fans paced by their arrival schedule, every fan ends at the run's end, Redis use about 1.6 kB per waiting fan (10,000 fans ≈ 16 MB of production's 1 GiB).

### D-3 Virtual fans and their credentials

- **Token:** the simulator mints its fans' core tokens itself with an Ed25519 key of its own (`SIM_TOKEN_PRIVATE_KEY`; the core and gateway hold `SIM_TOKEN_PUBLIC_KEY`), issuer `urn:eps:sim:<env>`, kind `guest`, subject `guest:sim-<run>-<n>`, claims `rh` (the rehearsal event) and `run`, the fan's synthetic address in `cip`, 5-minute life (renewed). The verifiers accept this issuer **only** on the named rehearsal: the gateway's join refuses it for any sale that is not a rehearsal of that event; the core accepts it only on the checkout operations (create, read, details, pay, cancel, OTP) and its gate refuses a hold on anything but that rehearsal. A sim token on a real sale is answered 403 and never trusted for its address.
- **Journey** over HTTP/1.1 keep-alive to the in-cluster services: `arrive → join (proof of work solved for real) → poll status (server's next poll, floored by pollSeconds, jittered) → admitted (pass) → hold (quantity from the mix, product weighted by capacity) → details (email fan-<run>-<n>@sim.invalid) → OTP when asked (reads the simulated code from the response; 2% type it wrong once) → pay (method from the mix; a card fan posts the outcome to the provider's hosted page in the cluster, as a person presses Approve) → follow the order until paid, declined or expired → done`, abandoning at hold or payment with `abandonRate`.
- **Addresses:** fans get spread synthetic addresses (one per fan in a /16 of their own); bots get a few /24s and data-centre ranges. Bot scoring therefore sees real address signals; the core's hold limits per client apply per synthetic address.
- **Ground truth:** the simulator knows which clients are bots; the system does not. Its reports carry fans and bots by state, bots admitted, bot tickets, real fans who had to enter a code, request counts and latency per step.

### D-4 Bots

The bot share is split evenly across: refresh spammers (poll every 200 to 500 ms, no jitter), account farms (20 to 60 places per /24 with one shared browser print), headless clients (an automation user agent and a missing proof of work), queue skippers (hold without a pass, with a forged pass, or with another session's pass) and cap breakers (try 8 tickets, then several `+tag` emails). Admitted bots buy the most allowed by card. Each behaviour is a strategy in `internal/simulation`; the step 5 crowd test keeps its own copy (it tests the scorer without HTTP).

### D-5 Metrics (I4, R2)

Step 5's registry stays (no new dependency; same text format). Added series, never with personal data in a label (`organiser`, `sale`, `pool`, `provider`, `method`, `result` only):

- Core: `eps_orders_paid_total`, `eps_tickets_sold_total`, `eps_payment_attempts_total{provider,method,result}`, `eps_degrade_refused_total`, `eps_sim_reports_total`.
- Gateway: `eps_live_streams{}` (open streams), `eps_live_frames_sent_total`.
- Worker: `eps_live_frames_total`, `eps_live_frame_seconds` (build time), `eps_rehearsal_keys_deleted_total`.
- Simulator: `sim_fans{state}`, `sim_bots{state}`, `sim_requests_total{step,result}`, `sim_request_seconds{step}`.

A **Prometheus server** in the chart (`prom/prometheus` pinned, a namespace-scoped Role to list pods, one scrape job per annotation `eps.dev/scrape-interval`: 2 s for gateway, core and worker, 15 s otherwise; 6 h retention; 100m/200Mi requests, 256Mi limit on previews).

### D-6 Aggregator and live frames (E8, R2)

The worker's lease holder runs the aggregator. Each second it builds one frame for each **watched** sale (a Redis sorted set: the gateway adds a sale while a dashboard stream is open, the core while a run is active; at most 20 sales per second):

- **Exact values (Redis and Postgres):** queue state, pre-queue, line, slow lane, shopping, admitted total (the queue's own counters); stock per pool instance (sum of shards: left, held, sold, capacity); the rehearsal merchant's or organiser's breaker state for A and B; degraded; orders paid, tickets sold and revenue for the sale's event (one indexed query); payment attempts of the last 60 s by provider and result, and failovers in total (one query).
- **Rates:** per-minute rates are the change of those totals over the last 60 s of frames (or the frames there are), never computed in the browser.
- **System (Prometheus, when `PROMETHEUS_URL` is set):** gateway requests/s, core p95 and 5xx share over 30 s, ready pods per service. Without it the frame says `system: null`.
- **Timeline:** transitions between consecutive frames (breaker opened or closed, failover, sold out, degrade on or off, run state).
- **Truth:** the active run's latest report.
- **Suggestion:** `suggestDegrade` when core p95 > 2 s or the 5xx share > 5% in each of the last 30 frames; the switch is never flipped.

Frames go out on Redis pub/sub (`live:<sale>`) and into a list of the last 900 (15 minutes, 1 h expiry).

### D-7 Live stream on the gateway

`GET /v1/live/{saleId}` on the gateway, as server-sent events. The browser connects **directly** to the gateway (CORS for the exact web origins), not through a Vercel function: the dashboard fetches a 5-minute organiser core token from `/api/core-token` and opens the stream with `fetch` (EventSource cannot send a header), renewing the token on each reconnect. The gateway checks the organiser token and that the sale belongs to its organiser (403 otherwise), sends `retry: 2000`, the history (up to 900 compact points for the sparklines) and the latest frame, then each new frame with `id` = its timestamp, and a comment every 15 s. Streams are exempt from the 30 s write deadline (step 0); at most 200 streams per gateway and 10 per organiser; one Redis subscription per sale per gateway, fanned out in process. The dashboard shows "stale" after 5 s without a frame and reconnects with back-off.

### D-8 Dashboard screen

Studio `/studio/sales/<sale>/live`: tiles (in queue, pre-queue, admitted per minute, shopping, sold of capacity, revenue, payment success rate, failures per minute, failovers, bots flagged and sent to the back), SVG sparklines (queue, admitted per minute, sold per minute) with failover markers, provider A and B lights with breaker state, stock bars per pool (left, held, sold), a bots panel (flags, OTP challenges, refused passes and, for a run, the truth: "flagged N of M bots, K real fans flagged"), the timeline, the system strip, the rate dial and pause (step 5's live patch), the degrade switch and its suggestion. Charts are plain SVG; no chart library. Studio `/studio/sales/<sale>/simulator`: the rehearsal (create, reset, open the waiting room), the knobs with defaults and limits, Start and Stop, the state, the summary, and the **honest load panel**: the run's measured numbers next to step 5's k6 numbers and the arithmetic for 100,000 fans with its assumptions, labelled as arithmetic, not a test.

### D-9 Graceful degradation (E11, Thin)

One switch per sale (`PATCH /v1/organiser/sales/{saleId}/degrade`), stored on the sale (`degraded`, `degraded_at`, `degraded_by`, history in the outbox) and in its signed Redis copy, so every service sees it within 2 s:
- **Core:** new holds get 503 `EPS-BUSY` with `Retry-After: 30` and the honest message; held orders keep their timers and can still pay.
- **Gateway:** the admitter lets no one in while degraded (the line keeps its order); status says `degraded: true`, so the waiting room shows the busy message and keeps polling.
- **Shop:** the event page is already a static page (cached per publish); its availability answer carries `degraded: true`, and the page then shows the busy banner in place of the buy controls. Deviation from the first plan (a control-schema copy and a revalidation of the brand's pages): the availability answer reaches every open page within its 2 s CDN life without a publish, and the core stays the one writer of sale state (P2).
- Off reverses all three within 10 s.

### D-10 As built

- **Tests over real HTTP.** `internal/simtest` starts the whole system in one test process (core contract handler, gateway with its admitter and live stream, both providers, the internal simulator API and the simulator engine) behind real HTTP servers over testcontainers Postgres and Redis; the simulator, rehearsal, stream and degrade tests use it.
- **Summary rate per order.** The summary's payment success rate is orders paid of those that pressed Pay (a payment that failed over from A to B and succeeded counts as paid); the dashboard's last-minute figure is labelled as attempt-level, so an outage shows there as failed attempts at A.
- **Reconnecting.** The dashboard also drops and reopens a connection that brought no frame for 10 s (half-open connections), and listens for a page-local `eps-live-drop` event that cuts the connection like a network drop (used by the e2e test; it can only be sent by the page itself).
- **Per-sale counters.** Bot flags (`sus`) and refused passes (`prj`) are counted on the sale's queue state hash when they happen, so a frame reads them in one call; the summary splits flags into bots and real fans only after the run, from the bots' places the simulator reports in its final report (the system never sees them while it runs).
- **Keys reach the gateway sooner.** The core publishes the core-token keys to Redis every 10 s (was every minute): in a fresh environment the web app's first signing key otherwise reached the gateway, and so the dashboard's direct stream, only up to a minute later (the stream retries meanwhile).
- **Internal port is required.** The core stops when it cannot bind its internal port (Kubernetes restarts it) instead of serving without the simulator's API.
- **A pool deadlock fixed (found by the simulator).** `checkout.Complete` loaded the live snapshot inside its transaction; with every pool connection held by paid transactions waiting on one snapshot load, the pool deadlocked. It now loads the snapshot before the transaction, and the rehearsal snapshot source reads mapping and document in one transaction.
- **Health.** `/readyz` failures are logged as a JSON line with the failing checks and the request id; every binary's logger is also the process default.
- **Metrics.** The standard-library registry stays (no Prometheus client dependency); added `eps_orders_paid_total`, `eps_tickets_sold_total`, `eps_payment_attempts_total{provider,method,result}`, `eps_degrade_refused_total`, `eps_sim_runs_total`, `eps_sim_reports_total`, `eps_sim_runs_lost_total`, `eps_live_streams`, `eps_live_streams_opened_total`, `eps_live_frames_sent_total`, `eps_live_frames_total`, `eps_live_frame_seconds`, `eps_rehearsal_keys_deleted_total`, `sim_fans{state}`, `sim_bots{state}`, `sim_requests_total{step,result}`, `sim_request_seconds{step}`.

## Risks / Trade-offs

- [A small cluster limits run sizes] → defaults of 1,000 fans, a cap per environment, and the measured numbers on screen; scaling claims are arithmetic, labelled.
- [The simulator shares CPU with the system it tests] → its own limits (1 CPU, 256 Mi), at most 200 requests in flight; the dashboard shows pod counts.
- [The simulator's key can mint tokens] → accepted only on rehearsal events and checkout operations; a compromised simulator can only buy rehearsal stock.
- [A rehearsed outage leaks into the real checkout] → a merchant account and breaker per rehearsal event, tested.
- [Rehearsal data fills Redis] → automatic clean-up within 2 hours of a run's end; one active run per environment.
- [Prometheus rates lag] → only the system strip uses them; every on-sale number is exact.

## Migration Plan

Migration 00004 adds columns to `sales.sales`, `sales.orders` and `sales.payment_attempts`, the `sales.simulation_runs` table and three SECURITY DEFINER functions owned by `eps_sales_index`. `runtime-secret.sh` adds `SIM_CONTROL_TOKEN` and the `SIM_TOKEN_*` pair to an existing `eps-runtime` without changing other keys. The chart adds Prometheus and replaces the simulator Job with a Deployment. Rollback: revert the release; sales work without the dashboard; the columns are additive.

## Open Questions

None that change the specs. Cluster run sizes are set by `SIM_MAX_FANS` per environment.

## Deviation: fewer Vercel deployments (owner's request during step 6)

Vercel Hobby has a daily deployment limit, so Vercel now skips building a commit that changes nothing the web app uses. `apps/web/vercel.json` has an `ignoreCommand` (`deploy/scripts/vercel-ignore.sh`) that compares the commit with the branch's last deployed commit (`VERCEL_GIT_PREVIOUS_SHA`; on production without it, `HEAD^`; a preview without it always builds, because its first deployment is what the preview workflow binds to the pull request's database). A build is needed when a path matches `apps/web/**`, `api/**`, `package.json`, `package-lock.json` or `.nvmrc` (`deploy/scripts/web-paths.cjs`; there is no other shared package); docs, specs, `services/**`, `deploy/**`, `load/**` and `.github/**` alone skip; anything unclear builds. The `wip/**` rule stays.

The CI jobs that wait for Vercel (`preview.yml` web, `deploy.yml` web-smoke) use the same rule through `.github/scripts/vercel-deployment.cjs`: they find the newest ancestor with a ready Vercel deployment in the same environment and, when the files changed since it need no web build, use this commit's deployment if Vercel made one anyway, else that ancestor's (logged as `FALLBACK` with both commits). `EXPECT_WEB_COMMIT` follows the deployment actually used. When the web app did change, or no such ancestor exists, the strict wait for this commit's own deployment applies unchanged, as do the `vercel[bot]` and host checks. Tests: `deploy/scripts/web-paths.test.cjs`, `deploy/scripts/vercel-ignore.test.sh` and `.github/scripts/vercel-deployment.test.cjs` (CI job "slug").

## Review decisions

The three reviewers' findings and the coordinator's decisions (D-a to D-j), as built. The PR body maps each item to its commit.

- **D-a Rehearsal money.** A rehearsal takes only one-off payments at the simulated providers: wallet, saved card, `saveCard` and instalments are refused with 422 `EPS-PAY-METHOD` (`payments.RehearsalMethod`, checked in `PayOrder` and the wallet) and are not offered (the order view hides them). Payment attempts carry `merchant_event`; rehearsal orders make no ledger postings and are never issued (`checkout.Issuable`), so step 7's issuer skips them by the same rule. `sales.real_orders` (security-invoker view) is what organiser reports read. The clean-up anonymises rehearsal orders (contact details and answers) before it marks a rehearsal cleaned.
- **D-b Production safety.** Runs are refused while a real sale on the platform has fans in its waiting room (`sales.real_sales_near`, a SECURITY DEFINER function owned by `eps_sales_index`, then the queue counts); one run at a time **on the cluster** through a Kubernetes Lease `eps-simulator` in namespace `eps-sim` (every environment's simulator has a Role there; chosen over a shared Redis key because environments have separate Redis instances and the Lease needs no new dependency, only the pod's own token); at most 1,500 requests a second per run (a pacer in the simulator, `SIM_MAX_RPS`); one spammer in a 9-slot bot cycle; production cap 5,000 fans; 3 runs an hour per organiser. In production the shared sample organiser runs at the default size only and its real sale cannot be degraded. Every run start, stop and degrade switch is logged with who made it. The panel's copy states these rules instead of "never uses up the shared cluster".
- **D-b in the e2e suite.** The refusal during real sales holds in the test environments too, and other e2e tests leave real sales with fans in their pre-queue (waiting-room.spec opens a sale 20 minutes later). `simulator-run.spec.ts` is therefore its own Playwright project, `rehearsal`, which runs first; `desktop` and `phone` depend on it. The production smoke runs with `--no-deps`, so it never starts a rehearsal. The panel names the reason: "A real sale on this platform has fans in its waiting room" apart from "Another rehearsal is running", and the hourly limit has its own message.
- **D-c** The rehearsal closes after its run (paused, opening moved past the horizon).
- **D-d Degraded mode.** Admitted fans keep their turn: a pass that expired inside the degraded window is accepted for the window's length after it (`degraded_window_start`/`_end` on the sale). The busy copy is "Your place in the queue is kept; buying resumes when the load eases." / "Locul tău în coadă este păstrat; cumpărarea se reia când scade aglomerația.". The switch reads the generation before the list (no lost update). The suggestion resets while degraded and the core error share excludes `eps_degrade_refused_total`.
- **D-e** The simulator stops every fan when no report succeeds for 25 s; Start checks the rules (sample, preflight) before it resets a used rehearsal; deleted rehearsals wait 20 minutes (the hold time) before their clean-up. **Rejected:** tracking every rehearsal key in a set; that would add a write to the hot path of every queue and stock call. Instead the clean-up runs a bounded, resumable SCAN whose cursor is kept in Redis, so no run blocks Redis and an interrupted clean-up continues.
- **D-f Honest numbers.** The note says this run's numbers are measured here; the load card keeps the step 5 sandbox numbers, this run's measurements and the arithmetic under separate headings. "Sold out after" comes from the system (the last paid order once every pool instance's paid units reach its capacity), not the simulator. Fans and bots are counted when they make their first request, next to the planned number. The dashboard tile reads "Places flagged (bots and fans)" and the bots-against-truth split appears only after the run. The 100k arithmetic is sized on 2,000 polls a second at p95 13 ms per gateway (about 5 gateways) and names the opening burst. Requests per second are "average over the run" with the busiest second next to it; p95 is over the whole run (a reservoir sample). "Paid at B during the outage" is shown next to failovers.
- **D-g SSE.** Each write has a 10 s deadline; a stream ends at its token's expiry; the aggregator watches at most 20 sales, 5 per organiser, chosen fairly, with its own pool and at most that many builds in parallel; the write-deadline exemption covers only `/v1/live/`; a latest frame older than 5 s is not sent as live; the browser judges staleness by the frame's own time; frame failures are logged at a limited rate and counted in `eps_live_frame_errors_total`.
- **D-h** NetworkPolicies: the core's internal port 8089 only from the simulator, Prometheus only from the worker, Redis only from core, gateway, worker and the providers. The cluster strip on the dashboard is labelled platform-wide.
- **D-i CI and Vercel.** The ignore step diffs with `--no-renames` and checks `previous_filename`; unknown paths build; the CI fallback requires `d.ref === branch` and a commit after the merge base, skips when there is no earlier deployment and uses the newest READY deployment (redeploys included). Limitation: Vercel compares with `VERCEL_GIT_PREVIOUS_SHA`, falling back to `HEAD^`, so a push of several commits is judged against its last ready deployment only through CI.
- **D-j Scale (owner decision, 28 Nov 2026: "cap + honest maths").** Runs stay capped per environment (5,000 production, 2,000 previews), refused during real sales; 100k is shown only as arithmetic from measured rates. `docs/planning/feature-scope.md` (E7, Act 2) and `architecture.md` R3 now say so (R3 also records the Deployment instead of a Job). The Test lines of waiting-room "Opening spike" and inventory "Throughput" (`load/queue.js`, `load/inventory.js`) move to step 12, which measures the cluster with k6; those specs belong to other capabilities, so step 12's `/opsx:update` edits them.
- **Also fixed.** corr M2: the aggregator has its own pool, concurrency at most its size. corr L3: the bot-flag crossing comes back atomically from the poll script. corr L4: `OnChange` carries the scope (organiser and event). corr L7: paid counters are incremented after commit, labelled `rehearsal`. spec L2: timeline values are translated. spec L5: the degrade e2e asserts within 10 s of each switch. spec L6: the summary test requires `done`; "During a run" asserts each listed number; "Outage on schedule" asserts new payments complete on B (`TestOutageRunPaysAtB`); "Metrics present" covers `eps_degrade_refused_total`, `eps_sim_reports_total`, the providers' and the simulator's series (`fakepsp_knob_changes_total` added for the providers' knobs).
