# Design

## Context

See proposal.md (Why). Step 0 gives the Go module (`cmd/core`, `cmd/worker`, `cmd/fakepsp`), JWT verification through JWKS (U1), guest identities, `organiser_id` on every row with row-level security (U2), and the sales schema owned by Go (P2). Step 1 gives fee policy, buying rules and instalment settings; step 3 gives immutable sale snapshots (P1) with all-in prices. This step adds the money path. It must hold up under the step 6 simulator (about 170 paid orders per second at peak) and must never touch real card data or real money.

## Goals / Non-Goals

**Goals:**
- Every order and payment attempt is a persisted state machine; every transition is explicit and every failure has an error code (O3).
- At most one successful charge per order, whatever the retries, double clicks, timeouts or failovers (D6).
- Failover from A to B decided by a shared circuit breaker, invisible to the fan (D5).
- Fake providers that behave like real PSPs across a network boundary (O4).
- A wallet whose balance is always the sum of its journal lines (O6).
- An outbox that never loses an event (P3).

**Non-Goals:**
- The queue pass, sharded stock, per-person caps across sale moments and risk-based OTP (step 5). This step's holds use a simple single-counter inventory behind the interface step 5 replaces.
- Ticket issuing and the rotating QR (step 7): this step emits `order.paid`; a stub consumer marks orders issued until step 7 lands.
- Squad group holds (step 8), refunds as an organiser action (G6, Vision), real e-money handling.

## Decisions

### D-1 Order state machine (O3, D3)

States: `held`, `paying`, `payment_failed`, `paid`, `issued`, `expired`, `cancelled`, `refunded`.

| From | Event | To | Notes |
|---|---|---|---|
| (new) | hold succeeds | `held` | stock taken, `hold_expires_at = now + hold_minutes` |
| `held`, `payment_failed` | payment attempt created | `paying` | hold extended so it ends no earlier than attempt deadline + 30 s |
| `paying` | attempt `succeeded` | `paid` | inventory commit and sale rows in one Postgres transaction with the outbox row |
| `paying` | attempt `declined` / `failed` and no failover left | `payment_failed` | hold still live; fan may retry |
| `paying` | attempt `failed` before the provider accepted, other provider available | `paying` | a new attempt on B (D-4); invisible to the fan |
| `held`, `payment_failed` | hold deadline passes (sweeper) | `expired` | stock released |
| `held`, `payment_failed` | fan cancels | `cancelled` | stock released |
| `paid` | `ticket.issued` consumed | `issued` | step 7; stub consumer until then |
| `expired`, `cancelled` | late attempt `succeeded` | `paid` if stock can be re-taken, else `refunded` | refund through the same provider with its own idempotency key |

`paying` never expires while an attempt is unresolved; an attempt has a hard deadline of 120 s, after which the reconciler resolves it (D-5). Transitions run as `UPDATE ... WHERE id = $1 AND state = ANY($allowed) RETURNING`, so two racing transitions cannot both win; the loser re-reads and returns the current truth.

### D-2 Payment attempt state machine

States: `created`, `submitted`, `challenge_required`, `succeeded`, `declined`, `failed`, `unknown`, `refund_pending`, `refunded`.

- `created → submitted` when the provider returns 2xx with a charge id.
- `created → failed` when the provider returns 5xx, a connection error, or the breaker is open, all before a charge id exists. Only this kind of failure allows failover.
- `created → unknown` on a timeout (3 s) with no response: the provider may have charged.
- `submitted → challenge_required` when the provider asks for the simulated 3-D Secure / OTP code; `challenge_required → submitted` on code entry.
- `submitted | challenge_required → succeeded | declined` from the signed webhook or a status query.
- `unknown → submitted | succeeded | declined | failed` after a status query by idempotency key (a provider "no such charge" means `failed`, and only then may another provider be tried).
- `succeeded → refund_pending → refunded` for late payments on expired orders and for a duplicate success.
A partial unique index allows at most one non-terminal attempt per order and at most one `succeeded` attempt per order; a second success (should never happen) is refunded automatically and alerts.

### D-3 Idempotency keys (D6)

