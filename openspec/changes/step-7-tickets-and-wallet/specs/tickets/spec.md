# Spec Delta

## Purpose

Issues tickets only from confirmed payments and gives each ticket a rotating, signed entry code that screenshots and old copies cannot reuse, plus the personalisation, check-in, day-picking, guest-link and wallet-pass actions a fan takes before the event.

## ADDED Requirements

### Requirement: Tickets are issued only from confirmed payments
The system SHALL issue tickets only when it consumes an `order.paid` event from the event stream, one ticket per paid unit, recorded under the order's organiser, event and snapshot version, and SHALL emit a `ticket.issued` event for each ticket. A payment confirmed on a status page, a redirect or a client request alone MUST NOT issue a ticket.

#### Scenario: Paid order gets its tickets
- **WHEN** the worker consumes `order.paid` for an order with 2 GA passes and 1 parking add-on
- **THEN** 3 tickets exist for that order, each with its product, the order's snapshot version, generation 1 and status valid, and 3 `ticket.issued` events are in the outbox
Test: services/internal/tickets/issue_test.go

#### Scenario: Declined or expired order gets no ticket
- **WHEN** an order ends declined or expired and no `order.paid` event exists for it
- **THEN** no ticket exists for the order and the order's status page shows no ticket
Test: services/internal/tickets/issue_test.go

#### Scenario: Status page shows issuing honestly
- **WHEN** an order is paid but its tickets have not been issued yet
- **THEN** the status page and the trip wallet show "Issuing your tickets" and refresh on their own until the tickets appear
Test: apps/web/tests/e2e/tickets.spec.ts

### Requirement: Issuing is idempotent
Issuing SHALL be idempotent per order unit: a redelivered or duplicated `order.paid` event MUST NOT create extra tickets or extra `ticket.issued` events.

#### Scenario: Event delivered twice
- **WHEN** the same `order.paid` event is delivered twice, including concurrently to two worker replicas
- **THEN** the order still has exactly one ticket per paid unit and one `ticket.issued` event per ticket
Test: services/internal/tickets/issue_test.go

### Requirement: Tickets carry a generation that reissuing increments
Each ticket SHALL have a generation number starting at 1. Reissuing a ticket (claim, transfer, resale, upgrade, day pick or cancellation by the organiser) MUST increment the generation in the same transaction as the change of holder or product, and MUST record a revocation of all earlier generations for that event.

#### Scenario: Reissue revokes the old generation
- **WHEN** ticket T at generation 1 is reissued to a new holder
- **THEN** T is at generation 2 with the new holder, and the event's revocation feed has an entry saying generations below 2 of T are invalid
Test: services/internal/tickets/reissue_test.go

#### Scenario: Concurrent reissues
- **WHEN** two reissues of the same ticket run at the same time
- **THEN** one succeeds and the other fails with a conflict, and the generation increases by exactly one
Test: services/internal/tickets/reissue_test.go

### Requirement: Entry code rotates every 15 seconds and is signed
The entry code SHALL encode the format version, key ID, event ID, ticket ID, generation and the 15-second time window, signed with Ed25519 by the platform's ticket key. A new code MUST be shown for every window. The code MUST NOT contain personal data.

#### Scenario: Code changes each window
- **WHEN** the holder keeps the live ticket open for 45 seconds
- **THEN** at least three different codes are shown, each for its own window, with a visible countdown to the next change
Test: apps/web/tests/e2e/tickets.spec.ts

#### Scenario: Code verifies with the published public key
- **WHEN** a code is decoded and checked with the public key for its key ID
- **THEN** the signature is valid and the decoded event, ticket, generation and window match the ticket and the current window
Test: services/internal/tickets/token_test.go

#### Scenario: Altered code fails
- **WHEN** any byte of the token, such as the generation or the window, is changed
- **THEN** the signature check fails
Test: services/internal/tickets/token_test.go

#### Scenario: Same test vectors in Go and the browser
- **WHEN** the web verifier checks the fixed test vectors produced by the Go signer
- **THEN** it accepts the valid vectors and rejects the altered ones exactly as Go does
Test: apps/web/tests/unit/ticket-token.test.ts

### Requirement: Only the current holder can get codes
The system SHALL return entry codes only to the ticket's current holder, for its current generation, as a buffer of at most 10 minutes of upcoming windows. The fan page MUST keep codes in memory only and MUST NOT show a code once its buffer has run out. Revoked, suspended or used-up generations MUST get no codes.

#### Scenario: Another user asks for codes
- **WHEN** a signed-in user who is not the holder requests codes for a ticket
- **THEN** the request fails with not found, without saying whether the ticket exists
Test: services/internal/tickets/codes_test.go

#### Scenario: Buffer is at most 10 minutes
- **WHEN** the holder requests codes
- **THEN** the response holds codes for the current window and no more than 39 following windows
Test: services/internal/tickets/codes_test.go

#### Scenario: Old holder after a reissue
- **WHEN** the previous holder of a reissued ticket opens it
- **THEN** no code is returned and the page says the ticket has moved
Test: services/internal/tickets/codes_test.go

#### Scenario: Offline after the buffer runs out
- **WHEN** the phone is offline and the last buffered window has passed
- **THEN** the page shows "Reconnect to show your code" and no QR
Test: apps/web/tests/e2e/tickets.spec.ts

