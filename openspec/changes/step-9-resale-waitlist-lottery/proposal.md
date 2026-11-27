# Proposal

Step 9: Face-value resale, self-filling waitlist, lottery and upgrades

- **Demo act:** Act 3 (a fan resells a ticket at face value: the old QR dies at the door, the buyer gets a new one), with the lottery and waitlist shown briefly as Act 2 alternatives to the click race.
- **Features:** F6 transfer and resale at face value (Real), E10 waitlist that fills itself (Thin), E9 pre-registration and lottery sale (Thin), C7 upgrades (Thin).
- **Decisions:** O5 and the step 7 reissue primitive (new code, old one revoked), O3 (order and payment states), O6 (EPS Wallet ledger for seller payouts), P1 (sale snapshots), P3 (event stream), E5/E6 from step 5 (bot scores and caps per person).

## Why

NIBIRU's tickets are named and cannot be transferred or resold (teardown section 5), so fans who cannot go lose their money or turn to touts with copies of the same barcode. TicketSwap, Ticketmaster and Tomorrowland show the fix: resale inside the platform at no more than face value, with the seller's barcode cancelled and a new one issued. On-sale day can also be fairer than a race: a lottery after pre-registration (Tomorrowland, Ticketmaster Ticket Request), and a waitlist with a pre-authorised card that takes returned tickets automatically (Shotgun, TicketSwap Auto buy). Upgrades (GA to VIP) are revenue NIBIRU's API already hints at.

## What Changes

- Organisers set a resale policy per event (on or off, opening date, published resale fee). Fans list a ticket at no more than its face value, or transfer it free to a friend; on a sale or transfer the old generation is revoked and the new holder gets a new code and personalises it again. The seller is paid out to their EPS Wallet (simulated).
- A waitlist per sold-out product: the fan pre-authorises a card (simulated); a returned ticket or a resale listing goes automatically to the first person, and on failure to the next.
- Lottery sale moments: registration in a window, one entry per person, a verifiable random draw, single-use access codes for winners.
- Upgrades: an Upgrade button on a ticket that charges the difference plus the organiser's upgrade fee and reissues the ticket as the better product.

## Capabilities

### New Capabilities
- `resale`: resale policy, face-value cap, listing, buying a resale ticket, transfer to a friend, reissue and re-personalisation, seller payout.
- `waitlist`: joining with a simulated pre-authorisation, automatic allocation of returned and resold tickets, leaving.
- `lottery-sales`: pre-registration, the draw and winners' access codes.
- `upgrades`: upgrade paths, paying the difference and reissuing the ticket.

### Modified Capabilities
- None.

## Impact

- **Go (services/):** `resale`, `waitlist`, `lottery` and `upgrade` packages in the core API; allocation and draw jobs in the worker; sales-schema tables (resale policies, listings, transfers, waitlist entries, lottery moments, registrations, access codes) under row-level security; outbox events `resale.listed`, `resale.sold`, `ticket.transferred`, `waitlist.allocated`, `lottery.drawn`, `ticket.upgraded`.
- **Fake providers (cmd/fakepsp):** an authorise / capture / void path for waitlist pre-authorisations (simulated, no card data).
- **Next.js (apps/web):** resale and transfer actions on the live ticket, a resale section on the storefront product, waitlist join and leave, lottery registration and the organiser's draw screen, the upgrade dialog; organiser settings for resale, lottery and upgrade paths in EPS Studio; RO and EN.
- **Contract:** new paths in `api/openapi.yaml`.