- **Client to core:** every `POST /v1/orders/{id}/payments` and `POST /v1/wallet/topups` carries `Idempotency-Key` (UUID v4 created when the fan presses Pay, reused on retry and double click). Stored in `idempotency_keys(organiser_id, scope, key, request_hash, response, created_at)` with a unique key on `(scope, key)`, kept 24 h. Same key and same body: the stored response is returned (HTTP 200 with `Idempotent-Replayed: true`); same key, different body: 422 `EPS-IDEMPOTENCY-MISMATCH`; same key still in flight: 409 `EPS-IN-PROGRESS` with `Retry-After: 1`.
- **Hold creation:** `POST /v1/orders` also takes `Idempotency-Key`, so a double click does not take stock twice.
- **Core to provider:** each attempt's id (ULID) is sent as the provider's idempotency key; the fake provider returns the original charge for a repeated key. Failover to B creates a new attempt and a new key only after A's attempt is known not to have charged (D-2).

### D-4 Circuit breaker and failover (D5)

- One breaker per provider, shared by every core replica through Redis: per-second buckets `pay:{psp:a}:win:<unix_s>` (hash: `ok`, `err`) with 60 s TTL; the breaker reads the last 20 s.
- **Counts as failure:** 5xx, connection error, timeout. **Not a failure:** a decline (it is the card's answer, not the provider's health).
- **Opens** when, over the last 20 s, there are at least 20 calls and the failure rate is 50% or more, or after 5 consecutive failures. State is written to `pay:{psp:a}:breaker` (`closed|open|half_open`, `opened_at`) so all replicas agree within 1 s (local cache 1 s).
- **Open** for 15 s, then **half-open**: up to 5 probe attempts; 4 successes close it, any failure reopens it for 30 s (doubling, max 120 s).
- **Routing:** the organiser's preferred provider (A by default) while its breaker is closed or half-open with probe slots; otherwise the other provider. An attempt that fails before acceptance on A is retried once on B within the same Pay press. If both breakers are open the order stays `held` or returns to `payment_failed` with `EPS-PAY-PROVIDERS-BUSY` and "Your tickets are still held".
- Every failover emits `payment.failover` (from, to, reason) to the outbox, and breaker changes emit `payment.provider_state`.

### D-5 Webhooks and reconciliation

- The fake provider posts to `POST /v1/webhooks/psp/{provider}` on the core (in-cluster service URL) with `EPS-Signature: t=<unix>,v1=<hex HMAC-SHA256(secret, t + "." + body)>`; the core rejects signatures older than 5 minutes or invalid, and dedups by provider event id (`webhook_events` unique key).
- Webhooks are retried by the provider with exponential back-off (1, 2, 4 ... 64 s, 8 tries).
- The worker's payment reconciler queries the provider for every attempt in `submitted`, `challenge_required` or `unknown` older than 20 s, so a dropped webhook still resolves the order (and the status page never lies).

### D-6 Fake provider API (O4, D4, D13)

`cmd/fakepsp`, deployed twice (`psp-a`, `psp-b`, env `PROVIDER_ID`), state in memory plus Redis for charges (so restarts keep charges).

- `POST /v1/charges` `{amount_minor, currency, idempotency_key, method: hosted_card|apple_pay|google_pay|saved_card, payment_method_token?, customer_ref?, save_card?, return_url, webhook_url, metadata}` → `201 {charge_id, status: pending|requires_action|declined, action_url?}`.
- `GET /v1/charges/{id}`, `GET /v1/charges?idempotency_key=` for status checks.
- `POST /v1/charges/{id}/challenge {code}`: the simulated 3-D Secure / OTP step; the code is shown on the provider's page ("Simulated bank code: 123456").
- `POST /v1/refunds {charge_id, idempotency_key}`.
- `GET /pay/{charge_id}`: the hosted page. It shows test outcomes as buttons ("Test card: approve", "decline: insufficient funds", "decline: 3-D Secure failed", "cancel at bank") and simulated Apple Pay and Google Pay sheets. **It has no card number field**; nothing card-like is ever typed or sent to EPS.
- Saved cards: `save_card: true` on a succeeded charge returns `payment_method_token` (`pm_<provider>_<random>`), brand and a test label ("Test Visa •••• 4242"); EPS stores only the token, the provider and the label.
- Knobs, `PUT /admin/knobs` (admin bearer token, in-cluster only, also proxied through the core for the Studio panel): `failure_rate` (0–1, 503 before acceptance), `decline_rate` (0–1), `latency_ms` and `latency_jitter_ms`, `outage` (on: every call hangs until the client timeout or returns 503, `outage_mode: hang|503`), `timeout_rate` (accepts the charge but never answers the create call), `webhook_delay_ms`, `webhook_drop_rate`, `challenge_rate` (share of charges that require the code). `GET /admin/stats` returns counters. Defaults: all zero, latency 150 ms ± 100.
- Metrics: `fakepsp_charges_total{provider,outcome}`, request latency histogram.

