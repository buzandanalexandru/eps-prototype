# Feature scope

Agreed in the planning session on 27 Nov 2026 (OpenSpec explore mode), from two review rounds on the EPS Feature Review page.
This is a planning record. The OpenSpec specs and changes will be written from it after the architecture decisions.

## Decisions so far
1. **Demo structure:** three acts with the builder as the spine. Act 1: an organiser builds and publishes a branded shop. Act 2: a live on-sale rehearsal with capped simulated fans (at most 5,000 in production, 2,000 on previews), bots and a failing payment provider; 100k fans appear only as arithmetic from measured rates (owner decision, 28 Nov 2026). Act 3: squad buy and trip wallet. The last 30 seconds cover the architecture.
2. **Timing:** the 48-hour clock has not started; planning happens before it.
3. **Extras planned in:** AI copilot (H1), Kubernetes + Helm (I2), k6 load test (I3), face-value resale (F6).
4. **Merges:** the prepaid wallet and cashless balance become one **EPS Wallet** (D12); influencer codes and referral links become one **Influencer & Promoter Hub** (C6).
5. **OTP before payment is risk-based** (E12): the organiser switches it on, and only suspicious or high-quantity buyers are asked.
6. **Architecture split:** a Next.js (TypeScript) control plane (builder, catalogue, dashboards, fan UI, AI) and a Go data plane (queue, inventory, checkout, payments, issuing).
7. **Product principle:** open to every organiser through a white-label builder. NIBIRU is organiser #1, never hard-coded.
8. **Headline fan features:** squad buy (F1) and rotating QR (F3).

## Depth
- **Real:** works end to end and appears in the video.
- **Thin:** a real, clickable screen with sample data and simple or simulated logic.
- **Vision:** no screen; named on the roadmap page and in the vision.

Totals: **70 features: 46 Real, 23 Thin, 1 Vision.** No real payments: every payment path, Apple/Google Pay included, runs through simulated providers.

## A · Platform for every organiser

| ID | Feature | Depth | Act | What it is | Notes |
|---|---|---|---|---|---|
| A1 | Organiser sign-up and self-serve onboarding | Real | Act 1 | An organiser creates an account, names their organisation, picks a template and has a draft shop in minutes, without talking to sales. |  |
| A2 | Several brands per organiser | Real | Act 1 | One organiser account holds several brands, for example a resort and a festival, each with its own look and domain. |  |
| A3 | Team roles | Thin | Act 1 | Owner, admin, box office and finance roles, scoped to the organiser, a brand or an event. A simple screen in the prototype. | Thin means: A roles screen with sample members; permissions enforced only for owner vs others. |
| A4 | Shared EPS marketplace | Thin | Vision | An optional public site that lists every organiser's events, for discovery across the platform. | Thin means: One public page listing every organiser's published events. |
| A5 | Payment account per organiser | Thin | Act 1 | Each organiser connects their own merchant account, or lets the platform act as merchant. Simulated in the prototype. | Thin means: A connect screen; payouts are simulated. |
| A6 | Published fees and a fee-policy switch | Real | Act 1 | The platform fee is public, and the organiser chooses whether they absorb it, pass it to the fan, or split it. The fan always sees the all-in result. |  |

## B · Builder: brand and storefront

