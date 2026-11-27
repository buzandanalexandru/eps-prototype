# Ticketmaster / Live Nation: competitive research (for the NIBIRU / EPS CTO interview)

Research date: 27 Nov 2026. Tags: **[seen]** = read directly on a crawled page or help-centre/API response during this session; **[source]** = from public docs, press or regulators found by search (not crawled by me). Raw data: `/tmp/claude-0/-home-user-Bypass/c485ec14-88d3-5b3d-a0d9-7ea480a3d697/scratchpad/competitors/raw/ticketmaster/` (help centre: `help_articles.txt`, 406 articles; business pages `biz/`; developer portal `dev/`; Live Nation `ln/`; blocked TM pages `tm_events/` and `00_*`/`01_*`). Scripts: `.../scripts/ticketmaster/`.

## Platform overview

- Ticketmaster (TM) is the ticketing arm of Live Nation Entertainment (LN). LN is also the world's largest promoter and a large venue operator, and it owns or operates House of Blues, VIP Nation, Live Nation Premium Tickets, Veeps (livestreams) and others (footer of livenation.com) [seen].
- Its own stated scale: **1.3M tickets sold a day, 100M+ monthly site visitors, 20M+ monthly app users, and the app in 21 markets** (business.ticketmaster.com/solutions/ticket-sales) [seen]. Distribution: "1B+ addressable fans, 1K+ global partners, 30+ Commerce API integrations, 25+ markets" [seen]. It is in about 30 to 37 countries, including Poland (since 2014) and Czechia (Ticketpro, bought in 2017). I found no Romanian operation (Eventim.ro and local players serve Romania) [source].
- Brand portfolio on the business site: **Ticketmaster, Front Gate Tickets** (festivals), **TicketWeb** (clubs), **Universe** (self-serve/DIY events), **Elevate**, the organiser suite **TM1** (TM1 Events / Reports / Demand / Marketing / Entry), the white-label app **Ignite**, **Promoted Ads**, and the **Nexus** partner network [seen].
- Financials: Q2 2026 ticketing operating income was **$264.7M on $852.2M revenue (31% margin)**, against 2.1% for concerts. Ticketing is the profit engine (TicketNews, earnings call) [source].
- Legal position in 2026: the **DOJ settled** its antitrust case mid-trial (TM keeps its ownership; up to 13 amphitheatres divested; 50% of tickets reserved for non-exclusive venues; **service fees capped at 15%**). The **states' case went to verdict on 15 Apr 2026**: a NY jury found that LN/TM illegally monopolised primary ticketing and amphitheatres and tied venues to ticketing. Remedies, possibly a breakup, are pending before Judge Subramanian, and LN is appealing [source].

## What was crawled vs blocked

