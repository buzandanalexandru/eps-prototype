# Design

## Context

See proposal.md for the why. Interfaces assumed from earlier steps:

- **Holds (steps 4 and 5, O2):** the core API can take a hold of N units of a product from its capacity pools atomically, with a TTL, extend a hold's TTL, move units from one hold to another without touching stock, and release units back to stock. Expiry is handled by the hold itself.
- **Orders and payments (step 4, O3):** an order for a hold moves held, paying, paid, declined or expired; payment attempts carry idempotency keys and go through fake providers A and B with failover, Apple/Google Pay, saved cards, the EPS Wallet, or an instalment plan whose first instalment is the first charge. `order.paid` is written to the outbox when payment is confirmed; a refund of a confirmed payment is available (simulated).
- **Queue and caps (step 5, O1, E6):** checkout requires a valid queue pass when the sale uses the waiting room; caps per person are checked by the bot-defence capability at hold and at payment.
- **Tickets (step 7):** issuing from `order.paid`, personalisation, and the reissue primitive (generation bump plus revocation).
- **Sale settings (step 5):** the squad switch and squad window are two more fields on the sale settings screen and in the sale snapshot. If step 5 does not add them, this step adds a "Squad buy" section to that screen.

## Goals / Non-Goals

**Goals:**
- Each friend pays their own share; nobody pays for seats they did not choose unless the leader covers them on purpose.
- No oversell and no stuck stock: every seat ends paid, covered or released.
- Reuse the existing hold, order and payment machinery instead of a parallel path.

**Non-Goals:**
- Squads mixing several products (one product per squad in the prototype; a squad add-on such as camping is a later extension).
- Splitting one seat's price between people, or paying for a friend's seat without being the leader.
- Messaging friends from EPS: the leader shares links with their own apps (share sheet, copy link).

## Decisions

### D1 · Squad = group hold + one child order per seat (O7)
Creating a squad takes one group hold of N units (N between 2 and the sale's maximum per order) under the leader's queue pass, and creates `squads` and `squad_seats` rows (seat 1 is the leader's). Each seat, when someone starts paying for it, gets its own child order: one unit is moved from the group hold into a one-unit hold owned by that order, so stock never changes during a squad. Every child order records the squad ID and the snapshot version and unit price of the group hold. Issuing, the status page and the trip wallet need no squad-specific code.
*Alternative:* one order with N payment attempts from different payers. Rejected: the order state machine assumes one payer, and refunds and receipts become per-attempt special cases.

### D2 · Activation and squad window
A new squad is `forming` with the normal checkout hold TTL. It becomes `active` only when the leader's own share is paid; then the group hold's TTL is extended to the squad window. The window is set per sale: default 30 minutes when the sale uses the waiting room, up to 48 hours otherwise. The leader paying first stops someone from locking stock for free and proves the squad is real.
*Alternative:* activate without the leader paying. Rejected: free stock-locking at the on-sale peak is an abuse path.

### D3 · Seat links
Each seat has a 128-bit random token, stored only as its SHA-256 hash; the link is `/s/<brand>/squad/join/<token>`. The leader can revoke a seat's link and get a new one while that seat is unpaid. A link opens for anyone (guest or signed in) and does not need a queue pass: the stock is already held. Joining binds the seat to that person for the seat's payment lock (D4); a second person opening a bound seat sees "Someone is paying for this spot".

### D4 · Paying a share, concurrency
Starting payment moves the seat `open → paying` with a compare-and-set on the seat row and a payment lock of 10 minutes, capped at the squad deadline; a second payer on the same seat is refused before any charge. The payment attempt's idempotency key is `squad:<seat id>:<attempt no>`. `order.paid` for the child order moves the seat to `paid` and writes `squad.seat_paid`; a declined payment moves it back to `open`. Caps per person are checked for the payer at join and at payment; while unpaid, seats count against the leader's cap, so a squad never exceeds the leader's cap at creation.

### D5 · Deadline, release and late payments
The worker schedules every active squad's deadline (a Redis sorted set by deadline, with a Postgres sweep as the backstop). At the deadline it closes the squad in one transaction: `open` seats become `released` and their units go back to stock; `paying` seats get a grace of up to 2 minutes for their attempt to resolve, then are released if not paid. A payment confirmation that arrives for a released seat is refunded automatically (simulated) and writes no ticket; the friend's page says the spot expired and the money was returned. The squad ends `complete` (all seats paid or covered) or `closed` (some released).

### D6 · Leader covers unpaid seats
Before the deadline the leader can cover all or some unpaid seats in one order (one payment, normal flow). Covered seats issue tickets held by the leader; each keeps its link as a claim link, and a friend who opens it later can claim the ticket for free, which reissues it to them (step 7 primitive) so they personalise it themselves. Claim links stop working once the claimed ticket is reissued or the event's personalisation deadline passes.

### D7 · Price lock
All seats pay the unit price of the group hold's snapshot, all-in, even if the price ladder steps up during the squad window. The share shown is that price; instalment plans use the product's plan from the same snapshot.

### D8 · Privacy
The leader sees each seat's state and the display name the friend chose when joining, never their email, payment method or personalisation details. Friends see the leader's display name and the count of paid seats only.

### D9 · Live status
The squad page polls `GET /v1/squads/{id}` every 3 seconds (cheap, cached per squad in Redis); server-sent events are not needed at this scale.

## Risks / Trade-offs

- [Squads hold stock during a hot on-sale] → Short default window with the queue on, leader must pay first, organiser can switch squads off per sale; held squad seats show on the live dashboard as held stock.
- [A friend pays at the last second] → 2-minute grace for in-flight attempts; anything later is refunded automatically.
- [Lost worker during expiry] → The Postgres sweep releases any squad past its deadline; holds also expire on their own (O2), so stock cannot stay stuck.
- [Link forwarded to the wrong person] → One seat per link, revocable while unpaid; the payer chooses the ticket's name at personalisation.
- [Caps gamed by making squads] → Unpaid seats count against the leader's cap; each payer's cap is checked at payment.

## Migration Plan

New tables and endpoints only; the squad switch defaults to on for new sales and off for sales already published. Rollback is a redeploy; open squads expire and release on their own.
