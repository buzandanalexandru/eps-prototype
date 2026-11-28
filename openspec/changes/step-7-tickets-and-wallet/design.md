# Design

## Context

See proposal.md for the why. This step builds on interfaces from earlier steps, assumed as follows:

- Step 4 writes `order.paid` to the outbox in the same transaction that marks an order paid (P3, O3); the event carries the order ID, organiser, event, snapshot version, buyer (user ID or guest ID) and its lines (product, quantity, all-in unit price). For an instalment order it fires when the first instalment is confirmed, and step 4 records the plan's state (scheduled, paid, overdue, grace).
- Step 0 gives Better Auth sessions, short-lived JWTs that Go verifies (U1), guest buyers with a claim flow, and row-level security keyed by organiser (U2). Step 1 gives the box office role (A3) and the catalogue's product flags: personalisation required with a deadline, check-in deadline and late check-in fee, pick-the-day-later (C3).
- Step 2 gives the ticket and email design from the brand kit (B12); step 3 the sale snapshot each order was sold under (P1).

## Goals / Non-Goals

**Goals:**
- A code that cannot be reused from a screenshot, checked on a phone with no network.
- One reissue mechanism (generations plus revocation feed) that steps 8 and 9 use for claim, transfer, resale and upgrades.
- A trip wallet that always tells the fan the next thing to do.

**Non-Goals:**
- Device binding of tickets (DICE, SafeTix). Named in the vision; the prototype binds codes to the signed-in holder only.
- A delayed QR release before doors (DICE). The code is available as soon as the ticket is issued, so the demo can show it; a per-event release time is a later setting.
- Real Apple or Google Wallet passes (need signing accounts), NFC entry, exit and re-entry scans.
- Transfer, resale and upgrades themselves (step 9) and squad claim (step 8); this step only provides the reissue primitive.

## Decisions

### D1 · Issuing from the stream, idempotently (P3, O3)
The worker's consumer group reads `order.paid` and, in one sales-schema transaction, inserts one ticket per unit with a unique key `(order_id, line_no, unit_no)`, then writes `ticket.issued` to the outbox. Redelivery hits the unique key and is acknowledged without new rows. The status page and the trip wallet show "Issuing your tickets" between `order.paid` and `ticket.issued`, never a blind success.
*Alternative:* issue synchronously in the payment webhook handler. Rejected: couples issuing to the payment path at peak load, and the outbox already guarantees delivery.

### D2 · Ticket token format (O5)
The code is `E1.` followed by base64url of a 104-byte binary token:

| Bytes | Field |
|---|---|
| 1 | format version (1) |
| 1 | key ID |
| 16 | event ID (UUID) |
| 16 | ticket ID (UUID) |
| 2 | generation (uint16) |
| 4 | time window = floor(unix seconds / 15) (uint32) |
| 64 | Ed25519 signature over the 40 bytes above |

About 143 characters, which fits a version 8 QR at medium error correction and scans well from a phone screen. No personal data is in the code; the door page shows the holder's name only from its synced manifest (D6).
*Alternatives:* a JWT (too long for a fast-scanning QR); HMAC with a shared secret on every scanner (any scanner could mint tickets; rejected in O5).

### D3 · Signing keys
CI generates the Ed25519 key pair once per environment (`openssl genpkey -algorithm ed25519`) into the core API's Kubernetes secret with a one-byte key ID; previews get their own key. Only the core API holds the private half. Public keys are served at `GET /v1/ticket-keys` (key ID, public key, valid from, valid to) and cached by door devices. Rotation adds a new key ID for signing and keeps the old public key published until the last event signed with it has ended. The private key never reaches Vercel or the browser.

### D4 · Codes to the holder: a short in-memory buffer
The fan's ticket page calls `GET /v1/tickets/{id}/codes`, which returns signed codes for the current window and the next 39 (10 minutes), only to the ticket's current holder, only for a valid generation. The page keeps them in memory (never localStorage or the service-worker cache), shows the one for the current window, and refetches when fewer than 5 minutes remain. Without a network after the buffer runs out it shows "Reconnect to show your code", never a stale code. The buffer length is the longest time a revoked holder could still present codes, which is why it matches the door's stale-sync warning (D7).
*Alternative:* sign on every 15-second tick. Rejected: the fan's phone at a crowded gate often has no signal.

### D5 · Clock skew
Both pages correct their own clock: at each sync they read the server time (`Date` header of the keys and revocations responses) and keep an offset. The fan page picks the window from corrected time. The door accepts windows `w-1, w, w+1` around its corrected current window `w`, so a code stays valid for at most 15 seconds after its own window ends (30 seconds in all) and a phone up to 15 seconds fast is not refused. A code from window `w-2` or earlier is rejected as "Code expired: this looks like a screenshot or an old code." A door device that has never synced its clock shows a warning and uses the device clock.

