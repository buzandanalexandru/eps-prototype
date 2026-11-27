# Step 10: Organiser growth

**Demo act:** Act 2 (the sales dashboard and the promoter leaderboard filling live from simulator sales, the second VIP shop on the same event), with Act 1 touches (payment account connect) and Vision-act breadth (marketplace, audience export, webhooks).
**Features:** G1 (Real), C6, G3, G5, B11, A4, A5 (Thin).
**Decisions:** P1, P2, P3, O3, U2, R2, S3 (see `docs/planning/architecture.md`).

## Why

After step 9 the platform sells well but gives the organiser almost nothing back: no view of revenue, no growth tools, no way to own or move their data. Tixr (Rewards, Promoter System, owned data, webhooks), Shotgun (CRM, ambassador cashback) and Eventbrite (real-time dashboards, tracking links, REST + webhooks) show that this is what organisers choose a platform for, and EPS today has none of it (`docs/research/feature-matrix.md`, S7). EPS also already sells NIBIRU through eMAG, so "one event, several shops" is a real need (`nibiru-eps-teardown.md` §7).

## What Changes

- **Sales dashboard (G1, Real):** orders, gross revenue, fees and net per product and per day for each event and brand, live while the on-sale runs, with simulated and sample sales labelled.
- **Influencer & Promoter Hub (C6, Thin):** promoters and influencers with their own discount codes and tracked links; ordinary promo codes and access codes for presales and partners; sales attributed per code and link through checkout and the event stream; a live leaderboard and a payouts list (commission owed, simulated).
- **Audience (G3, Thin):** a buyers table per organiser with consent flags and a CSV export that only includes consenting, non-deleted buyers.
- **Webhooks (G5, Thin):** organiser webhook endpoints subscribed to `order.paid`, `ticket.issued` and other stream events, signed deliveries with retries, a delivery log and a test button.
- **Sales channels (B11, Thin):** several shops on one event (presale, VIP, partner such as a marketplace channel), each with its own products and optional access code, all drawing on the same capacity pools.
- **Marketplace (A4, Thin):** one public page listing every organiser's published events that opted in, linking to their own shops.
- **Payouts (A5, Thin):** a payment account screen per organiser (connect your own merchant account or let the platform act as merchant, both simulated) and a simulated payouts list.

## Capabilities

### New Capabilities
- `organiser-dashboard`: live sales figures per event, brand, product and day, with fees and net.
- `promoter-hub`: promoters, influencer and promo/access codes, tracked links, attribution, leaderboard, promoter payouts list.
- `audience`: the organiser-owned buyer list, consent flags and export.
- `webhooks`: organiser endpoints, signed event delivery, retries and the delivery log.
- `sales-channels`: several shops per event with their own products and rules over shared pools.
- `marketplace`: the cross-organiser public listing of published events.
- `payouts`: the organiser's payment account choice and the simulated payout list.

### Modified Capabilities
None. Cross-step behaviour (attribution carried on orders, channel recorded on orders, codes at checkout) is written in this step's own capabilities, per `spec-map.md`.

## Impact

- **apps/web:** EPS Studio screens (Sales, Promoters, Audience, Webhooks, Shops, Payments), the public `/marketplace` page, storefront handling of `/s/<brand>/<shop>`, `?ref=` links and code entry.
- **services/core:** attribution and channel fields on holds and orders, code validation at hold time, sales summary endpoint.
- **services/worker:** stream consumers for the sales and promoter roll-ups and for webhook delivery.
- **api/openapi.yaml:** sales summary, code validation, channel and attribution fields.
- **Postgres:** control tables (promoters, codes, links, webhook endpoints, channels, payment accounts) and sales tables (order attribution, roll-ups, webhook deliveries), all with organiser ID and row-level security.
- No new dependencies.
