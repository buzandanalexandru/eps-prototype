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

## Risks / Trade-offs

- [A screenshot shown within the acceptance window, at most 30 s] → The door's single-entry rule rejects the second person; the demo shows a screenshot taken a minute earlier.
- [Two offline doors admit the same ticket] → Recorded as a conflict at sync (D8); door devices of one gate sync every 30 s when online.
- [A revoked ticket at a door that has not synced] → Bounded by the 10-minute code buffer and the stale-sync warning.
- [WebCrypto Ed25519 missing on an older phone] → Library fallback, tested in unit tests against the same vectors as Go.
- [A leaked private key lets anyone mint codes] → Key only in the core API secret, never logged; rotation by key ID; the vision names an HSM or KMS.
- [Issuing lag during the on-sale peak] → The worker scales separately (S2); "Issuing your tickets" is an honest state, and the dashboard shows issue lag.

## Migration Plan

New tables and endpoints only. CI adds key generation before the first deploy of this step; a preview without a key fails its health check rather than issuing unsigned codes. Rollback is a redeploy of the previous images; issued tickets stay in the sales schema.

## Open Questions

- Which QR decoder fallback is lightest on low-end Android; decided during the build by a quick scan test, without changing the specs.