### D6 · Revocations and the offline door
A reissue (claim, transfer, resale, upgrade, admin cancel) increments the ticket's generation in the same transaction and inserts a revocation row `(event_id, ticket_id, min_valid_generation, reason, seq)`; suspension for an overdue instalment plan is a revocation with reason `suspended` that a later row lifts. Door devices fetch `GET /v1/events/{id}/door-sync?since=<seq>`: new revocations, entries recorded by other devices, and a small manifest (ticket ID, product, holder display name if personalised). Devices sync every 30 seconds while online and keep everything in IndexedDB.
Offline, the door still rejects old generations it knows about; for one it has not heard about, the old holder cannot fetch codes for it anymore (D4), so the exposure is bounded by the 10-minute buffer and the time since the device's last sync.

### D7 · Door page as an installable offline app
`/door/[event]` is a Next.js page plus a service worker that caches its shell. Staff sign in (owner, admin or box office of that organiser, A3) and press "Prepare this device", which downloads keys, the event's sync package and the server time while online. Scanning uses the camera through the BarcodeDetector API where present, otherwise a small JS QR decoder. Signature checks use WebCrypto Ed25519, with a small audited Ed25519 library as the fallback for browsers without it. Scans are written locally first, then uploaded idempotently by scan ID. The header shows "Last synced N min ago" and turns amber after 10 minutes.
*Dependencies to confirm at build time (small utilities):* a QR encoder for the fan page, a QR decoder fallback, an Ed25519 fallback.

### D8 · Entry and double scans
A device rejects a second valid scan of the same ticket it has already admitted ("Already scanned at 21:04 at Gate B"). Across devices, entries arrive through door-sync; if two offline devices admitted the same ticket, the server keeps the first by device time and records a conflict that the door page and the organiser see. This is honest about the offline trade-off instead of hiding it.

### D9 · Trip wallet data sources
The timeline is assembled in Next.js server components from one Go call, `GET /v1/me/trip`, which returns the fan's tickets (all organisers, filtered by holder from the JWT, not by organiser), each with its snapshot product type (pass, day ticket, add-on, table, time slot, transport), dates from the snapshot, personalisation and check-in state, and the related instalment plan state from step 4. Items are grouped by event and ordered by start time; items without their own time sit at the event's start. Each event card uses that brand's tokens. Next actions come from one deterministic rule list, earliest deadline first: pay an overdue instalment, personalise, pick the day, check in, then show the code on the event day.
*Alternative:* the web app queries the sales schema directly. Rejected by P2 (each side writes and owns its own schema).

### D10 · Thin parts
- **Personalisation:** name (and the fields the product asks for) per ticket until the product's deadline; editable until then, locked after.
- **Online check-in:** free until the check-in deadline; after it, "Late check-in" charges the product's late fee through the step 4 payment flow (fake providers or EPS Wallet) and completes on `order.paid`.
- **Pick the day later:** a flexible day ticket shows "Pick your day" from the snapshot's days with stock; picking reissues the ticket (new generation) for that day.
- **Signed guest link:** `/t/<order>.<signature>` (HMAC with a server secret, order-scoped) opens the guest's trip wallet for that order without signing in; the printable ticket shows details and "Open the live code on your phone", never a static code.
- **Wallet pass preview:** buttons open a branded preview of the pass with a note that real passes need Apple and Google signing accounts. Google's pass format supports rotating barcodes, so the vision names that route.

### D11 · As built (step 7 implementation)

Decisions made while building, on top of D1 to D10 and what steps 3 to 6 actually built:

