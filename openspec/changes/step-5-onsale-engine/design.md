# Design

## Context

See proposal.md (Why). Step 4 provides orders, holds behind an inventory interface (`Reserve`, `Extend`, `Commit`, `Release`), the payment state machine and the outbox. Redis runs inside the cluster as one primary with a replica; keys are prefixed by organiser and event (U2) and use hash tags so they can move to Redis Cluster later. The waiting-room page is served by Vercel as a static shell; all dynamic calls go to the Go gateway on DOKS. Sizing target from festival-tech section 10: about 50k arrivals per second at opening at the edge, and a controlled 200 to 500 checkouts per second at the core.

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

## Risks / Trade-offs

- [One Redis instance does not gain throughput from shards] → shards remove the single hot key and are cluster-ready by hash tag; one instance handles about 100k operations per second, enough for the demo; Redis Cluster is the scale-up path in the vision.
- [Session-bound passes can still be copied with their session secret] → stops casual sharing and replay; a WebCrypto non-extractable key (DPoP-style) is the next step.
- [Scoring may flag a real fan] → thresholds are additive and no single human behaviour reaches 70; risky fans get an OTP, not a block; the simulator measures false positives.
- [Shuffle of a large pre-queue takes time] → batches of 5,000 keep 100k under 2 s; status shows "shuffling" meanwhile.
- [Reconciler races live traffic] → two reads of Postgres around the snapshot and two-round confirmation avoid acting on transient drift.

## Migration Plan

New sales-schema tables and Redis keys only. The step 4 single-counter inventory is replaced behind its interface; the step 4 hold tests run unchanged against the sharded engine. Rollback: revert the release; sales without the queue keep working without passes.

## Open Questions

None that change the specs. The data-centre address list is a small static list for the prototype.