| Target | Result |
|---|---|
| `www.ticketmaster.com/robots.txt`, `www.ticketmaster.co.uk/robots.txt` (curl) | **403** `{"response":"block"}`: even robots.txt is blocked for non-browser clients [seen] |
| ticketmaster.com home, ticketmaster.co.uk home, `/discover/concerts`, two event pages (Stray Kids at Levi's Stadium, a stadium show; 49 Winchester at August Hall, a club show) via Playwright | **All 403**, served by TM's own "abuse component" (`/epsf/c38f4db4/asset/abuse-component.css`, `block.json`) plus sensor-style beacons at `/{uuid}/{uuid}/tl` and `/mfc` (this URL shape matches a commercial bot-defence vendor such as Kasada; that is inferred, not confirmed). **Per the rules I did not try to get past it**, so I did not see the seat map, ticket picker, queue page or checkout. |
| `trust.ticketmaster.co.uk` | 403 [seen] |
| help.ticketmaster.com (Zendesk) | 200. robots.txt allows `/api/v2/help_center/*/articles`, so I pulled **all 406 en-us articles** through 5 public API pages, plus the home page in Playwright [seen] |
| business.ticketmaster.com (robots: crawl-delay 10, respected) | 200 on 17 pages: home, solutions, TM1 Events, ticket sales, fan experience, event day, marketing, TM1 updates, distributed commerce, Ignite, Nexus, Promoted, arts & theatre, Smart Queue, SafeTix, the AI page [seen] |
| developer.ticketmaster.com | 200: products, Discovery API v2, Partner API, getting started, SDKs (the Commerce, Widgets and Presence pages return 404 now) [seen] |
| blog.ticketmaster.com ("How a Ticketmaster queue works") | 200 [seen] |
| livenation.com home and `/events` | 200. Event cards link out to `ticketmaster.com/.../event/{id}?preferredOfferProvider=LN` [seen] |
| Cart / checkout | Not reachable: the event pages are blocked. No account, payment or order attempted. |

Total: about 40 requests, one at a time, 1.2 to 10 s apart.

## Fan features

**Discovery and personalisation**
- Personalised recommendations, search, and city and category pages (business site) [seen]. LN.com has "Top Picks / Upcoming / Presales / Just Announced" tabs, "+N nearby" date grouping, geo by `market_id`, popular artists by market, and an "ECP" events query sorted by `ursa_ranking` with a `foryou` flag and an `hmac_id` (personalised feed) [seen].
- Event reminders: "SET REMINDER" on an event sends a text **one hour before on-sale** with a direct link [seen, help].
- Artist presale sign-up pages, fan-club presales, sponsor/card presales (**Citi, Chase, Mastercard, AAdvantage, Wyndham Rewards, PENN Play, Audience Rewards**), and promo codes through an "Unlock" button next to Filters [seen, help].
- **Reserved by Spotify**: Spotify gets an allocation and holds tickets for the artist's top listeners during a reservation window, so they need not race the on-sale [seen, help].
- **Ticket Request** (lottery-style): fans pick shows and ticket type and add a card, with a temporary $1 authorisation; the card is charged only if the request is fulfilled. This avoids a first-come race [seen, help].
- **Fan Reviews** on artist pages; the invitation email comes within 72 h of the show and cannot be opted out of [seen, help]. Setlists on artist pages [source, blog].
- Deals: **Me+3 4-packs** (four for the price of three), Summer of Live 2-/4-packs, student rush/lottery/SRO, military (Vet Tix), donations [seen, help].
- Distributed and AI discovery: an **app in ChatGPT** (`@Ticketmaster`, with sponsored placements planned), Gemini, Claude, **Alexa+ launch partner**, Google Event Search and **AI Mode** (agentic), and affiliates through Impact Radius [seen on biz AI page and distributed-commerce page; source for ChatGPT specifics].

**Seat maps and the ticket picker** (could not crawl; from help):
- An interactive seat map with a list view, filters (quantity, price, area), a **Perks filter** ("Ticket Perks": items or experiences per seat), a pink-dot **Verified Resale** layer, and an **Accessible Tickets** button or wheelchair icon that filters accessible and companion seats [seen, help].
- Seat-adjacency rules: **no single seat left open** (you cannot strand a seat at a row end or between occupied seats), and **aisle seats sold only in multiples of 2 or 4** [seen, help].
- Obstructed-view labels, GA/SRO explanations, and an "are my seats together" help article [seen, help].
- Seat types: **Official Platinum** (premium, price set by the organiser and adjustable with demand; TM says "no algorithmic surge pricing"), Standard, VIP packages, and Verified Resale [seen, help]. The UK dropped "platinum" labels after the CMA undertakings [source].

**Pricing display**
- **All-In Price** in the US: the price shown includes the service fee, before taxes and optional delivery fees, on the event list view and seat map. The fee breakdown sits behind a Subtotal dropdown. For the busiest sales TM shows the **All-In price ranges when the sale opens** (an "All-In Prices on Every US Event" badge) [seen, help]. This started on **12 May 2025** under the FTC Junk Fees Rule [source].
- UK (CMA undertakings, Sep 2025): **24 h notice when tiered pricing is used**, clearer information on prices [source].

## Organiser / venue features

- **TM1 Events**: drag-and-drop custom floor plans with inline editors; price levels, ticket types and holds; **reusable templates** (floor plan, pricing, ticket types, offers) for tours, seasons, festivals and multi-venue portfolios; visual seat selection to reclassify inventory and see the revenue effect; reusable presale/VIP/offer templates; a **shared workspace for promoters, agents and box office with permissions, version control and an audit trail** [seen].
- **TM1 Reports / Measurement**: buyer demographics, sales by location, shareable "event groups" to compare events. "17.8M reports viewed in 2024" [seen].
- **TM1 Demand**: one dashboard for demand and on-sale (**artist sign-ups, reminders, Smart Queue metrics, real-time on-sale performance**) [seen].
- **TM1 Marketing** and **LiveAnalytics**: audience segmentation from attendance data, DIY email journeys, audience insights [seen].
- **Promoted Ads**: self-serve native ads and sponsored listings on ticketmaster.com, CPC with no minimums, automatic creative from event data, bid and budget recommendations (Maximum / Recommended / Custom), Nielsen DMA geotargeting, CSV reports. Its figures: "600 clients, 3M tickets, $273M GTV", and a case study claiming "4.7x ROAS" [seen].
- **Upsells and bundles**: VIP upgrades, parking, merch, experiences in the purchase flow, **also extended to resale tickets**; bundles with discounts; travel/hotel packages; collectible digital tickets (NFTs) [seen].
- **Ignite**: a white-label team or venue app with modules for rideshare and parking, next events, venue info, merch, **venue vouchers**, sponsorships, directions, F&B, upgrades, in-app transfer, entry, ticket customisation, an integrated purchase flow and custom discovery [seen].
- **Nexus Partner Network**: 85+ certified partners (CDP/analytics: StellarAlgo, KORE, Two Circles, Pumpjack; POS: Agilysys, VenueNext; loyalty: FanMaker, Givex; app makers: YinzCam; entry hardware: Fortress) [seen].
- **Arts & theatre**: subscriptions and season configuration, automatic renewals, online exchanges and upgrades, **donation flows with round-up at checkout** [seen].
- **Distributed Commerce / Partner API**: list once and sell through affiliates and channel partners. NFL **"Open Distribution"**: season-ticket members list once and reach several marketplaces [seen].
- Festivals (Front Gate): multi-day, camping and add-ons, tiered ticketing [source].

## On-sale, queue and anti-bot

- **Smart Queue** (a virtual waiting room) [seen, biz + blog + help]:
  - The waiting room opens **15 to 30 min before** the sale. At on-sale time everyone in it gets a **randomised place** (so arriving early inside the window gives no advantage over others in the window; arriving after the start puts you at the back).
  - Fans are let in "at a rate that minimises check-out errors and maximises sell-through". **Only a few hundred shop at once** (fewer at small venues). Your turn is **held for 10 minutes**, with an audio bell if you step away.
  - In-queue messaging covers inventory, added dates and prices. Organisers get live demand signals from the queue.
  - Rules: one device and one browser; one tab per show (several shows in several tabs are allowed); **refreshing can get you flagged as a bot**. Presale codes are entered at the front of the queue.
- **Checkout timer** on every page; its length is set by demand [seen, help].
- **Verified Fan**: registration (account, email, phone) before the on-sale. Registrants are screened for bot or scalper risk and purchase history, then invited with codes. TM claims about 5% of Verified Fan tickets reach resale, against 20 to 30% otherwise [source]. Eras 2022: **3.5M registered, 1.5M invited, 3.5bn system requests (4x the previous peak)**, and the public sale was cancelled [source].
- Account-level controls [seen, help]: **Persona ID verification** ("Security Check", SOC2/ISO27001/NIST IAL2); **passkeys** (FIDO); 2FA; phone and email verification; "activity paused or suspended" holds (fixes: enable cookies, switch to mobile data, turn off extensions); ticket limits enforced across accounts by name, email, billing address and card; orders cancelled for limit evasion; and a captcha flow in the Partner API.
- Edge bot defence blocks even robots.txt for automated clients (seen in this crawl).
- Criticism: the **FTC (Sep 2025) alleges TM tolerates brokers' multi-account limit evasion and supplies brokers with "TradeDesk" software**, in breach of the BOTS Act [source].

## Payments, fees, instalments / BNPL, split payment

- Accepted [seen, help]: Visa, Mastercard, Amex, Discover, Diners, debit cards with a card-network logo, **PayPal, Venmo, Apple Pay, Google Pay**, TM gift cards (including Nift eGift). Prices are in the event's local currency. **Only one card per transaction** (plus a gift card). Apple Pay, Google Pay, Diners and gift cards cannot be used on Verified Resale.
- **BNPL**: **Flex Pay by Upgrade** (a "buy now, pay later" loan, not on every event), **Citi Flex Pay** (fixed monthly fee, no interest, $75+; you must finish paying before transferring or reselling, where the organiser allows those). PayPal Pay Later is also available through PayPal [seen, help].
- **Split payment** happens *after* checkout: **Venmo Split Pay** ("Split on Venmo" sends charge requests to friends; on desktop, through a QR code). There is no true multi-payer checkout [seen, help].
- Fees [seen, help + source]:
  - **Face value**: set by the organiser.
  - **Service fee**: per ticket, negotiated with venue or organiser and shared; it may include profit.
  - **Facility charge**: set by the venue; TM keeps none of it.
  - Order processing and delivery fees (status differs by market).
  - **Allianz Ticket Protection** (insurance, 100% reimbursement).
  - Exchange fee per ticket.
  - **Resale: free to list; the seller pays a service fee out of the sale price**.
  - Press estimates of total fees run at 30% or more of face value. The DOJ settlement caps service fees at 15% [source].
- Seller payouts: US bank account or debit-card payout; **1099-K tax ID validation (SSN/ITIN)** before payout; Face Value Exchange payouts typically arrive within 7 business days after the event [seen, help].

## Post-purchase

- **SafeTix**: an encrypted **rotating barcode refreshing every 15 seconds** (help) or "every few minutes" (biz), with an animated design, screenshots and recordings blocked, **device binding**, and a tie to the verified account. "Screenshots won't get you in" [seen].
- **Delayed barcode release**: for some events barcodes appear only about 3 days before the event ("delivery delay"), to fight speculative resale [seen, help].
- **Apple Wallet / Google Wallet** passes, which work offline, with **NFC "tap and go" contactless entry** (Google Wallet must be the default NFC app) [seen, help]. A wallet ticket becomes "invalid" once transferred or sold [seen].
- Other delivery: Mobile Entry, Print at Home (limited), mail/UPS, Will Call [seen, help].
- **Ticket Transfer**: free, all or some tickets, account to account. **The recipient gets a new barcode** and the sender's goes dead. The **organiser decides when transfer opens** (often not until close to the event). Add-ons and extras transfer separately if the organiser allows. VIP elements and insurance do not transfer [seen, help].
- **Resale**:
  - **Verified Resale** (organiser-enabled; the seller sets the price in the US).
  - **Face Value Exchange** (capped at the price paid, free for sellers).
  - **UK and Ireland resale capped at face value since 2018**.
  - Third-party resale by "trusted sellers" for non-TM primary events.
  - A **CashorTrade** partnership (fan-to-fan at face value or below).
  - Organiser-gated eligibility [seen, help; UK cap source].
- **Upgrades**: an "Upgrade" button on the order, where the organiser enables it. Also seat moves and exchanges for another date (24/48/72 h notice, a per-ticket fee, primary orders only) [seen, help].
- Add-ons after purchase from Order Details: parking passes, **Fast Lane** entry passes (also sold separately until the day before), merch through **Fanatics**, hotel packages, VIP packages [seen, help].
- **Digital collectibles (NFTs)**, exportable from my.ticketmastercollectibles.com [seen, help].
- Event status (cancelled / postponed / rescheduled / moved) with automatic refunds for cancellations [seen, help].

## On-site / day-of features

- SafeTix entry with **colour-coded scan feedback and sensory feedback on scanners** to make staff training quick [seen].
- **"Know Before You Go"** pre-event communications (mobile access, transfer, add to wallet) [seen].
- **TM1 Entry**: advanced validation; **re-entry and in-venue movement even after a transfer; exit scans**; organiser-adjustable ticket rules [seen].
- Entry and venue partner integrations (Fortress and others) [seen]; on-site seat upgrades and group offers through Nexus "specialty ticketing tools" [seen].
- Ignite in-venue modules: F&B ordering, venue vouchers, merch, parking and rideshare, directions [seen].
- Parking and Park & Ride passes (Hollywood Bowl); venue info (bag policy, camera policy, lawn chairs, lost and found) [seen, help].

## AI / data features

- AI across discovery, search, purchase and support (biz AI page) [seen]:
  - structured event data, schema and real-time availability tuned for Google AI Overviews and AI Mode ("4x the SEO traffic of competitors, +16% YoY");
  - Alexa+;
  - an app in ChatGPT;
  - **a conversational AI virtual assistant** for tickets, refunds and account questions, with smart routing to agents. "1.25M fan inquiries a month; 36M proactive notifications in 2024" [source].
- Marketplace ranking from "demand signals, event metadata, seat attributes, pricing data" [seen].
- **AI for pricing** of unsold inventory is on the roadmap (Q2 2026 earnings call) [source].
- Fan data: SafeTix chain of custody (purchase, then transfer, then scan) is sold to organisers as "verified fan data" for marketing; also LiveAnalytics and the TM1 Marketing segmentation [seen].

## Observed tech stack and APIs

- **ticketmaster.com**: edge "abuse component" (`/epsf/<build>/asset/abuse-component.css`, `tm.css`, `block.json`), sensor endpoints `/{uuid}/{uuid}/tl` and `/mfc`, OneTrust (cookielaw) consent on .com, ContentSquare session analytics, GA4 (two properties) [seen].
- **help.ticketmaster.com**: Zendesk Guide (`ticketmaster-us.zendesk.com`), public Help Center API `/api/v2/help_center/en-us/articles.json`, trending articles by `view_3days_count`, Ketch consent (`global.ketchcdn.com/.../live_nation/ticketmaster_com`), Quantcast, `cdn.p-n.io` [seen].
- **livenation.com**: Next.js App Router (RSC `?_rsc=` fetches); **GraphQL at `api.livenation.com/graphql`** (operations `SEARCH_POPULAR_ARTISTS`, `ECP`→`getEvents(market_id, top_selling, foryou, hmac_id, sort_by: ursa_ranking, …)`); a content API `content.livenationapi.com/v1/modules/presales/1/content?market_id=…` and `/v1/featured-genres`; `/api/geo.json`; Ketch consent; Branch deep links (`livenation.app.link`) [seen].
- **Developer platform** [seen]:
  - **Discovery API v2** (`app.ticketmaster.com/discovery/v2/`): events, attractions, venues, classifications, suggest; sources TM, Universe, FrontGate, TMR resale; **5,000 calls a day, 5 req/s** by default; `Rate-Limit-*` headers; CORS.
  - **Partner API** (`/partners/v1`, distribution partners only):
    - cart reserve with **polling** and captcha token, hold time in the response, non-adjacent and multi-price-level reserves;
    - **Braintree** payment nonce; an encryption certificate for card data;
    - commit or delete cart; order redemption and claim flow; refunds;
    - ADA, resale listings, password-protected offers, All-In fee display, co-branding;
    - **back-end reservation hosts go into maintenance 02:00 to 04:00 local event time** (a sign of a legacy, regionally sharded "Host" core).
  - Other APIs: Top Picks, Inventory Status, Season Ticketing, International Discovery (being merged into Discovery), Publish API; **Presence SDK** (embed tickets and entry in team apps), Purchase SDK, Ignite SDK.
- Identity: passkeys, Persona KYC, Sign in with Apple/Google (being retired), 2FA [seen, help].

## Distinctive features (what is genuinely hard to copy)

1. **Scale-tested on-sale stack**: Smart Queue + Verified Fan + Persona + edge bot defence + demand dashboards (TM1 Demand), with real-time in-queue messaging and organiser control over the admission rate. Nobody else has a queue at Eras-level traffic in production.
2. **Several allocation mechanisms besides first-come**: Verified Fan invites, **Ticket Request (lottery with $1 authorisation)**, **Reserved by Spotify** holds, card-member presales, fan-club codes.
3. **SafeTix chain of custody**: a rotating 15 s token, device binding, NFC wallet, re-issue on transfer, delayed barcode release, exit and re-entry scans. One identity graph runs from purchase to gate, and it feeds the resale controls.
4. **Closed-loop resale under organiser policy** (Verified Resale, Face Value Exchange, UK face-value cap, CashorTrade), with upsells extended to resale tickets.
5. **Organiser suite depth**: TM1 floor-plan editor, reusable templates, a collaborative workspace with an audit trail, Promoted Ads (self-serve CPC on the marketplace), Ignite white-label app, 85+ Nexus integrations.
6. **Distribution and AI-channel reach**: Partner API, affiliates, ChatGPT, Alexa+, Google AI Mode, NFL Open Distribution. The marketplace's demand is itself the product sold to organisers.
7. Seat-map rules that protect fill rate (no orphan seats, aisle pairs), Ticket Perks per seat, accessible-seat filters.

## Weaknesses and criticism

1. **Monopoly and trust**: jury verdict of illegal monopolisation and tying (Apr 2026), with a breakup still possible. The DOJ settlement's 15% fee cap. FTC suit (Sep 2025) over drip pricing and broker tolerance (**TradeDesk**). The Senate PSI minority report "So Casually Cruel" (Mar 2026) on dynamic pricing and pushing tickets to resale before the general sale [source]. Fans and venues see the incumbent as the problem.
2. **Fees and pricing anger**: fee stacks of 30% or more; Official Platinum and "in-demand"/tiered pricing (Oasis 2024, £148.50 to £337.50; CMA undertakings Sep 2025); an EU Commission look at dynamic pricing (2024); All-In arrived only under regulatory compulsion [source]. Price is opaque until the queue releases you.
3. **On-sale failures**: Eras 2022 (site overload, public sale cancelled), the France Eras queue crash in 2023; "do not refresh or you're a bot"; first-come scarcity is still the default [source].
4. **Security and privacy**: the 2024 Snowflake-linked breach (ShinyHunters claimed 560M records) [source]; heavy KYC (Persona ID checks, SSN for sellers) adds friction and data risk.
5. **Rigid post-purchase**: one card only; split pay only after the fact through Venmo (US); transfers locked until the organiser opens them; barcodes released late; exchanges need Fan Support, a fee and 24 to 72 h notice; Fan Review emails cannot be opted out of; help articles get poor votes (e.g. "How do I buy tickets online?": 1 up, 5 down) [seen].
6. **Legacy core**: nightly 02:00 to 04:00 local maintenance window on transacting hosts, polling-based reserve API, a fragmented brand stack (TicketWeb, Universe, Front Gate, TM1) [seen].
7. **Closed ecosystem**: entry works best in the TM app; screenshots are blocked and fans must hold a TM account; the Partner API is restricted to official partners; the open Discovery API is read-only with a low quota.
8. **No presence in Romania**, and its features are US-first (Venmo, Citi, US bank or SSN for resale payouts).

### Takeaways for an EPS "next-gen" pitch (my inference)
- To match TM on the basics: a waiting room with a randomised start window, ID-bound rotating QR and NFC wallet passes, transfer that re-issues the ticket, and face-value resale under organiser policy.
- Where EPS can beat it:
  - real split payment at checkout and RO/EU instalments (e.g. card instalments, not a US BNPL);
  - transparent pricing by default;
  - fair allocation (lottery or request) for festivals;
  - a single modern multi-tenant core with no maintenance windows;
  - resort and festival bundles (accommodation, cashless wristband, camping) as first-class products;
  - an open API for organisers.

## Source URLs

Crawled [seen]:
- https://help.ticketmaster.com/hc/en-us and https://help.ticketmaster.com/api/v2/help_center/en-us/articles.json?per_page=100&page=1..5 (406 articles). Key ones: What-is-the-queue-and-how-do-I-join (9781366115985), What-is-the-All-In-Price (35127799858193), How-are-ticket-prices-and-fees-determined (9663528775313), Why-do-my-tickets-have-a-moving-barcode (26584309038353), How-does-Ticketmaster-s-Face-Value-Exchange-work (9781464415249), What-are-Platinum-Tickets (9782440112017), How-does-Flex-Pay-by-Upgrade-work (52879596899473), What-is-Citi-Flex-Pay (47333589088657), What-is-Venmo-Split-Pay (44520768823697), What-are-the-accepted-payment-methods (9613844661009), Reserved-by-Spotify (47849883539473), How-do-I-request-tickets-for-my-event (40436800695953), Security-Check-through-Persona (37371579224209), passkeys (37580664256145), How-does-Ticket-Transfer-work (9786975926673), Why-can-t-I-access-my-mobile-tickets-yet (18357208731153), What-are-Ticket-Perks (46658694846737), CashorTrade (45504735218193)
- https://business.ticketmaster.com/ , /solutions/ , /solutions/event-creation-management/ , /solutions/ticket-sales/ , /solutions/fan-experience/ , /solutions/event-day/ , /solutions/marketing-and-measurement/ , /solutions/tm1-updates/ , /distributed-commerce/ , /ignite-solution/ , /nexus-partners/ , /promoted/ , /arts-and-theaters/ , /smart-queue/ , /business-solutions/safetix/ , /how-ticketmaster-uses-ai-to-enhance-event-discovery-fan-support-and-experiences/
- https://blog.ticketmaster.com/how-ticketmaster-queue-works/
- https://developer.ticketmaster.com/products-and-docs/ , /apis/discovery-api/v2/ , /apis/partner/ , /apis/getting-started/ , /sdks/
- https://www.livenation.com/ , https://www.livenation.com/events
- Blocked (403): https://www.ticketmaster.com/ , https://www.ticketmaster.co.uk/ , https://www.ticketmaster.com/discover/concerts , two event pages, https://trust.ticketmaster.co.uk/security-and-tech

Press / regulators [source]:
- https://www.axios.com/2026/04/15/live-nation-ticketmaster-lose-antitrust-case ; https://ca.billboard.com/business/legal/live-nation-verdict-jury-says-concert-giant-is-an-illegal-monopoly-in-total-defea
- https://www.nbcnews.com/business/business-news/ftc-sues-ticketmaster-live-nation-alleged-illegal-ticket-resale-tactic-rcna232147 ; https://www.pbs.org/newshour/nation/ftc-sues-ticketmaster-saying-it-uses-illegal-tactics-to-make-fans-pay-more-for-live-events
- https://www.pbs.org/newshour/nation/a-biden-era-junk-fee-rule-is-now-in-effect-ticketmaster-says-it-will-display-fees-more-clearly
- https://www.gov.uk/government/news/cma-secures-changes-from-ticketmaster-following-oasis-tickets-investigation ; https://www.which.co.uk/news/article/oasis-tickets-ticketmaster-to-make-pricing-clearer-following-cma-probe-a99yW5M0BI4E
- https://www.hsgac.senate.gov/wp-content/uploads/2026_03_16-Live-Event-Ticket-Report-1.pdf
- https://www.ticketnews.com/2026/07/ticketmaster-outlines-ai-driven-ticket-pricing-future-in-earnings-call/
- https://www.marketplace.org/story/2026/05/15/why-does-ticketmaster-charge-such-high-fees
- https://business.ticketmaster.com/press-release/taylor-swift-the-eras-tour-onsale-explained/ ; https://fortune.com/2023/07/11/ticketmaster-france-crashes-taylor-swift-eras-tour-1-million-queue/
- https://www.nbcnews.com/pop-culture/ticketmaster-says-verified-fan-registration-doesnt-measure-fandom-fans-rcna81187
- https://www.bleepingcomputer.com/news/security/ticketmaster-confirms-massive-breach-after-stolen-data-for-sale-online ; https://www.theregister.com/2024/05/31/snowflake_breach_report
- https://business.ticketmaster.co.uk/why-ticketmaster-supports-resale-caps-in-the-uk-and-has-done-for-years/
- https://business.ticketmaster.com/ticketmaster-drives-a-new-era-of-ai-powered-event-discovery-with-app-in-chatgpt/ ; https://www.musicbusinessworldwide.com/ticketmaster-launches-app-within-chatgpt-bringing-event-discovery-to-openais-chatbot/
- https://www.musicbusinessworldwide.com/ticketmaster-launch-30-countries-acquisition-ticketpro/
- https://techcrunch.com/?p=1828729 (SafeTix launch)