| ID | Feature | Depth | Act | What it is | Notes |
|---|---|---|---|---|---|
| B1 | Brand kit to design tokens | Real | Act 1 | Logo, colours, fonts and corner radius become design tokens used everywhere, with an automatic contrast check. |  |
| B2 | Starting templates | Real | Act 1 | Festival, resort, club night and theatre templates, each a ready set of pages and sections. |  |
| B3 | Pages from sections and blocks | Real | Act 1 | Pages built from blocks: hero, line-up, ticket grid, calendar, map, FAQ, gallery, countdown. Add, remove, reorder, configure. |  |
| B4 | Live preview on desktop and phone | Real | Act 1 | Every change shows immediately in a preview that switches between desktop and phone. |  |
| B5 | Draft, preview link, publish, rollback | Real | Act 1 | Changes stay in a draft until published. A private link shows the draft. Every published version is kept and can be restored. |  |
| B6 | Checkout stays locked | Real | Act 1 | The checkout takes the brand's tokens and a few fixed slots (custom questions, consents) but no free layout, so it stays fast and safe at peak load. |  |
| B7 | Subdomain per brand | Real | Act 1 | Each brand is published at its own address, for example sunwave.eps.events, straight away. |  |
| B10 | Language and currency per shop | Thin | Act 1 | Each shop chooses its languages (RO, EN, ...) and currency (RON, EUR). | Thin means: RO and EN text on the storefront; currency shown per shop. |
| B11 | Several shops per event | Thin | Act 2 | One event sold through several shops: presale, partners, VIP, a marketplace channel such as eMAG, each with its own products and rules. | Thin means: Second shop on the same event (for example a VIP shop) with its own products. |
| B12 | Email and ticket design | Thin | Act 1 | The confirmation email, ticket PDF and wallet pass use the brand kit. | Thin means: Preview of the email and ticket with the brand kit applied. |

## C · Catalogue: products, capacity, pricing

| ID | Feature | Depth | Act | What it is | Notes |
|---|---|---|---|---|---|
| C1 | Events, venues, line-ups | Real | Act 1 | Events with dates, times, venue, line-up and descriptions, reusable as a new edition next year. |  |
| C2 | Core product types | Real | Act 1 | Multi-day pass by zone, day ticket, and add-ons with their own capacity (camping, parking). |  |
| C3 | NIBIRU's special product types | Thin | Act 1 | Ticket that is a spend voucher, table by category, time slot (padel), and a flexible day ticket where the day is picked later. | Thin means: Product types exist in the model and NIBIRU's seed uses them; sold through the normal checkout. |
| C4 | Shared capacity pools | Real | Act 1 | One capacity can be shared by several products (GA and GA Plus draw from one Arena pool), and a product can sit in several pools. |  |
| C5 | Price ladder by date or by sales | Real | Act 1 | Prices step up on set dates or after a number of sales, with a public note that prices are not based on the buyer. |  |
| C6 | Influencer & Promoter Hub | Thin | Act 2 | Discount codes and tracked links per influencer or promoter, with live tracking of how much each one sold, a leaderboard and payouts. Also covers ordinary promo and access codes for presales and partners. | Merged; Thin means: Create codes and links, see live counts from simulator sales; payouts are a list. |
| C7 | Upgrades | Thin | Act 3 | Move from one product to a better one (GA to VIP) and pay the difference. | Kept in chat; Thin means: Upgrade button on a ticket that charges the difference through the simulated providers. |
| C8 | Buying rules | Thin | Act 1 | Age limits, a parental-consent requirement, maximum per order and per account. | Thin means: Per-account caps are Real inside E6; age and consent rules are a setting plus a checkout question. |
| C9 | Instalment plans per product | Thin | Act 1 | A deposit now and monthly payments, set per product. | Thin means: Instalment option configured per product; the schedule is generated, later payments simulated. |
| C10 | NIBIRU rebuilt as organiser #1 | Real | All acts | The demo data is NIBIRU's own 2027 catalogue, rebuilt in the builder with no code. Clearly labelled as sample data. |  |

## D · Checkout and payments

