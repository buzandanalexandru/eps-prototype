# Design: step 10, organiser growth

## Context

See `proposal.md` for why. This step builds on: the immutable sale snapshot (P1) and storefront at `/s/<brand>` (step 3); holds, order states and the fee policy applied to every order (steps 1 and 4, O3, A6); the outbox and Redis Streams with `order.paid`, `ticket.issued`, `snapshot.published` (P3); the simulator that sends real orders through the gateway and core (R3); and row-level security on every table (U2). Next.js writes only the control schema, Go only the sales schema (P2).

## Goals / Non-Goals

**Goals:**
- One attribution path, from link or code to order to leaderboard, that is real end to end, so the Act 2 leaderboard moves with simulator sales.
- A dashboard whose numbers reconcile exactly with the orders in the sales schema.
- Thin screens that are clickable and honest about what is simulated.

**Non-Goals:**
- Real merchant onboarding, KYC, bank transfers or invoicing (no real payments).
- Email or SMS campaigns, ad pixels, a public REST API beyond webhooks (named in the vision).
- Cashback paid to fans; promoter payouts are a computed list only.

## Decisions

### D1 · Codes and channels compile into the sale snapshot, with a small live code set
Channels (shops), their product subsets and access-code requirements are part of the sale snapshot (P1), because they shape what is sold. Promo, influencer and access codes change during a sale, so they are published as a separate versioned **code set** per event (control schema, compiled on save, emitted as `codes.published` on the outbox and cached by core in Redis under `org:<id>:event:<id>:codes`). Core validates a code at hold time against the cached code set; it never reads draft tables.
*Alternative:* republish the full snapshot for every new code. Rejected: a new influencer code mid-sale would bump the storefront version.

### D2 · Attribution flows checkout → order → stream → roll-up
- A tracked link is `/s/<brand>[/<shop>]?ref=<link-slug>`. The storefront stores the ref in a first-party cookie for 7 days (last click wins) and sends it with the hold request; a typed code overrides the ref.
- Core resolves ref or code to an `attribution {promoter_id, code_id | link_id, source}` at hold time, stores it on the hold and copies it onto the order (sales schema). The discount, if any, is applied before the all-in price is shown in the cart (D1 rule of step 3: no new fee appears at checkout).
- `order.paid` carries `attribution`, `channel_id`, `gross`, `fee`, `net` and `simulated`. The worker's `growth-rollups` consumer group updates Redis hashes (`org:<id>:event:<id>:promoter:<id>` with orders, tickets, net) and an hourly Postgres roll-up table. Roll-ups are idempotent on the event ID (a processed-events set), so a redelivered event never double counts.
- The leaderboard and dashboard read Redis for live numbers and Postgres for history; both push updates to the browser over SSE (R2), throttled to one update per second per screen.
*Alternative:* query orders on every dashboard load. Rejected at 100k-fan rates; the roll-up is also what the vision's scale story describes.

### D3 · Dashboard money
Gross is the all-in price paid; fee is the platform fee by the fee policy at sale time (from the snapshot recorded on the order); net = gross − fee − promoter discount effect already inside gross. Amounts are integers in minor units per currency; no currency mixing in one total. Days are bucketed in Europe/Bucharest. Simulated orders carry `simulated=true` and the dashboard shows a "Simulated" filter (on by default for the sample NIBIRU event, which is labelled sample data). A reconciliation test sums orders and compares to the roll-up.

### D4 · Webhooks: signing and retries
- Endpoints (URL, chosen event types, secret) live in the control schema; the secret is generated server-side, shown once and stored encrypted. HTTPS only; private, loopback and link-local addresses are refused at save and at send (DNS resolved and checked before each delivery, against SSRF).
- The worker's `webhooks` consumer group creates a delivery row per (event, endpoint) and POSTs JSON `{id, type, created_at, organiser_id, data}` with headers `EPS-Webhook-Id`, `EPS-Webhook-Timestamp` and `EPS-Signature: v1=<hex HMAC-SHA256(secret, timestamp + "." + body)>`. 5-second timeout; any 2xx is success.
- Retries with exponential backoff and jitter: 30 s, 2 min, 10 min, 1 h, 6 h (6 attempts), then `failed`. "Redeliver" in the log makes a new attempt with the same event ID, so receivers deduplicate on `EPS-Webhook-Id`. After 20 consecutive failures the endpoint is disabled and flagged.
- A built-in demo receiver at `/api/dev/webhook-sink/<endpoint>` (sample organiser only) shows verified deliveries in the video.

### D5 · Audience export
The buyers table is a per-organiser view over paid orders (guest and account buyers), with consent flags captured at checkout (step 4 consent slots). CSV export includes only buyers with marketing consent unless the owner picks "all buyers (for service messages)", which is logged. Cells starting with `= + - @` are prefixed with `'` (CSV injection). Deleted users are excluded. Export is owner and admin only (A3).

### D6 · Marketplace and payouts
`/marketplace` is a server-rendered page over published snapshots of organisers with `marketplace_listed=true` (default on, a switch in settings), cached until the next `snapshot.published`. Payment accounts: a choice between "Connect your own merchant account" (a simulated connect flow against the fake provider, no bank or card details collected) and "EPS as merchant"; the payouts list is computed from paid net per week with status `scheduled` / `paid (simulated)`.

### D7 · Isolation
All new tables carry `organiser_id` with RLS; Redis keys are prefixed `org:<id>`; promoter, code and webhook IDs from another organiser resolve as not found. Promoter logins are out of scope: the organiser shares a read-only promoter stats link (signed token, revocable).

## Risks / Trade-offs

- [Roll-ups drift from orders] → idempotent consumers, reconciliation test, and a nightly recompute job in the worker.
- [Webhook receivers slow down the worker] → separate consumer group, per-endpoint concurrency of 2, timeouts; deliveries never block issuing.
- [Code brute force at the storefront] → code checks are rate limited per session and count toward bot scoring (step 5); codes are at least 6 characters.
- [SSE fan-out on many open dashboards] → throttled server push, one aggregator stream per event.

## Migration Plan

New tables only (control and sales migrations). Existing orders have null attribution and the default channel; the default channel is created for every existing event by the migration. Rollback: drop the new tables; nothing in steps 0–9 reads them.