- **Issuer (D1).** The worker's `issuer` consumer group (the same group step 4's stub used, so its position carries over) now runs `tickets.Service.HandleEvent`: `order.paid` issues, `order.instalment_overdue` / `order.instalment_paid` suspend and lift, `order.cancelled` / `order.refunded` cancel, `order.claimed` moves a guest's tickets to the claiming account. Every handler **recomputes from the order's current truth under its row lock** (`FOR UPDATE`), so duplicate, concurrent (XAUTOCLAIM after 30 s, two worker replicas) and out-of-order deliveries converge. Issuing is one statement (`INSERT … SELECT FROM unnest(...) ON CONFLICT (organiser_id, order_id, line_no, unit_no) DO NOTHING`), the move `paid → issued` and **one `order.issued` event per order listing the new ticket ids** in the same transaction (one event per order instead of one per ticket: half the outbox rows at the on-sale peak; step 4's consumers already knew `order.issued`). The consumer handles up to 8 entries of a read in parallel (`outbox.Consumer.Parallel`) on its own pool (`WORKER_POOL_ISSUER`, 4). Rehearsal orders are never issued (`checkout.Issuable`, step 6 D-a), also not by the backstop.
- **Backstop and backfill.** The worker's lease holder issues paid orders still without tickets 30 s after payment (a lost event), 200 every 15 s (`sales.paid_orders_unissued`, a partial index on paid orders), and once per lease walks the orders step 4's stub marked `issued` without tickets (`sales.issued_orders_without_tickets`, pages of 200 by id): production's orders paid before this step get their tickets after the deploy, idempotently, with no one-off command.
- **Tables** (migration 00005, additive): `tickets` (unit key, generation 1 to 65,535, holder kind and id, the holder before the last reissue, state valid / suspended / cancelled, name, picked day, check-in), `door_counters` + `door_feed` (per event: revoked, suspended, lifted, entry, conflict, manifest rows; the sequence comes from the event's counter row under its row lock, so rows commit in sequence order and a cursor never skips a late commit), `door_devices`, `ticket_scans` (one admitted scan per ticket by a partial unique index). RLS enabled and forced, composite foreign keys, the feed append-only. Six SECURITY DEFINER functions owned by `eps_sales_index` (explicit `search_path`, EXECUTE for `eps_core` only): `ticket_organiser`, `holder_organisers`, `paid_orders_unissued`, `issued_orders_without_tickets`, `person_organisers`, `recent_sales`.
- **Migrations against a live system** (lesson from step 6's production deadlock). 00005 sets `lock_timeout = '5s'` and takes its locks on existing tables first, in one statement in a fixed order (`sales.orders` for the foreign key and the index, `sales.saved_payment_methods` for the index role's policy); everything else is new objects. `cmd/migrate` retries the whole run up to 5 times with backoff (2 s doubling, jitter) on SQLSTATE 55P03 or 40P01 (`db.MigrateAllRetry`; goose rolls a failed migration back as a whole). Tests: `internal/db/retry_test.go` (classification, backoff, bounded tries) and `retry_db_test.go` (a live writer holds `sales.orders` past the timeout; the retry applies 00005). Control migration 00010 adds `control.public_brand_looks(uuid[])` (published brand name, slug and tokens; owner `eps_shop_read`) for the wallet's brand colours.
- **Keys (D3).** `TICKET_SIGNING_KEY` (a random 32-byte Ed25519 seed, hex) is added to `eps-runtime` by `runtime-secret.sh` (new keys only, existing ones never change) and mounted into the core only (`core.ticketKeys`; CI's chart rules check the gateway, worker and simulator do not get it). Key id 1 (`TICKET_KEY_ID`); a rotation sets a new id and lists the old public key in `TICKET_RETIRED_KEYS` (`id:hex:validTo`) until its last event ended. Outside `EPS_ENV=dev` the core stops without the key; readiness has a `ticket-key` check. `GET /v1/ticket-keys` (public) lists key id, raw public key (base64url), validity and the server time.
- **Codes (D4).** `GET /v1/me/tickets/{id}/codes` signs the current window and the next 39 for the holder's current generation; anyone else gets 404, the previous holder 410 `EPS-TICKET-MOVED`, a suspended, cancelled or unnamed (named product) ticket 409 with its code; 30 buffers per holder per minute (Redis). The page keeps the buffer in React state only, refetches below 5 minutes left (at most every 20 s while failing) and shows "Reconnect to show your code" when the buffer has run out.
- **Browser verification (D7).** `lib/tickets/token.ts` uses WebCrypto Ed25519 and falls back to `lib/tickets/ed25519.ts`, a ~150-line verification-only implementation (BigInt, SHA-512 from WebCrypto); both pass the Go vectors (`services/internal/tickets/testdata/vectors.json`). **No dependency was added**: the QR encoder is our own (`lib/tickets/qr.ts`, byte mode, level M, versions 1 to 10; matrices checked with ZXing when recorded, pinned by hash in `qr.test.ts`). **Deviation:** no JavaScript QR *decoder* fallback: the door uses the browser's `BarcodeDetector` (Chrome on Android, the door phone) for the camera and for "scan from image"; elsewhere staff type or paste the code. A decoder library is the step 12 option if iOS door phones are needed.
- **Door (D6 to D8).** `/door/<event>` (owner, admin, box office; others 404 from the core) registers a device at a gate, downloads the keys and the manifest in pages of 2,000 with the feed cursor read before the first page, keeps everything in IndexedDB and a service worker (`/door-sw.js`, scope `/door/`) keeps the page and its scripts. Scans are written locally first (corrected time, a UUID scan id) and uploaded in batches of 200; the server locks each ticket in id order, so concurrent uploads never deadlock, and decides the first entry by device time (an earlier late arrival takes the first place; the other is a conflict row both devices read in the feed). The page syncs every 30 s while online and turns amber after 10 minutes.
- **Trip (D9).** `GET /v1/me/trip` joins every ticket the caller holds (all organisers, `holder_organisers`) with its snapshot (product type and name, own time from the pool instance or picked day in the event's time zone, deadlines, the overdue amount) and lists paid orders still issuing. Grouping, ordering and the next action are pure TypeScript (`lib/trip/timeline.ts`). Platform pages use the language setting (cookie), not a URL prefix, so the wallet is `/wallet` (not `/[locale]/wallet`); likewise `/wallet/tickets/<id>`, `/account`, `/t/<token>`, `/door/<event>`.
- **Personalisation, check-in, day pick (D10).** The snapshot gained optional `personaliseBy`, `checkinBy` and `lateCheckin` (all-in like a price step; `api/schemas/sale-snapshot.v1.json`); snapshots published before this step have none, so a named product's name is editable until the event starts and no online check-in is asked until the shop is published again. The late check-in fee is paid **from the EPS Wallet** in the check-in transaction (`wallet.PayFeeTx`, a ledger entry to the organiser's sales account; a short wallet is topped up through the simulated providers): a new payment purpose in the payment state machine would have reopened step 4's hot tables. Guests sign in for a late check-in. A flexible day ticket draws on an event pool, so picking records the day (a reissue) and has no per-day stock. The sample NIBIRU passes now carry placeholder deadlines (names by 1 July 2027, check-in by 10 July, 20 RON late).
- **Guest link.** `/t/<token>`: an order link of purpose `tickets` (HMAC with `ORDER_LINK_KEY`, order-scoped) valid until two days after the event (at most 400 days); the order page links it once the order is issued. A claimed order's link answers 410 ("added to an account"). A printout shows the details and "Open the live code on your phone", never a code.
- **Suspension.** Step 4 defaults a plan once an overdue payment's 7-day grace ends (the order is cancelled). So a ticket is suspended **while** its plan has an overdue payment (from the failed charge), lifted when it is paid, and cancelled with the order when the plan defaults.
- **Sample tickets (task 7.5): deviation.** Every ticket comes from a confirmed payment (the first requirement), and sales data is the Go side's (P2): seeding tickets would mean paid orders nobody paid. The demo buys on the sample shops (seconds with the simulated provider), as the e2e suite does.
- **GDPR deletion.** `POST /v1/me/erase` (accounts) and the account page's "Delete my data": see the fan-account requirement. Orders, attempts and ledger lines keep every amount and state; nothing names the person afterwards. Redis waiting-room places and cap entries expire on their own (at most two days); idempotency answers expire in 24 hours.
- **Pay now** on a scheduled instalment brings its due date forward (step 4's simulated plan clock): the worker then charges the plan's saved card at the simulated provider; on an overdue one it is step 4's pay-overdue.

## Risks / Trade-offs

- [A screenshot shown within the acceptance window, at most 30 s] → The door's single-entry rule rejects the second person; the demo shows a screenshot taken a minute earlier.
- [Two offline doors admit the same ticket] → Recorded as a conflict at sync (D8); door devices of one gate sync every 30 s when online.
- [A revoked ticket at a door that has not synced] → Bounded by the 10-minute code buffer and the stale-sync warning.
- [WebCrypto Ed25519 missing on an older phone] → Library fallback, tested in unit tests against the same vectors as Go.
- [A leaked private key lets anyone mint codes] → Key only in the core API secret, never logged; rotation by key ID; the vision names an HSM or KMS.
- [Issuing lag during the on-sale peak] → The worker scales separately (S2); "Issuing your tickets" is an honest state, and the dashboard shows issue lag.

## Migration Plan

New tables, functions and endpoints only (sales 00005, control 00010), with the lock-up-front rule and the migrate retry above. `runtime-secret.sh` adds `TICKET_SIGNING_KEY` to an existing `eps-runtime` before the release rolls out; a core without it does not start outside dev. After the deploy the worker backfills tickets for orders step 4's stub marked issued. Rollback is a redeploy of the previous images; issued tickets stay in the sales schema (the old stub ignores them).

## Open Questions

- Which QR decoder fallback is lightest on low-end Android; decided during the build by a quick scan test, without changing the specs.