| ID | Feature | Depth | Act | What it is | Notes |
|---|---|---|---|---|---|
| D1 | All-in price everywhere | Real | Act 2 | The price on the event page is the price paid. No fee appears for the first time at checkout. |  |
| D2 | Cart hold with visible timer | Real | Act 2 | Tickets are held for a few minutes with a countdown, then released automatically. |  |
| D3 | Clear order status | Real | Act 2 | An order moves through visible states (held, paying, paid, ticket issued, or declined / expired), and the status page shows the truth. |  |
| D4 | Two simulated payment providers | Real | Act 2 | Fake providers A and B with adjustable failure rate and delay, so failures can be shown on purpose. No real payments. |  |
| D5 | Automatic failover | Real | Act 2 | When provider A starts failing, new payments move to provider B without the fan noticing. |  |
| D6 | No double charges | Real | Act 2 | Every payment attempt carries a key so a retry or a double click never charges twice. |  |
| D7 | Resume payment link | Real | Act 2 | An order that was held but not paid can be resumed from a link while the hold lasts. | Raised to Real in review |
| D8 | Guest checkout and account claim | Real | Act 2 | Buy without an account; claim the tickets into an account later. |  |
| D9 | Checkout question builder | Thin | Act 1 | The organiser adds questions and consents to the checkout (fixed slots, see B6). | Thin means: Add questions and consents into the fixed checkout slots. |
| D11 | Instalment checkout | Real | Act 2 | Choose an instalment plan at checkout and see the schedule. | Raised to Real in review |
| D12 | EPS Wallet | Real | Act 2 | Fans top up a balance in advance and pay instantly from it when a sale drops. The same balance pays at the stands on site, and anything unspent becomes next-season credit. Simulated in the prototype; in production it needs an e-money licence or a regulated partner. | New; Real means: Top up, pay from balance in one tap during the on-sale; a ledger, no real money. A strong Act 2 moment. |
| D13 | Apple Pay, Google Pay and saved cards | Real | Act 2 | One-tap payment with Apple Pay or Google Pay, and saved cards (stored by the payment provider, never by EPS) for a quick payment without retyping details. | New; Raised to Real in review |

## E · On-sale day: queue and bots

| ID | Feature | Depth | Act | What it is | Notes |
|---|---|---|---|---|---|
| E1 | Sale settings screen | Real | Act 2 | The organiser sets the opening time, queue on or off, caps per account, and admission rate. |  |
| E2 | Waiting room with fair shuffle | Real | Act 2 | Fans who arrive before the opening wait in a pre-queue; at opening they are shuffled at random, then latecomers join in order. |  |
| E3 | Signed queue passes | Real | Act 2 | Each admitted fan gets a short-lived signed pass tied to their session; checkout refuses anyone without one. |  |
| E4 | Admission rate dial | Real | Act 2 | The rate at which fans leave the queue for checkout can be changed live to match checkout capacity. |  |
| E5 | Silent bot scoring | Real | Act 2 | Suspicious traffic is scored and quietly sent to the back after the shuffle, so bots can't learn and adapt. |  |
| E6 | Caps per person | Real | Act 2 | Limits per account or verified identity across all sale moments. |  |
| E7 | On-sale simulator | Real | Act 2 | A control panel that sends a capped run of simulated fans (at most 5,000 in production, 2,000 on previews), a share of bots and a provider outage at a rehearsal copy of a sale; runs are refused while a real sale has fans waiting. The 100,000-fan figure is shown only as arithmetic from measured rates, with its assumptions. | Owner decision 28 Nov 2026 ("cap + honest maths"); step 12 measures the cluster with k6. |
| E8 | Live on-sale dashboard | Real | Act 2 | Queue length, fans admitted per minute, tickets sold, payment failures and failovers, bots caught, stock left. |  |
| E9 | Pre-registration and lottery sale | Thin | Act 2 | Fans register weeks before; tickets are allocated by draw instead of a race. | Thin means: A registration form and a draw button; winners get access codes. |
| E10 | Waitlist that fills itself | Thin | Act 2 | Fans pre-authorise a card; returned or resold tickets go automatically to the next person on the list. | Thin means: Join the waitlist; a returned ticket goes to the first person. |
| E11 | Graceful degradation | Thin | Act 2 | If load is extreme, event pages switch to static versions and the shop shows an honest message instead of crashing. | Thin means: A switch that serves static event pages and an honest busy message. |
| E12 | Risk-based OTP before payment | Real | Act 2 | An organiser can switch on an extra one-time code just before payment for high-demand events. It is only asked of buyers whom bot scoring marks as suspicious, or who buy above a set number of tickets, so real fans pass through. | New; Real means: A simulated code shown on screen; triggered by the bot score or the ticket count. |

## F · Fan after purchase