### Requirement: Ticket signing keys are kept on the server and published
The private ticket key SHALL exist only in the core API's secret, generated by CI per environment. The public keys SHALL be published with their key IDs and validity, and an old public key MUST stay published until every event signed with it has ended.

#### Scenario: Public key set is published
- **WHEN** anyone requests the ticket keys
- **THEN** the response lists each key ID with its Ed25519 public key and validity period, and no private material
Test: services/internal/tickets/keys_test.go

#### Scenario: Private key never reaches the web app
- **WHEN** the web app's build output and environment are searched for the ticket private key
- **THEN** it is not found
Test: apps/web/tests/unit/no-ticket-secret.test.ts

#### Scenario: Missing key stops the service
- **WHEN** the core API starts without a ticket signing key
- **THEN** its health check fails and it issues no codes
Test: services/internal/tickets/keys_test.go

### Requirement: Revocation feed for door devices
The system SHALL publish, per event, an ordered feed of revocations and suspensions since a cursor, readable only by that event's organiser staff, so door devices can learn about reissued and suspended tickets.

#### Scenario: Door device reads revocations since its cursor
- **WHEN** a door device of the event's organiser requests the feed since its last cursor
- **THEN** it receives only the newer revocations in order with a new cursor
Test: services/internal/tickets/revocations_test.go

#### Scenario: Other organiser's staff are refused
- **WHEN** staff of another organiser request the feed for this event
- **THEN** the request fails with not found
Test: services/internal/tickets/revocations_test.go

### Requirement: Overdue instalments suspend the code
A ticket whose instalment plan is overdue past its grace period SHALL be suspended: it shows "Payment overdue" with a pay-now action and no code, and its suspension MUST appear in the revocation feed. Paying the overdue amount MUST lift the suspension.

#### Scenario: Plan goes overdue
- **WHEN** the ticket's instalment plan passes its grace period unpaid
- **THEN** the holder gets no code, the ticket shows "Payment overdue", and the feed carries a suspension
Test: services/internal/tickets/suspend_test.go

#### Scenario: Overdue amount paid
- **WHEN** the holder pays the overdue instalment and `order.paid` for it is consumed
- **THEN** the suspension is lifted in the feed and codes are returned again
Test: services/internal/tickets/suspend_test.go

### Requirement: Personalise each ticket before its deadline
For products that require it, the holder SHALL name each ticket (and fill the fields the product asks for) until the product's personalisation deadline; after the deadline the name MUST be locked. A ticket that requires personalisation MUST show "Name this ticket" instead of a code until it is named.

#### Scenario: Name a ticket before the deadline
- **WHEN** the holder enters a name for a ticket before the deadline
- **THEN** the ticket shows the name and its live code
Test: manual check on the deployed preview (name a NIBIRU pass and see the code appear)

#### Scenario: Deadline has passed
- **WHEN** the holder tries to change the name after the deadline
- **THEN** the change is refused and the page says the name is locked
Test: services/internal/tickets/personalise_test.go

### Requirement: Online check-in with a paid late check-in
For products with online check-in, the holder SHALL check in free until the check-in deadline. After the deadline, check-in MUST charge the product's late check-in fee through the simulated payment flow and complete only when that payment is confirmed.

#### Scenario: Check in on time
- **WHEN** the holder presses Check in before the deadline
- **THEN** the ticket shows checked in at no charge
Test: manual check on the deployed preview (check in a ticket before its deadline)

#### Scenario: Late check-in
- **WHEN** the holder checks in after the deadline
- **THEN** the late fee is shown all-in, charged through the simulated providers or the EPS Wallet, and the ticket shows checked in after the payment is confirmed
Test: services/internal/tickets/checkin_test.go

### Requirement: Pick the day later on a flexible ticket
A flexible day ticket SHALL let the holder pick one day from the event's days that still have stock; picking MUST reissue the ticket for that day and is final.

#### Scenario: Pick a day
- **WHEN** the holder picks Saturday on a flexible day ticket
- **THEN** the ticket becomes a Saturday ticket at a new generation and Saturday's stock decreases by one
Test: manual check on the deployed preview (pick a day on a NIBIRU flexible day ticket)

### Requirement: Signed link for guest buyers
A guest buyer SHALL be able to open their order's tickets from a signed link without an account. The link MUST be scoped to that order and MUST fail if altered. Printable tickets MUST NOT carry a static entry code.

#### Scenario: Guest opens the signed link
- **WHEN** a guest opens the link from their confirmation email
- **THEN** they see the order's tickets with live codes and an offer to claim them into an account
Test: apps/web/tests/e2e/tickets.spec.ts

#### Scenario: Altered link
- **WHEN** the order number or signature in the link is changed
- **THEN** the page shows "Link not valid" and no ticket data
Test: apps/web/tests/unit/signed-link.test.ts

### Requirement: Wallet pass preview
The live ticket SHALL show "Add to Apple Wallet" and "Add to Google Wallet" buttons that open a preview of the pass in the brand's design, with a note that real passes need signing accounts. No pass file is produced.

#### Scenario: Open the pass preview
- **WHEN** the holder presses Add to Google Wallet
- **THEN** a branded pass preview with the event, product and holder name appears, with the signing-accounts note in the fan's language
Test: manual check on the deployed preview (open both pass previews in RO and EN)
