# Design

## Context

See proposal.md for the why. Interfaces assumed from earlier steps:

- **Tickets (step 7):** tickets with generations, the reissue primitive (generation bump plus a revocation row in one transaction, conflict on concurrent reissue), personalisation, the door revocation feed and the trip wallet.
- **Checkout and payments (step 4):** holds with TTL, order and payment state machines, idempotent attempts through fake providers A and B, refunds (simulated), `order.paid` on the event stream, and the EPS Wallet double-entry ledger (O6) with a way to post a credit to a fan's balance.
- **On-sale engine (step 5):** caps per person, bot scores per account or session, access codes that unlock a product in checkout (the same mechanism presale codes use).
- **Sale snapshots (step 3, P1):** this step adds the resale policy, lottery moments and upgrade paths to what publish compiles into the snapshot. If the snapshot compiler is closed to new fields, they are read from a small `sale_extras` record versioned with the snapshot.

## Goals / Non-Goals

**Goals:**
- Resale where the old code provably stops working and the price can never exceed face value.
- One reissue path for transfer, resale, waitlist allocation and upgrades.
- Lottery and waitlist that are simple but honest: verifiable draw, real (simulated) pre-authorisation.

**Non-Goals:**
- Profit sharing on resale (TicketSwap FairShare), resale above face value, auctions.
- Real card pre-authorisations or real payouts: both are simulated ledger and fake-provider operations.
- Ticket insurance, refunds by the organiser (G6 is on the roadmap), downgrades.

## Decisions

### D1 · Face value
Face value is the all-in amount the ticket's holders have paid for it: the original price paid (after any discount) plus any upgrade difference, excluding late check-in fees and upgrade fees. It is stored on the ticket and carried across reissues. A listing's price must be between 1 and the face value; the API enforces it, the UI only mirrors it.
*Alternative:* the product's current list price. Rejected: an early-bird ticket could then be resold at a profit once the price ladder steps up.

### D2 · Resale policy per event
Per event, in EPS Studio: resale on or off, opens at, closes at (default 2 hours before doors, so no listing or transfer can race an offline door), and the published resale fee (a fixed amount per ticket, shown to the seller before listing). Tickets on an unfinished instalment plan, already admitted at the door, suspended, or with a pending transfer cannot be listed.

### D3 · Listing and selling
Listing a ticket creates a `listed` listing; the seller's code keeps working until it is sold, because the seller may still go. A buyer selects a resale ticket on the storefront (shown with its "Face value" label); the listing goes `held` for the normal checkout TTL and the buyer pays the listing price all-in through the step 4 flow. On `order.paid` for a resale order, one transaction: reissue the ticket to the buyer (new generation, personalisation cleared so the buyer names it again), mark the listing `sold`, post the ledger entries (buyer payment clearing to seller wallet for price minus fee, fee to the fee account), write `resale.sold`. A delist racing a hold: whoever changes the listing row first wins; the buyer's hold or the delist is refused.
*Alternative:* move the ticket at hold time. Rejected: the seller would lose a ticket for a payment that may fail.

### D4 · Transfer to a friend
The holder enters a friend's email or creates a transfer link (128-bit token, stored hashed). The transfer is `pending` until the friend accepts (signed in, or as a guest who then claims); the sender can cancel until then. On accept the ticket is reissued to the friend. Transfers follow the same closing time and blocks as resale. Free.

### D5 · Revocation at the door
Every reissue writes a revocation that door devices sync (step 7, D6). The old holder's page shows "Sold" or "Transferred" and gets no codes. A screenshot of an old code fails on its time window within 30 seconds, and the 10-minute code buffer bounds what an unsynced door could still accept from the old holder; because resale and transfers close 2 hours before doors, every door device that synced at preparation already knows.

### D6 · Waitlist with a simulated pre-authorisation
Joining a waitlist for a sold-out product authorises the product's face value on a fake provider (authorise / capture / void, no card data, Apple/Google Pay or saved card token from step 4). Entries are ordered by join time; bot-flagged entries are moved behind the others, as in the waiting room (E5). Allocation runs in the worker when a ticket returns to that product: a new resale listing, or a ticket the organiser returns to sale. The worker holds the ticket for the first entry, captures its pre-authorisation for the listing price (never more than authorised), and on `order.paid` reissues the ticket (resale order) or issues it (returned stock). A failed capture voids the entry with a notice and moves to the next. Leaving the list voids the pre-authorisation. Pre-authorisations are re-authorised every 7 days (simulated) to mirror real expiry.

### D7 · Lottery draw fairness
A lottery moment has a product, registration window, tickets to allocate and a maximum per registrant. When registration opens, the server generates a 32-byte random seed and publishes `SHA-256(seed)` on the lottery page. After registration closes, the organiser presses Draw once: each registration's rank is `HMAC-SHA256(seed, registration_id)`, sorted ascending, bot-flagged registrations after all others; winners are taken in order until the ticket count is used. The seed and the ordered list of registration IDs are then published, so anyone can recompute the draw. The draw is idempotent: a second press returns the stored result.
*Alternative:* `ORDER BY random()` in Postgres. Rejected: not reproducible and not verifiable.

### D8 · Winners' access codes
Each winner gets a single-use access code bound to their account for their quantity, valid for a purchase window (default 48 hours), redeemed through the step 5 access-code mechanism in normal checkout. Unused codes can be passed to the next ranked registrants with a "Draw next" action that continues down the same published order.

### D9 · Upgrades
The organiser defines upgrade paths between products of an event (GA to VIP) with an upgrade fee. The fan sees Upgrade when the target has stock; the price is `max(0, target current all-in price − face value) + fee`, shown all-in. Starting an upgrade holds one unit of the target; on `order.paid` one transaction reissues the ticket as the target product (new generation, name kept), adds the difference to the face value, and releases one unit of the original product back to its pools. Not offered on tickets on an unfinished plan, listed, suspended or already admitted.

## Risks / Trade-offs

- [Seller enters the venue, then the ticket sells] → Resale and transfer close 2 hours before doors; listings close automatically when an entry for the ticket syncs.
- [Waitlist allocation races public checkout for returned stock] → Returned tickets are held for the waitlist head before they are visible to anyone else.
- [Ledger drift on payouts] → Payout entries are written in the same transaction as the reissue; a reconciliation test sums every resale's buyer payment against seller credit plus fee.
- [The platform could pick a favourable seed] → The seed's hash is published before registration closes, so the seed cannot be chosen after seeing the entries; the vision names a public randomness beacon.
- [Thin waitlist and lottery under real load] → They are Thin by scope; their screens and logic are real, their scale story is in the vision.

## Migration Plan

New tables and endpoints only; resale is off by default for existing events. Rollback is a redeploy; pending listings and transfers stay as data and simply cannot be acted on.