| ID | Feature | Depth | Act | What it is | Notes |
|---|---|---|---|---|---|
| F1 | Squad buy | Real | Act 3 | One friend starts a group order; the tickets are held for everyone; each friend pays their own share (or instalment) from a link. | Headline |
| F2 | Trip wallet | Real | Act 3 | One timeline per fan: tickets, add-ons, table, padel slot, bus, with what to do next (personalise, check in). |  |
| F3 | Rotating QR ticket | Real | Act 3 | The ticket's QR code changes every few seconds, so screenshots and resold copies don't work. | Headline |
| F4 | Apple and Google Wallet passes | Thin | Act 3 | The ticket can be added to the phone's wallet. | Thin means: Apple and Google passes need signing accounts; the prototype shows the button and a pass preview. |
| F5 | Personalise and check in online | Thin | Act 3 | Name each ticket and check in before the deadline, as NIBIRU requires today. | Thin means: Name each ticket and press check in before the deadline. |
| F6 | Transfer and resale at face value | Real | Act 3 | Sell back or pass on a ticket at no more than face value; the old barcode is cancelled and a new one issued. | Decided earlier |
| F8 | Fan account | Thin | Act 3 | Orders, tickets and instalment schedules in one place. | Thin means: Orders, tickets and instalments listed. |

## G · Organiser operations and growth

| ID | Feature | Depth | Act | What it is | Notes |
|---|---|---|---|---|---|
| G1 | Sales dashboard | Real | Act 2 | Orders, revenue, fees and sales per product, per day. |  |
| G2 | Door scanning page | Real | Act 3 | A phone page that scans tickets at the gate and keeps working offline. | Raised to Real in review |
| G3 | Organiser-owned audience | Thin | Vision | The organiser owns and exports its buyer list and consents. | Thin means: A buyers table with consent flags and a CSV export. |
| G5 | Webhooks and public API | Thin | Vision | Events such as order paid or ticket issued sent to the organiser's systems; a documented API. | Thin means: A webhook list and a delivery log. |
| G6 | Refunds and cancellation | Vision | Vision | Refund orders or cancel an event and refund everyone in one step. | Refunds need the payment flow first; named on the roadmap. |

## H · AI

| ID | Feature | Depth | Act | What it is | Notes |
|---|---|---|---|---|---|
| H1 | Organiser AI copilot | Real | Act 1 | The organiser describes the event in a sentence; AI drafts the event, ticket tiers, prices and a themed shop, which they then edit in the builder. | Decided earlier |
| H2 | Fan AI concierge | Thin | Act 3 | A fan asks for a plan ("weekend for 4, hip-hop, 300 RON each") and gets an itinerary and a filled cart. Never buys on its own. | Thin means: A chat panel that turns a request into a suggested cart; the fan confirms. |
| H3 | Sell through AI assistants (MCP) | Thin | Vision | An MCP server so ChatGPT, Gemini or Claude can find and sell EPS events. | Thin means: A small MCP server exposing event search and cart links. |

## I · Platform, scale and demo hosting

| ID | Feature | Depth | Act | What it is | Notes |
|---|---|---|---|---|---|
| I1 | Organisers kept apart | Real | All acts | Every record belongs to one organiser, and nobody can see another organiser's data. |  |
| I2 | Kubernetes manifests and Helm | Thin | Vision | The backend packaged for Kubernetes, with autoscaling for the on-sale services. | Decided earlier; Thin means: Manifests and a Helm chart in the repo; the demo itself runs on managed hosting. |
| I3 | Measured load test (k6) | Real | Act 2 | A real load test with published numbers, shown in the video. | Decided earlier |
| I4 | Basic monitoring | Real | Act 2 | Metrics for queue, checkout and payments, feeding the live dashboard. | Kept in chat |
| I5 | Public demo deployment | Real | All acts | A public URL the interviewers can click through (front end on Vercel; backend host to decide). |  |

## Not in the product

| ID | Feature | Reason |
|---|---|---|
| B8 | Custom domain screen | Cut in review |
| B9 | Embed widget | Cut in review |
| D10 | Digital parental consent and accessibility requests | Cut in review |
| F9 | Taste-based discovery and friends going | Cut in review |
| G7 | Audit log | Cut in review |
| H4 | AI pricing and demand forecasting | Cut in review |
| J1 | Real payments | Out of scope |
| J2 | Seat maps | Out of scope |
| J3 | Cashless and RFID hardware | Out of scope |
| J4 | Native mobile apps | Out of scope |