### D-7 Holds and hold TTL (D2, O2)

- Hold length comes from the snapshot's buying rules (`hold_minutes`, default 8, allowed 5–15). EPS uses 5 minutes and Ticketmaster 10; 8 leaves room for the provider challenge.
- Step 4 inventory: `inv:{<org>:<event>}:<pool>` one integer per pool, reserved by one Lua script that checks every pool in the order and decrements all or none, recording the hold in `hold:{<org>:<event>}:<hold_id>` (hash pool → qty) and in the expiry index `holds:exp:{<org>:<event>}` (sorted set, score = deadline ms). Release and commit are Lua scripts that act only if the hold is still in state `held`, so a sweep racing a commit cannot double count. Step 5 replaces this behind the same `Reserve / Extend / Commit / Release` interface.
- The worker's sweeper runs every second, pops due holds from the expiry index and releases them, then moves the order to `expired`.
- One live held order per buyer per sale (account or guest session), so one person cannot pile up holds.

### D-8 Resume link (D7)

`/s/<brand>/orders/<order_id>/resume?t=<token>` where the token is `base64url(order_id | hold_expires_at | nonce) . HMAC-SHA256` with a resume key held by the core. It is valid only until the hold deadline, opens the payment step if the order is `held` or `payment_failed`, and otherwise shows the order's true state. It is shown on the status page with a copy button and in the confirmation email (recorded in the dev mailbox if no email sender is configured).

### D-9 Guest checkout and claim (D8)

A guest buys with an email and gets an order token (signed, order-scoped, 30-day life) that opens the order status page. A signed-in account whose email is verified through Better Auth can claim guest orders with the same normalised email; the claim moves `buyer_id` to the account in one transaction and emits `order.claimed`. A claim never crosses organisers' data rules: it only re-points orders the person already owns by email.

### D-10 Instalments (D11, C9)

- Plans come from the snapshot's instalment settings: deposit (fixed or %), number of monthly payments, day of month.
- The schedule is computed in integer minor units: deposit first, then equal parts, the rounding remainder on the last part; the parts always add up to the all-in total.
- The deposit is a normal payment attempt with `save_card: true`; an instalment order cannot be paid with a wallet or a one-off method that cannot be saved. The order becomes `paid` when the deposit succeeds; the plan is `active`.
- The worker's instalment runner charges due parts with the saved token (idempotency key `inst:<plan>:<part>`). Statuses: `scheduled`, `paid`, `overdue` (failed charge, grace 7 days), `cancelled`. A Studio and fan "simulate next due date" action moves the clock for one plan so the demo can show it.

### D-11 Wallet ledger (O6, D12)

