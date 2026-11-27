# Proposal

Step 8: Squad buy

- **Demo act:** Act 3 (headline: four friends, one group hold, each pays their own share from a link on their own phone, then each names their own ticket).
- **Features:** F1 squad buy (Real, headline).
- **Decisions:** O7 (squad buy as a group hold), built on O2 (timed holds), O3 (order and payment states), P1 (sale snapshots), P3 (event stream), U1 (guests and JWTs), O5 and the step 7 reissue primitive.

## Why

Groups go to festivals together, but every platform makes one person pay for everyone and chase friends for money afterwards (Ticketmaster split pay comes only after the fact; NIBIRU has none). During a hot on-sale, the friend who gets through the queue also has minutes to decide for the whole group. Squad buy lets the friend who gets through hold tickets for everyone, while each friend pays their own share, or the first instalment, from their own phone. It is the feature nobody else has, and it closes the three-act demo.

## What Changes

- A squad leader takes one group hold for N tickets of a product (within the sale's caps) and pays their own share to start the squad; the hold then lasts for the squad window set by the organiser.
- Each seat gets its own unguessable link. A friend opens it, with or without an account, sees the all-in share price (locked at hold time), the deadline and the squad's progress, and pays their share or first instalment through the normal payment flow.
- Each paid seat is its own order linked to the squad, so issuing, the status page and the trip wallet work as for any order; the friend holds and personalises their own ticket.
- At the deadline, unpaid seats go back to stock, unless the leader covers them first; covered seats can still be claimed by the friend later, which reissues the ticket to them.
- A live squad page for the leader and friends; a late payment for an expired seat is refunded automatically (simulated).

## Capabilities

### New Capabilities
- `squad-buy`: group holds, seat links, paying a share, squad deadline and release, leader cover and claim, squad status, and the rules squad buy adds on top of holds, caps and payments.

### Modified Capabilities
- None. (Squad buy's rules on holds and caps are written in `squad-buy`, per spec-map.md.)

## Impact

- **Go (services/):** a `squad` package in the core API (create, seats, join, pay share, cover, claim), squad expiry in the worker, new sales-schema tables (squads, squad_seats) under row-level security, outbox events `squad.created`, `squad.seat_paid`, `squad.completed`, `squad.expired`.
- **Next.js (apps/web):** "Buy as a squad" on the storefront product and in the checkout, `/s/[brand]/squad/[id]` (leader view), `/s/[brand]/squad/join/[token]` (friend view), RO and EN.
- **Contract:** squad paths in `api/openapi.yaml`.
- **Settings:** a squad toggle and squad window per sale in sale settings, read from the sale snapshot.
