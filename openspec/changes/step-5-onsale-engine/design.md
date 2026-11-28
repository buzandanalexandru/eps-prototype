# Design

## Context

See proposal.md (Why). Step 4 provides orders, holds behind an inventory interface (`Reserve`, `Extend`, `Commit`, `Release` returning whether a payment won, `ReleaseMany`, `HoldState`, `Unsell`, `Used`, `Rebuild`, `Lock`, and the orphan sweep's `HoldIndex`; step 4 design D-15 and D-16), the payment state machine and the outbox. Redis runs inside the cluster as one primary (no replica), its append-only file (`everysec`) on a block-storage volume in production; a Redis loss is recovered from Postgres (inventory counters rebuilt from the orders, sale copies rewritten by the worker, token keys republished by the core; waiting places are lost and fans join again behind the line: see D-13). Keys are prefixed by organiser and event (U2) and use hash tags so they can move to Redis Cluster later. The waiting-room page is served by Vercel as a static shell; its calls go through the web app's own-origin route `/api/shop/queue/...` (a Vercel function) to the Go gateway on DOKS: joins carry a fresh core token, polls only the queue token (D-12, D-13). Sizing target from festival-tech section 10: about 50k arrivals per second at opening at the edge, and a controlled 200 to 500 checkouts per second at the core.

## Goals / Non-Goals

**Goals:**
- Fair entry: arrival before opening gives no advantage within the pre-queue; after opening it is first come, first served (O1).
- Checkout load set by a dial, never by the crowd (O1).
- Bots slowed silently, real fans never blocked; OTP only for risky or bulk buyers (E5, E12).
- No oversell and no hot key; Postgres stays the record of truth (O2).

**Non-Goals:**
- Edge WAF or CAPTCHA vendors, ID verification, device attestation (named in the vision).
- Pre-registration and lottery (step 9, E9).
- The simulator and the dashboard (step 6); this step exposes the metrics they read.

## Decisions

### D-1 Sale and sale settings (E1)

A **sale** is one sale moment of one event (presale, general sale) linked to a published snapshot version; caps count across all sales of the event. Settings are owned by Go in the sales schema and edited by Studio through the OpenAPI contract (P2: each side writes its own schema); every save makes a new settings version, cached in Redis at `sale:{o:e}:settings`. Live fields (admission rate, active-session limit, pause, OTP switch, bot defence) take effect within 2 s; structural fields (opening time, pre-queue window, queue on/off) cannot change after the pre-queue opens. Defaults: pre-queue opens 30 min before; rate 2,000 fans/min; active sessions 1,500; hold 8 min; cap 4 per order, 6 per person; OTP off, threshold 4 tickets.

### D-2 Queue flow (O1, E2)

1. **Join** `POST /v1/queue/{sale}/join` (gateway). The browser sends a session secret it created (random 32 bytes in sessionStorage, header `X-EPS-Session`) and a light proof-of-work solved by the waiting-room page (SHA-256 with 16 leading zero bits, about 50 ms on a phone). The gateway returns a signed queue token `qt` (same format as the pass, D-4, kind `queue`). One place per account (`q:{o:e}:acct:<id>` → qid) or per session; rejoining returns the same place.
2. **Pre-queue:** before opening, members go into `q:{o:e}:pre` (sorted set, score = arrival ms).
3. **Shuffle at opening:** a worker job takes the lock `q:{o:e}:shuffle`, reads the pre-queue, applies a Fisher-Yates permutation with a crypto-random seed (the seed is stored with the sale for audit), and writes positions 1..N into `q:{o:e}:line` (score = position) in pipelined batches of 5,000; then sets `q:{o:e}:tail = N` and `q:{o:e}:state = open`. Joins during the shuffle wait in the pre-queue and are included; joins after go to the line with `INCR tail`.
4. **Status** `GET /v1/queue/{sale}/status` (header `qt`): returns position ahead of you, an estimate from the current rate, and `admitted` with a pass when it is your turn. The page polls every 10 s with ±3 s jitter; the server also returns `next_poll_ms`.
5. **Admission:** one admitter per sale (lease lock `q:{o:e}:admitter`, 5 s, renewed each tick) ticks every 500 ms with a token bucket of `rate/120` tokens per tick (fractions carried) limited by `active_limit - ZCARD(q:{o:e}:active)`; it `ZPOPMIN`s that many from the line into `q:{o:e}:admitted` (hash qid → admitted_at). Positions are audited to Postgres in batches by the worker.
6. **Queue off:** joining issues a pass at once (still rate-limited and scored); checkout still requires the pass.

### D-3 Redis key design

All keys carry the hash tag `{o:e}` = `{<organiser_id>:<event_id>}` (queue keys use `{o:e:s}` with the sale id).

| Key | Type | Purpose |
|---|---|---|
| `q:{o:e:s}:pre` | ZSET | pre-queue, score = arrival ms |
| `q:{o:e:s}:line` | ZSET | the line, score = position (penalised scores for suspects, D-6) |
| `q:{o:e:s}:tail` | STRING | last position handed out |
| `q:{o:e:s}:admitted` | HASH | qid → admitted_at |
| `q:{o:e:s}:active` | ZSET | pass id → expiry ms (active checkout sessions) |
| `q:{o:e:s}:acct:<id>` | STRING | one place per account or session |
| `q:{o:e:s}:state` | STRING | `pre`, `shuffling`, `open`, `paused`, `closed` |
| `pass:{o:e:s}:<pass_id>` | HASH, TTL 600 s | state `active|used|revoked`, order id |
| `inv:{o:e:p:<i>}` | HASH | shard i of pool p: `remaining`, `allocated`, `sold` |
| `inv:{o:e:p:<i>}:holds` | HASH | hold id → qty on this shard |
| `inv:{o:e:p:<i>}:exp` | ZSET | hold id → deadline ms |
| `inv:{o:e:p}:meta` | HASH | shard count, capacity, snapshot version, frozen flag |
| `cap:{o:e}:<identity>` | HASH | `held`, `sold` tickets for this person |
| `bot:{o:e:s}:<qid>` | HASH | signal counters and score |
| `rl:{o:e:s}:ip:<ip>:<minute>`, `rl:{o:e:s}:qid:<qid>:<10s>`, `rl:{o:e:s}:pass:<id>:<minute>` | STRING, TTL | rate limits |
| `otp:{o:e}:<order_id>` | HASH, TTL 300 s | code hash, tries, resends |

### D-4 Pass format and lifetime (E3)

`v1.<base64url(payload)>.<base64url(signature)>`, Ed25519 (same family as O5, so the core verifies with a public key and only the gateway holds the private key; `kid` for rotation). Payload: `{v, kind: "pass", pid (128-bit random), org, evt, sale, sub (account id or guest identity), sid (SHA-256 of the session secret), qid, iat, exp}`. Life: 10 minutes from admission (O1). Checkout (hold, payment, OTP) requires header `X-EPS-Pass` on sales with the queue; the core checks signature, expiry, org, event and sale against the path, `sid` against `X-EPS-Session`, and the Redis pass state. A pass can carry one live order; it is revoked when that order is paid or cancelled, and the active-set entry is removed so the admitter can let the next fan in. A pass that expires while its order is `paying` stays valid for that order only until the attempt resolves.

### D-5 Rate limits

Join: 10 per minute per client IP (subnet /24: 200). Status: 1 per 2 s per queue token; faster polls get the cached last answer, not an error, and count as a bot signal. Checkout: 60 requests per minute per pass. Over-limit joins get 429 with `Retry-After`; everything else degrades silently.

### D-6 Bot scoring and where suspects go (E5)

Signals collected at join and while waiting, each adding points to a 0–100 score in `bot:{...}`:

| Signal | Points |
|---|---|
| Proof-of-work missing or invalid | 40 |
| Polling faster than every 2 s (per 10 calls) | 10, max 30 |
| Poll intervals too regular (std dev under 100 ms over 10 polls) | 20 |
| More than 3 queue tokens from one client IP; more than 50 from one /24 | 15 / 25 |
| Known data-centre address range (configured list) | 20 |
| Automation markers in the user agent (headless) | 25 |
| Several accounts sharing one browser fingerprint hash (sent by the page) | 15 |
| Guest or account created within the last hour | 10 |
| Checkout called without a pass, with a forged pass or with another session's pass | 30 each, max 60 |

Thresholds: **suspect** at 70 or more, **risky** at 40 to 69. Scores are computed from the start, but **acted on only after the shuffle**, so nothing visible changes before opening. A suspect's line score is moved to `tail + 1,000,000 + position` (the back, in its own order); a score of 90 or more is held in a "slow lane" admitted only when the line ahead is empty. Suspects still see a normal status page with a slowly moving position; they never get an error or a block message. Risky buyers are sent to OTP at checkout (D-8). A real fan opening two tabs or reloading is not a suspect on its own (at most 30 points). The simulated client IP is trusted only on rehearsal sales with a valid simulator run token (step 6); on every other sale the gateway uses the proxied client address.

### D-7 Caps per person (E6)

Identity: account id; for guests, the normalised email (lower case, `+tag` removed, dots removed for Gmail). At hold time a Lua script on `cap:{o:e}:<identity>` checks `held + sold + qty <= cap` and adds to `held`; payment success moves it to `sold`; release or expiry subtracts. The per-order cap is checked in the same request. Postgres `cap_ledger` is the record of truth and is checked again in the paid transaction; a guest who later signs in is counted under both identities. Over-cap requests get `EPS-CAP-REACHED` with how many the person can still buy.

### D-8 Risk-based OTP (E12)

When the organiser has switched OTP on, payment for an order requires a verified code if the buyer is risky (score 40 to 69) or a suspect who reached checkout, or the order has more tickets than the threshold. The code is 6 digits, valid 5 minutes, stored hashed in `otp:{o:e}:<order>`; up to 3 wrong tries per code and 3 resends. It is **simulated**: shown on screen in a "simulated SMS" panel. Exhausting tries cancels the order, releases the stock and adds 20 points. Everyone else goes straight to payment.

### D-9 Sharded stock (O2)

- At sale start each pool's capacity is split into N shards, `N = min(32, max(1, ceil(capacity / 250)))`, allocations as even as possible; `allocated` per shard is fixed until a reconcile or capacity change.
- **Reserve(hold, pool, qty):** pick a start shard from a hash of the hold id, then walk shards; on each shard a Lua script takes `min(remaining, needed)`, records `holds[hold] += take` and sets `exp[hold] = deadline`, atomically within that shard. A multi-pool order reserves pool by pool in a fixed order; if any pool comes up short, the shards already taken are released (compensation). If the core crashes mid-way, the partial entries expire on their own deadline. No script spans two shards, so no cross-slot atomicity is needed.
- **Extend:** raise `exp[hold]` on every shard that has the hold.
- **Commit:** per shard, if `holds[hold]` exists: `sold += qty`, delete from `holds` and `exp`; idempotent. If the hold is gone (expired), commit fails and step 4's late-payment rule applies.
- **Release:** per shard, if `holds[hold]` exists: `remaining += qty`, delete; idempotent.
- **Expiry:** the worker's shard sweeper calls a Lua `sweep(now)` on every shard each second, releasing due holds.
- **Invariant per shard:** `remaining + Σholds + sold = allocated`.
- **Sold out:** a pool is sold out when every shard has `remaining = 0` and no holds; while holds remain the shop says "All tickets are held right now; some may come back" (Glastonbury pattern).

### D-10 Reconciler

Every 15 s per active pool (and on demand from Studio): read each shard with one Lua `snapshot` (remaining, held, sold, allocated), then read Postgres paid quantity for the pool twice around it. If the two Postgres reads differ, skip this round. Checks:
1. Each shard's invariant; a broken shard is repaired by setting `remaining = allocated - held - sold`.
2. `Σ sold (Redis)` against Postgres paid quantity. Redis ahead by commits whose order is not paid after 2 minutes: those are returned to `remaining`. Postgres ahead (a Redis loss): `sold` is raised and taken out of `remaining`; if `remaining` cannot cover it, the pool is **frozen** (no new holds), an `inventory.oversell_risk` event is written and the dashboard shows it.
3. `Σ allocated` against the snapshot's capacity; a capacity change from a new snapshot adjusts shard allocations.
A correction is applied only if the same drift is seen in two rounds in a row. Every correction writes an `inventory_adjustments` row and an `inventory.drift_corrected` event.

### D-11 Checkout gate ordering

For a hold: pass check → per-order cap → per-person cap (Lua) → sharded reserve → order row. For payment: pass check → OTP check if required → step 4 payment flow. Failures release what was taken, in reverse order.

### D-12 As built (step 5 implementation)

Decisions made while building, on top of D-1 to D-11:

- **Where things run.** The gateway serves join and status, runs the admitter (a 5 s lease per sale, renewed each 500 ms tick) and the draw at opening (moved from the worker: it is pure Redis and must start the moment the sale opens), and signs passes. It keeps **no Postgres pool**: the core publishes the core-token public keys to Redis every minute, and every sale save writes the event's sales to Redis, both signed with `SNAPSHOT_CACHE_KEY`; the worker rewrites the copies of sales opening within a day either side every minute (`sales.events_with_sales`, a SECURITY DEFINER function owned by `eps_sales_index`), copies the queue audit (joins, drawn positions, admissions, the draw's seed) to Postgres in batches every 2 s, runs the shard sweeper every second and the reconciler every 15 s (one worker at a time, a Redis lease).
- **The browser reaches the gateway through our own origin.** The shop's fixed CSP allows only `'self'`, and behind the load balancer the gateway sees one source address, so every waiting-room call is a Vercel function invocation (a cost and a hop at an opening spike; serving the waiting room from the gateway's own host is the step 12 path). The waiting room calls `/api/shop/queue/<sale>/join|status`, which the web app forwards to the gateway's `/v1/queue/...` with a fresh core token (account or guest, with the browser's address in `cip`) and the browser's user agent (`X-EPS-Client-UA`). The two queue operations are in the contract with `x-eps-service: gateway`; the core answers them with 421 `EPS-WRONG-SERVICE`.
- **Session and pass storage.** The page keeps one session secret per browser in localStorage (all tabs share one place), a tab id in sessionStorage, the queue token per sale and the pass per event. One place per account or guest identity (`qo:<keyed hash of sale|sub>`); a place whose pass was used, revoked, ran out or was not collected in time lets the person join again at the back.
- **Redis keys as built** (all through `redisx`): inventory shard `i` of an event on `{org:event:<i>}` (`inv:<instance>` hash alloc/used/sold/halt/frz, `hs:<hold>` the hold's units on that shard, `hx` the shard's expiry index); each hold's record on one of 16 home tags `{org:event:h<k>}` (`hold:<id>` with state, deadline and its units per shard; `hx` and `hc` indexes); the event's meta on `{org:event:0}`; every queue key of a sale on `{org:event:<first 12 hex of the sale>}` (`qs` the state hash: state, tail, token bucket, admitted count, the draw's seed and progress; `qp` pre-queue, `ql` line, `qw` slow lane, `qa` shopping places, `qm` the draw's snapshot, `qr` admissions per tick, `qu` audit, `qf:<qid>` one hash per place with its entry, signals and passes, `qo:<who>` place by identity, `qi`/`qn`/`qk` address, /24 and print sets, `qz` rate limits, `qx` locks); caps and OTP challenges on the organiser tag (`cap`, `otp`).
- **Shards (D-9).** Allocation per shard is set by a resize: on first use or when the live capacity differs, new holds stop on every shard for a moment (`frz`, at most 3 s), each shard keeps what it has taken and the free units are spread evenly; a capacity below what is taken leaves every shard short in proportion and marks the instance "over", so its commits check the capacity one at a time and nothing sells above it. The shard count only grows. A hold's record is written first (state `reserving`), then the units shard by shard, then the record becomes `held` with where its units are; a failure part way returns them and drops the record. The record decides every race exactly like step 4's hold hash (`Release` reports `committed` atomically); shard entries are settled after it, and the shard sweeper finishes what a crash left (commit, release, or follow an extended deadline; a `reserving` record past its deadline is lapsed). A Redis loss rebuilds the event from the orders onto shard 0 and the next hold re-splits it. Step 4's `RedisInventory` was removed in review (D-13).
- **Reconciler (D-10)** compares per shard (used = sold + live entries), the allocations with the capacity and Postgres's paid units with the counters; commits left unpaid are found per hold (an index of committed holds, checked after 2 minutes) rather than from the sums. A pool it halts (oversell risk) stays halted: lifting it is an operator action (no Studio button yet), and the on-demand run from Studio is not built.
- **Honest sold out.** The availability answer keeps `status: sold_out` for a full pool (nothing can be bought now) and adds `allHeld: true` when its units are held, not sold; the shop then says "All tickets are held right now; some may come back".
- **Checkout gate (D-11).** The checkout calls a `Gate` before stock is taken: the event's open sale (the latest opened; an event with sale moments but none open sends the fan to the next one's waiting room), its pass (signature with the public key only, expiry, organiser, event, sale, session and the buyer's own subject, and the pass's state in Redis), the cap per order, then the cap per person. A sale's hold length replaces the snapshot's. A payment needs the order's own pass from the same session (its 10 minutes may have run out: the hold bounds it); a payment through the order's signed resume or view link needs no pass (the link is itself a capability bounded by the hold). Passes end through the worker's `onsale` consumer of order events: paid uses the pass, cancelled, expired or refunded revokes it; either frees a shopping place at once.
- **Caps (D-7).** Each live order is an entry under the person's Redis cap hash with its own expiry (hold deadline plus the payment allowance), so a lapsed hold stops counting by itself; paid tickets come from the `cap_ledger` table (the record of truth, written in the paid transaction). The cap is the stricter of the sale's cap per person and the snapshot's per-account maximum; step 4's per-account count is skipped when a sale applies. A guest is counted under its guest id until it gives an email, then under the normalised email (checked again there). Payment success checks the ledger again and refunds an order that would go above the cap.
- **OTP (D-8).** Whether a code is needed is decided at hold time (score 40 or more, or more tickets than the threshold) and asked only while the sale's OTP switch is on. Three wrong tries per code, three resends; when the last try of the last code fails the order is cancelled and the place gets 20 points.
- **Bot scoring (D-6).** As designed, except: the "account created within the last hour" signal is not built (core tokens carry no account age); "checkout without a pass" scores only a pass with a good signature presented from another session (a missing or forged pass names no place); places from one address beyond 3, or one /24 beyond 50, score the earlier places too. Suspects are moved after the draw and again lazily when the admitter meets one at the front, so a score that rises while waiting still counts.
- **Studio.** `/studio/events/<id>/sales`: sale moments with every setting (opening in the event's time zone), the live rate dial, pause, the waiting-room link, live queue figures (state, pre-queue, line, admitted, shopping, the draw's seed) and the version history. "queue" is a reserved page slug (the waiting room's address).
- **Load.** `cmd/loadkit` (EPS_ENV=dev only) prepares a published event, an optional open sale, a token key the core trusts and N guest tokens with sessions and proofs of work; `load/inventory.js` and `load/queue.js` drive the core and the gateway.
- **Helm.** `QUEUE_PASS_PRIVATE_KEY` (the gateway only) and `QUEUE_PASS_PUBLIC_KEY` (gateway and core) are a pair made by `runtime-secret.sh` in `eps-runtime`; both services refuse to start without them outside `EPS_ENV=dev`. The gateway's HPA stays on CPU: scaling on requests per second needs a custom-metrics adapter (step 12).
- **Metrics** (`:9090`): `eps_queue_prequeue_size{sale}`, `eps_queue_length{sale}`, `eps_queue_admitted{sale}`, `eps_queue_active_sessions{sale}`, `eps_queue_joins_total`, `eps_queue_admitted_total`, `eps_queue_passes_issued_total`, `eps_queue_suspects_moved_total`, `eps_queue_slow_lane_total`, `eps_bot_suspects_total`, `eps_otp_total{outcome}`, `eps_pass_rejections_total{reason}`, `eps_inventory_free|held|sold{organiser,event,instance}`, `eps_inventory_corrections_total{kind}`, `eps_inventory_reconcile_rounds_total`, `eps_inventory_shard_entries_settled_total`, plus step 4's hold counters.

### D-13 Review decisions (three adversarial reviews of PR #7)

- **Upgrade from step 4 (D-a).** The sharded engine has its own loaded mark (`sinv-ready`): after the deploy the first use rebuilds every event from Postgres instead of trusting step 4's mark, a commit of a hold the engine has not loaded returns `ErrNotReady` so the checkout rebuilds and commits it, and the rebuild deletes step 4's counters, ready mark, expiry index and the hash of each live hold it took over. Step 4's single-counter engine is removed from the code.
- **Joins and the draw (D-b, D-c).** A join decides by Redis state and Redis's own clock: an open line takes the fan at the back whatever the gateway's clock says; a fan arriving at or after opening before the draw ran is a latecomer, behind every pre-queue member in arrival order. The draw is resumable: it snapshots the members who arrived before opening and a crypto seed first (the one seed the audit keeps), writes positions in batches that each record how far it got, under a 10 s lock renewed per batch; a rerun continues the same permutation, joins during the draw go behind it, and the tail is drawn plus latecomers.
- **One place, a pass per browser (D-d).** One place per account or guest; each of the fan's browsers (sessions, at most 4) collects its own pass for that place, issued by one Lua script per session. A pass presented from another session is scored only when it belongs to another account; expired or ended passes are never scored.
- **Scoring real crowds (D-e).** A browser print counts only when more than 20 people share it; the address signals (places per address or IPv6 /64, per /24 or /48) do not add up and alone reach at most 40 ("risky": a code at checkout when OTP is on, never "suspect"); a busy address (more than 20 places) with any other signal is a suspect. Polling is scored per tab, and a poll at or after the time the server told the page is never "fast". IPv6 hosting ranges are listed. The crowd test adds carrier-grade NAT (60 phones on 10 addresses of one /24), one popular phone model shared by a fifth of the crowd and fans with two tabs: 0 of 10,000 fans and 950 of 1,000 bots are suspects.
- **Guest identities (D-f).** Guest identities are free to make, so new guest places are limited per address (5 a minute; previews more) at the gateway, which has Redis (the web app has no Redis client: minting itself is not limited, recorded). A sale can be set to signed-in fans only (`accountsOnly`, a Studio toggle): guests are asked to sign in before joining. Residual risk: an attacker with many addresses can still make guest places; caps per person and OTP remain the backstop, and signed-in-only sales close it.
- **Caps (D-g).** An account with a verified email counts under both its account and its normalised email, so a guest order with that email shares its cap; tickets paid before the event had sale moments (no ledger row) count too, by buyer id or email. The ledger has a row per identity.
- **Redis capacity (D-h).** One hash per fan (entry, signals, passes), short keys (`qf`, `qo`, ... on a tag with the first 12 hex digits of the sale id), keyed hashes of addresses (never the address) with a 1 h life. Measured: about 1.35 kB per waiting fan including rate-limit counters and the audit entry until it is copied (budget 1.6 kB: 100,000 fans in about 160 MB). Production Redis: maxmemory 1 GiB, requests equal to limits (1,400 Mi), `noeviction`, the append-only file on a 5 Gi `do-block-storage` volume (the deploy replaces the StatefulSet once to add it; previews keep an emptyDir). Recovery after a lost Redis: see Context.
- **Polling cost (D-i).** The first poll after opening is spread over 2 to 15 s and re-joins before the pre-queue opens carry a few seconds of jitter; status calls carry only the queue token (no core token, so the web route reads no session); joins use Better Auth's session cookie cache (already on).
- **Honest estimate (D-j).** The estimate divides the fans ahead by the admissions actually made in the last 60 s (the shopping limit can hold them below the dial); it is hidden while paused. A suspect sees its drawn position less everyone let in since, which only goes down.
- **Demo safety (D-k).** The opening is within a year of now; a sale moment can be deleted until its pre-queue opens; at most 10 per event; Studio says when the organiser is the shared sample.
- **Settings races (D-l).** A Studio save carries the version it was made on (`baseVersion`): a save on top of a newer one is refused with `EPS-SALE-STALE` and the page shows the newer version. Pause, rate and shopping limit also have a live patch applied on top of whatever is current. The Redis copy carries a generation (the newest version or deletion) and is written only when it is not older than the one there.
- **Inventory.** A resize takes the instance's commit lock and every capped commit checks the capacity under it (no "over" shortcut). The reconciler absorbs only units of paid orders whose hold record the store lost (a paid order whose hold is still held is committed by the orphan sweep). The shard sweeper lapses a held record 5 s past its deadline whose order does not exist. A hold that finds no free stock but lapsed entries lapses them at once and tries again; a walk that found nothing free sets a 250 ms "empty" hint so failing holds do not walk every shard.
- **Operations.** The worker runs two replicas (one on previews): the singleton loops (sweepers, reconcilers, instalments, relay, purge, queue audit, sales refresher) run on the holder of a worker lease (10 s, renewed every 3 s), the order-event consumers on both. The core ends a pass and its cap entries itself when an order is paid or cancelled (the consumer is the backstop). The admitter ticks at most 8 sales at once, drops a sale (and its gauges) once it is deleted or 72 hours past opening; `events_with_sales` returns the nearest openings first, a page of 200 at a time, a year ahead. New metrics: `eps_http_request_seconds{service,route}` (ids folded), `eps_queue_admit_tick_seconds`, `eps_queue_join_rate_limited_total{range}`, `eps_queue_passes_unfetched_total`. Per-sale gauges are reported by every gateway from the same Redis: dashboards take the max per sale, never the sum. An admission whose pass is not collected within 60 s frees its shopping place.
- **OTP.** A check with no code sent, or after every try was used, records nothing; code calls are limited to 10 a minute per order and 30 per client. The OTP is a simulated flow (the code is shown on screen) and is documented as such; the resume link pays a held order by design (it is a capability bounded by the hold).
- **Keys and configuration.** Identity hashes of places and caps are keyed (an HMAC with a key derived from `SNAPSHOT_CACHE_KEY`), so a stored hash cannot be matched against guessed emails. Every service needs `EPS_ENV`; outside `dev` an empty `SNAPSHOT_CACHE_KEY` stops it. `loadkit` runs only with `EPS_ENV=dev` set explicitly. Stray binaries are git-ignored.
- **Personal data.** The waiting room keeps, per fan and for at most two days: a keyed hash of the account or guest id, a hash of the session secret, a keyed hash of the address (1 h for the address sets), behaviour signals and a score; Postgres keeps queue audit rows (identity hash, times, position, score) and cap ledger rows (identity hash, tickets). They go into the privacy notice and the step 7 deletion path (delete by identity hash per organiser).
- **Measured, not claimed.** The "Throughput" and "Opening spike" scenarios are targets measured on the cluster in steps 6 and 12; the sandbox numbers are recorded in the k6 scripts and the PR.

## Risks / Trade-offs

- [One Redis instance does not gain throughput from shards] → shards remove the single hot key and are cluster-ready by hash tag; one instance handles about 100k operations per second, enough for the demo; Redis Cluster is the scale-up path in the vision.
- [Session-bound passes can still be copied with their session secret] → stops casual sharing and replay; a WebCrypto non-extractable key (DPoP-style) is the next step.
- [Scoring may flag a real fan] → address signals alone reach at most risky, shared prints count only past 20 people, polling is scored per tab against the server's own schedule; risky fans get an OTP, not a block; the crowd test and the simulator measure false positives.
- [Shuffle of a large pre-queue takes time] → batches of 5,000 keep 100k under 2 s; status shows "shuffling" meanwhile.
- [Reconciler races live traffic] → two reads of Postgres around the snapshot and two-round confirmation avoid acting on transient drift.

## Migration Plan

New sales-schema tables and Redis keys only. The step 4 single-counter inventory is replaced behind its interface (its Redis state is taken over from Postgres on first use, D-13); the step 4 hold tests run unchanged against the sharded engine. Production's Redis moves onto a volume once (the deploy replaces its StatefulSet). Rollback: revert the release; sales without the queue keep working without passes.

## Open Questions

None that change the specs. The data-centre address list is a small static list for the prototype.