- Accounts in `wallet_accounts(organiser_id, id, kind, owner, currency, balance_minor)`: `fan_balance:<fan>`, `fan_season_credit:<fan>`, `psp_clearing:<provider>`, `organiser_sales:<organiser>`, `stand_sales:<organiser>`. The wallet is per fan per organiser (closed loop, like the NIBIRU card).
- Every money move is a `journal_entries` row with two or more `journal_lines` (account, signed amount) that add up to zero per currency; a deferred constraint trigger checks the sum. `balance_minor` is a cached column updated in the same transaction; a `CHECK (balance_minor >= 0)` on fan accounts makes overspend impossible.
- Top-up: a provider charge with `purpose=topup`; on success, debit `psp_clearing:<provider>`, credit `fan_balance`.
- Pay from wallet: one transaction that locks the fan account row, writes the entry (debit `fan_balance` then `fan_season_credit` second or first per organiser setting, credit `organiser_sales`), commits the hold and moves the order to `paid`. No provider round trip, so it is the fastest path in a drop.
- Stand payment (simulated): a stand screen debits the balance to `stand_sales`.
- Season credit: an organiser "close season" action moves each `fan_balance` to `fan_season_credit` with an expiry date.
- Journal lines are append-only; corrections are reversing entries. A ledger check (Σ lines = cached balances) runs in tests and as a worker job.
- **Regulation:** a real stored balance is e-money. Production needs an e-money licence or a regulated partner (an EMI) holding the funds, with EPS keeping only the ledger view; the prototype shows "Simulated balance, no real money" on every wallet screen.

### D-12 Outbox and streams (P3)

- `sales.outbox(id ULID, organiser_id, event_id, topic, type, payload jsonb, created_at, relayed_at)`, written in the same transaction as the state change.
- The worker's relay claims rows with `FOR UPDATE SKIP LOCKED LIMIT 500` every 200 ms (and on `LISTEN outbox`), `XADD`s them to `stream:{<topic>}` with the outbox id in the entry, then sets `relayed_at`. A crash between XADD and the update re-sends; consumers dedup by event id (at-least-once, effectively-once).
- Envelope: `{id, type, version, organiser_id, event_id, sale_id, occurred_at, payload}`. Topics: `orders`, `payments`, `wallet`. Streams trimmed with `MAXLEN ~ 1000000`; relayed rows deleted after 7 days.
- Consumer groups: `issuer` (stub now, step 7), `dashboard` (step 6), `webhooks` (step 10).

### D-13 Checkout questions (D9, Thin)

The organiser adds up to 5 questions (short text, single choice, checkbox consent; required or optional) into the locked checkout's question and consent slots. They are copied into the snapshot at publish (assumption on step 3, see Open Questions), so the checkout shows the questions of the version it sells; answers are stored per order.

### D-14 Error codes

Every failure returns `{code, message_key, retryable}` with codes such as `EPS-HOLD-SOLD-OUT`, `EPS-HOLD-EXPIRED`, `EPS-PAY-DECLINED`, `EPS-PAY-3DS-FAILED`, `EPS-PAY-PROVIDERS-BUSY`, `EPS-WALLET-INSUFFICIENT`, `EPS-IDEMPOTENCY-MISMATCH`; the screen shows the code so the fan can quote it to support.

## Risks / Trade-offs

- [A provider charges but times out, then B also charges] → B is only tried after A's attempt is known not to exist (D-2); the unique index on succeeded attempts plus the auto-refund catches anything left.
- [Breaker state in Redis adds a hop] → cached locally for 1 s; a Redis outage falls back to per-replica breakers.
- [The single-counter inventory is a hot key] → acceptable for this step; step 5 shards it behind the same interface.
- [Late webhooks after expiry sell stock someone else now holds] → late success re-reserves only free stock, otherwise refunds; never oversells.
- [Wallet row lock contention for one fan] → one fan pays once per drop; contention is per fan, not global.
- [Simulated Apple/Google Pay is not the real API] → labelled "simulated"; the adapter shape matches real wallet tokens, so a real provider is a swap.

## Migration Plan

New tables in the sales schema through the Go migrations from step 0; nothing existing changes. psp-a and psp-b deploy as new Helm releases. Rollback: revert the release; tables are additive.

## Open Questions

- From step 3 (as built): the snapshot's `rules.holdMinutes` (5 to 15, default 8, from `events.hold_minutes`) and `rules.checkoutQuestions` (`afterContact` and `consents` slot ids, from the selling shop's storefront document) are what the core uses; no control-plane endpoint is needed. Holds clamp to the live snapshot's instance capacities and treat over-capacity instances as sold out. The core re-prices every order from snapshot `v`, refuses hidden products and off-sale events (the published-only read returns nothing for them) and re-applies the per-order and per-account caps: the shop's checkout URL is never trusted.
- Assumption on step 0: an email sender or dev mailbox exists for the resume link and the confirmation email; if not, the links are shown on screen only.
