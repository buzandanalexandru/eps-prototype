# Self-service organiser platforms: Eventbrite, Tixr, Universe

Research date: 27 Nov 2026. This is competitive research for the NIBIRU / EPS CTO interview.

Tags: **[seen]** means observed directly in our own crawl (page text, buttons, network calls). **[source]** means taken from public sources (help centres, vendor blogs, press, review sites). Raw crawl output is in `raw/organiser-platforms/` and the scripts are in `scripts/organiser-platforms/`.

Method and rules followed: robots.txt was checked first for each host. Requests went one page at a time with about 1 s between them. We used no stealth and created no accounts. Nothing was ordered, and no payment details were entered. The crawl stopped at the ticket picker ("Continue" was never pressed). Where a host served a bot challenge, it was noted and skipped.

---

## 1. Eventbrite

### Overview
- Eventbrite is the largest self-service marketplace for ticketing and registration. It claims 90M+ unique ticket buyers, 4.7M events a year and nearly 180 countries [seen, organiser pages]. Its focus is small and mid-size events: nightlife, classes, community events, food and drink, and festivals at the lower end of the market.
- **Ownership:** Bending Spoons took Eventbrite private in an all-cash deal of about $500M ($4.50 a share). The deal closed in early March 2026, and large layoffs followed in April 2026. Andrea Parodi now leads the company. Bending Spoons has named a secondary-ticketing system, messaging and AI event creation as product directions [source: Billboard, heise, IQ, MBW]. The product has shipped quickly since then: "81 new features and improvements since April 2026" [seen, /product-updates].
- **Model:** free for free events, and a per-ticket fee for paid tickets. The buyer pays the fee by default, or the organiser can absorb it [seen].

### What was crawled vs blocked
- **Crawled [seen]:** about 30 pages. These were the homepage, discovery pages (/d/romania/all-events, London music), three London nightlife event pages with their **ticket pickers**, the organiser overview, all-features, pricing, sell-tickets, email marketing, Eventbrite Ads, promo codes, event payment, check-in app, Tap to Pay, reserved seating, timed entry, marketing platform, music festivals, instalment payments, collections, waitlist, the developer platform page and the monthly product-updates log (April to November 2026).
- **Blocked or skipped:** `/checkout-external*` is disallowed in robots.txt. The ticket picker is an iframe on that path. It loaded only because a user-style click on "Get tickets" opened it on three event pages, and we went no further. The checkout carries an **hCaptcha** frame [seen], which we did not touch. The eventbrite.co.uk organiser pricing URL returned 404.

### Fan features
- **Discovery** [seen]: a marketplace with city and category pages, search, personalised recommendations, an app, newsletters, "Follow" organisers (US buyers now follow the organiser automatically, Sep 2026) and "Like" events. The "Remind me" button (pre-sale interest) also adds the fan to the organiser's marketing list.
- **Organiser trust signals on the event page** [seen]: a "TOP ORGANISER" badge, follower count, number of events, years hosting and total attendees ("9k followers, 220 events, 8y hosting, 152.1k total attendees"). There is also a "Report this event" link.
- **Urgency badges** [seen]: "SALES END SOON", "Last chance — sales end soon", "Few tickets left", "Sales end in a day". These are backed by an `/e/api/{id}/urgency-signals` call.
- **Ticket picker** [seen]: an inline modal with a promo code field and a list of ticket types. Each shows the fee-inclusive price ("£13.44 incl. £1.44 Fee"), a quantity selector capped per type, an end-of-sales date, a description and sold-out tiers left visible ("1st Release – Sold out", which reads as social proof). It also showed a "2 for 1 deal" (bring-a-friend) ticket, free time-limited tickets ("Ladies free before midnight"), VIP table packages, "Queue Jump" add-ons and an 18-language switcher. "You won't be charged yet" appears on the Continue button.
- **Dynamic pricing disclosure in the picker** [seen]: "Prices might change over time. Your price isn't based on information about you. Price may vary."
- **Payment** [source]: Apple Pay, Google Pay (a Google Pay frame was seen), PayPal, and instalments only through PayPal Pay Later, where eligible.
- **Tickets** [source + seen in product updates]: in the app and on the mobile web, with Apple and Google Wallet added Apr 2026 and offline tickets cached in the app (Apr 2026). The organiser can delay QR release to 24 or 48 hours before the event (Oct 2026).
- **AI discovery** [seen]: since 5 Jun 2026, "Get your events discovered in ChatGPT". Events surface when people search a city in ChatGPT.

### Organiser features (detailed)
**Onboarding and event creation**
- Self-serve sign-up, "create an event in seconds", and **AI event creation** that writes the copy for listings, emails and ads ("launch campaigns 30% faster") [seen].
- Event types are single, recurring and timed entry. Timed entry supports up to 2,000 active slots (Jun 2026). Recurring events can have a different price per date [seen].
- Other features: an event page builder, collections (groups of events on one page), an organiser profile (rebuilt Apr 2026 with a cover image, past photos and a calendar view, plus a pinned featured event), an event flyer generator with a QR code (Apr 2026), save draft, and copy an event together with its emails [seen].
- Team roles, with access scoped to an organiser profile (Aug 2026). "Contact the organizer" messages are routed by permission [seen].

**Ticket types and inventory**
- Paid, free and donation tickets. Early bird, group tickets with a quantity discount, BOGO ("automated buy one get one free"), and hidden tickets revealed by a code [seen].
- **Time-bound tickets** (Apr 2026): set when each ticket type can be scanned, for example early access first [seen].
- **Reserved seating**: a drag-and-drop seating chart maker with sections, rows, tables and objects such as stages and bars, best-available assignment, and tiered pricing by area. The waitlist does not work with reserved seating [seen + source].
- **Add-ons and merchandise**: add-ons have their own checkout step after the tickets (Sep 2026), work in reserved seating and in the embedded checkout (Nov 2026), and can be bought with no ticket (Oct 2026). One-click duplicate [seen].
- Custom order-form questions, asked before or after payment and reorderable by drag and drop [seen].

**Pricing tools**
- **Promo codes**: percentage or fixed discount, reveal hidden tickets, track performance by code, bulk enable and disable (Sep 2026) [seen].
- **Automatic price changes / "Dynamic" price** (Aug to Sep 2026): up to 10 rules per ticket type, triggered by date, by quantity sold or by a countdown to the event, all of one trigger type, in priority order [seen + source help article 461515]. This is rule-based tier progression, not algorithmic pricing. Network calls `POST /e/api/{id}/dynamic-pricing-preview` and `POST /api/v3/dynamic_pricing/price/` were seen on the event pages.
- **Waitlist**: when a ticket frees up, the next person gets a limited time to claim it, and the offer then moves down the list. "Automated waitlists" are on the Nov 2026 roadmap [seen + source].

**Marketing**
- **Email campaigns**: 250 emails a day free; **Eventbrite Pro** from $15/month gives 2,000 to 10,000 a day. Unlimited contacts, buyers added to lists automatically, segments by past attendance and purchases, lists by city (Nov 2026), templates, scheduling, campaign ROI reporting and post-event emails (May 2026). Eventbrite claims open rates 59% above the industry average [seen].
- **SMS campaigns**: paid, opt-in only, US first (Nov 2026) [seen].
- **Eventbrite Ads**: paid on-marketplace placement in search, category pages, the homepage and the app. The organiser picks a goal (awareness, traffic, or attendance in beta) and a budget, and Eventbrite targets and places the ads. Claims: "4× more tickets", "9× more impressions", "30% higher CTR than Facebook Ads". Available in select markets only [seen].
- **Facebook and Instagram ad tools** with "Smart Audiences" built on Eventbrite data, Instagram Story ads and automated multi-event ads. Also: sell on Facebook, automatic Facebook Event publishing (US and Canada, Nov 2026), sell on TikTok (For You page and one-click buy), share to LinkedIn, and a Canva design integration [seen].
- **Tracking pixels** (Meta and others), **tracking links** for affiliates and promotions, retargeting ("6× ROI"), Seeka server-side ad tracking (Jun 2026) and in-app purchases attributed to ad trackers (Sep 2026) [seen]. There is **no built-in fan-referral or ambassador reward programme**; tracking links are the closest thing.
- A **Rokt** integration on the event and confirmation pages: post-purchase third-party offers, which are an ad revenue line for Eventbrite (calls to `apps.rokt-api.com`, `/home/api/rokt`, `/e/api/rokt`) [seen].

**Payouts and finance**
- Eventbrite Payment Processing is the default, with PayPal as an alternative in some markets [seen + source].
- **Instant Payouts** (funds in minutes), **pre-event scheduled payouts** and consolidated payouts for recurring events [seen].
- Automated refunds and policies, bulk refunds by ticket type or add-on (Nov 2026), order transfers to another date, type or event (repeatable, with a refund allowed afterwards), chargeback protection and customised fee structures [seen].
- Known friction: payouts can be held for risk or KYC review, and Eventbrite may claw back refunds from the organiser's other events [source: BBB, G2, merchant agreement].

**Reporting**
- Real-time dashboards. Cross-event sales over time (Jul 2026), add-on revenue, sales by ticket type, a report across all sessions, reports filtered by organiser profile, chosen export columns that are remembered, and custom-question exports. "Traffic & Conversion" now filters out bot views (Jul 2026). Sold-out alerts (on by default, Nov 2026). Pre-sale interest data ("Remind me", saves, views, shares) [seen].

**On-site**: see below.

**APIs and integrations**
- **REST API v3** [seen + source], with a free API key, OAuth, and endpoints for organisations, events, ticket classes, orders, attendees and venues. **Webhooks** fire on order placed, updated and refunded, attendee updated, checked in and checked out, and event, organiser, ticket class and venue changes. Webhooks can be set per organisation. The public event search API was retired years ago, so there is no open catalogue feed [source].
- **Embedded checkout widget** (`eb_widgets.js`), as a popup or an inline container. Only the launch button can be styled, not the checkout itself, and HTTPS is required [source].
- An App Marketplace with "100+ apps", including Mailchimp, Zoom, Facebook Live, YouTube, Seeka and Canva [seen].

**White-label**: no true white label. The organiser profile and event pages are customisable, but the experience stays Eventbrite-branded ("Powered by Eventbrite" in the widget [seen]).

### On-sale / queue / anti-bot
- **A waiting room is built in**: `queue_rpc/status/{eventId}` and `inline.eventbrite.co.uk/queue_rpc/renew/...` were polled on every event page [seen]. Product updates: "More reliable waiting rooms — tickets keep selling even when thousands arrive at once" (Aug 2026), "Waiting room position — attendees see their exact place in line" (Sep 2026) and real-time sold-out shown in the waiting room (Jul 2026) [seen].
- **hCaptcha** is embedded in the checkout iframe, invisible until it is triggered [seen].
- Per-ticket-type quantity caps (for example a maximum of 1, 2, 5 or 10) [seen]. Delayed QR release fights early resale (Oct 2026) [seen]. "Keeps your business safe from fraudulent activity" [seen]. A `creator-quality-signal` endpoint is used for organiser risk scoring [seen; robots-disallowed endpoint, not fetched].

### Payments, fees, instalments, split pay
- **US**: 3.7% + $1.79 service fee per paid ticket, plus 2.9% payment processing per order. Free events cost nothing [seen, pricing page].
- **UK**: about 6.95% + £0.59 per ticket combined [source]. In the picker, a £12 ticket showed a £1.44 fee and a £350 table showed £42, so about 12% all-in at the low end [seen]. **EU**: fees vary by country and package (for example 3.5% + €0.49, or 5.5% + €0.99 for higher packages) [source]. The fee is shown included in the price (all-in pricing) in the UK and EU pickers [seen].
- **Plans** [seen]: Pro, a subscription from $15/month for more email sends; "Pro plan" and "Sales partnership" tiers (custom, with a strategic advisor, Eventbrite-sponsored marketing and priority support). Plans are available in the US, CA, AU, NZ, UK and IE only.
- **Instalments**: no native plan. They come only through PayPal Pay in 3 / Pay Later, where the organiser is paid up front and PayPal carries the credit risk [seen, /features/instalment-payments]. No split payment between friends.
- An `/api/v1/checkout/ebplus/evaluate` call (likely an "Eventbrite+" or upsell eligibility check) and `express_checkout_conditions` were seen [seen; purpose not confirmed].

### Post-purchase
- Confirmation email (custom message, custom question answers, automatic emails on date or venue changes), wallet passes and offline tickets [seen].
- **Transfers**: no fan-to-fan transfer between accounts [source: help centre]. Organisers can move orders between dates and events, and attendees can make self-service changes if the organiser enables them [seen + source].
- **Resale**: none native. A partnership with **Tixel** (price-capped, organiser-controlled resale) [source: Eventbrite blog]. Bending Spoons has named secondary ticketing as a goal [source].
- Refunds are set by the organiser's policy. Eventbrite expects refunds on cancellation [source].

### On-site
- **Eventbrite Organizer app** (iOS and Android): QR scanning, bulk check-in and check-out with ticket-type filters (Oct 2026), multi-slot scanning, live attendance and no-shows, will-call and guest list, door sales of tickets and merchandise, and barcode download 3× faster (Jul 2026) [seen].
- **Tap to Pay** on the phone, with no reader hardware. Manual card entry on Android for CAD, AUD and GBP [seen]. Apple Pay and Google Pay at the door [seen].
- No RFID, cashless or wristband system, and no access-control hardware. BOCA printing is not mentioned. Tickets are printable PDFs [seen].

### AI / data features
- AI copy for event creation, ads and emails; ChatGPT discovery; "Smart Audiences" built on marketplace data; personalised recommendations; bot-filtered traffic analytics [seen].
- An OpenAI-hosted pixel (`bzr.openai.com` / `bzrcdn.openai.com`) fires on Eventbrite pages, alongside Google, Pinterest, Spotify and ZoomInfo pixels and Contentsquare session recording [seen]. The OpenAI pixel is probably for measuring ChatGPT ads or commerce, but that is not confirmed.

### Observed tech and APIs [seen]
- Consumer back ends for front ends: `/home/api/search/`, `/home/api/v3/promoted/events/` (paid placement), `/api/v3/destination/events/`, `/home/api/geo/place_from_request/` and nearby cities.
- Event page: `/e/api/{id}/ticket-information`, `/urgency-signals`, `/dynamic-pricing-preview`, `/me`, organiser profile and follow-status, and `/api/v3/events/{id}/tracking_beacons/` (the organiser's pixels).
- `conversions-manager/api/v1/attribution/event`, `switchyard/{uuid}` (experimentation or routing), `metrics/ag/g/c`, and Transcend consent management.
- Checkout: `purchase-flow-webapp` (express checkout conditions), the `/checkout-external` iframe, hCaptcha and the Google Pay frame.
- An affiliate network: `eventbrite.pxf.io` (impact.com).

### Distinctive
1. Its **demand marketplace**: Eventbrite claims about 25 to 30% of paid tickets come from its own discovery (app, search, newsletters), and it sells the self-serve **ads inventory** on top.
2. **An all-in-one marketing suite** with emails, SMS, social ads and smart audiences, priced per send.
3. **Truly self-serve** with a fast shipping rate: monthly public product updates.
4. **Instant payouts** and Tap to Pay with no hardware.

### Weaknesses / criticism
- High fees for buyers on cheap tickets (about 10 to 12% on a £12 ticket) and anger at fees on refunds [source: G2, Elliott, Trustpilot].
- **Payout holds** and KYC demands after events, slow support, and scam or "organiser didn't refund" complaints. Eventbrite pushes responsibility to the organiser; the BBB shows 477 complaints in 3 years [source].
- No native resale and no account-to-account transfer, weak for festivals. Instalments only through PayPal. No RFID or cashless. No real white label. Embedded checkout cannot be styled.
- Festival depth is shallow: no camping, parking or hotel packaging, and no membership or loyalty. Organisers are funnelled toward ads.
- Its owner Bending Spoons is known for heavy cost cutting and price rises after acquisitions (Evernote, Vimeo, WeTransfer), which creates organiser uncertainty [source: press].
- Rokt post-purchase ads and heavy third-party tracking on pages; a GDPR concern for EU organisers [seen].

### Sources
- Crawled: eventbrite.com (/, /organizer/overview, /organizer/features/*, /organizer/pricing, /features/instalment-payments, /features/waitlist, /platform, /product-updates, /d/romania/all-events, /b/united-kingdom--london/music), and eventbrite.co.uk events 2004647757406, 2003811312577 and 2002915585433.
- [Help: automatic price changes (461515)](https://www.eventbrite.com/help/en-us/articles/461515/)
- [Platform webhooks](https://www.eventbrite.co/platform/docs/webhooks)
- [Embedded checkout docs](https://amp.eventbrite.es/platform/docs/embedded-checkout)
- [Fee table (UK help 755615)](https://www.eventbrite.co.uk/help/en-gb/articles/755615/)
- [Transfer help](https://www.eventbrite.be/help/en-gb/articles/431834/how-to-transfer-tickets-to-someone-else)
- [Billboard: Bending Spoons deal](https://www.billboard.com/pro/eventbrite-acquisition-bending-spoons-500-million/), [heise: layoffs](https://heise.de/-11257427), [IQ: layoffs](https://iqmagazine.com/2026/04/eventbrite-makes-layoffs-following-bending-spoons-acquisition/), [MBW: secondary ambitions](https://www.musicbusinessworldwide.com/eventbrite-eyes-jump-into-secondary-ticketing-market-after-500m-acquisition-by-bending-spoons)
- [BBB complaints](https://prod.bbb.org/us/ca/san-francisco/profile/online-event-registration/eventbrite-inc-1116-76754/complaints?page=1), [G2 reviews](https://www.g2.com/products/eventbrite/reviews), [Ticket Tailor review summary](https://www.tickettailor.com/blog/eventbrite-reviews-summary-pros-cons)

---

## 2. Tixr

### Overview
- Tixr, based in Santa Monica and founded in 2012, is a "live events operating system" for large and complex events: festivals, nightlife, pro sports (MLS, NWSL), motorsports, comedy and destination events. It claims 700+ partners [seen]. Festival clients include Lightning in a Bottle, Splash House, Riot Fest, Elements, Gulf Coast Jam, Zamna (Tulum, Ibiza, Egypt, India), Forbidden Forest (UK) and Eden Ibiza [seen].
- It opened a **London office in July 2024** for its European expansion, led by an ex-Festicket executive [seen]. VENU made a strategic investment in Sept 2025 alongside an exclusive 5-year deal [source].
- **The product is three pillars on one data model**: **Tixr** (the fan purchase experience), **Studio** (the organiser back end, with full mobile parity) and **Door** (the on-site iOS app and box office) [seen].
- Its sales motion is a **demo**, not self-serve sign-up. A "self-serve tier" exists for smaller organisers. Above it, every client gets a dedicated Client Experience Manager [seen].

### What was crawled vs blocked
- **Blocked:** `www.tixr.com` (homepage, events, festival pages, the ticket picker, `/groups/help/`, `studio.tixr.com`) all return **403 with a DataDome challenge** (`geo.captcha-delivery.com`), to curl and to headless Playwright alike. Per the rules, we did not try to get past it, so **no Tixr event page, festival page or ticket picker was crawled.** `clientsupport.tixr.com` (Intercom) shows only a login shell; its articles are behind the Studio login.
- **Crawled [seen]:** `creators.tixr.com`, the organiser marketing site (Webflow, open, with a sitemap of 239 URLs). That was 28 pages: home, products, Tixr (fan), Studio, Door, Platform, Integrations, Reserved Seating, Memberships, and product and case-study posts on Resale, Rewards, Waitlist, high-demand on-sales, fraud and scalping, Order Protection, Fan Transfers, Memberships, Stateside, LIB, Zamna, Europe, AI and Tixr Arena. The FAQ answers come from the raw HTML (they are collapsed accordions on the page).

### Fan features
- **Event pages**: white-label, organiser-branded, with "dynamic page layouts" and "multi-dimensional navigation" across tickets, VIP, hotel packages, camping, parking, merchandise, food and drink, and travel, all in **one checkout ("unified commerce")** [seen].
- **The Tixr app** holds tickets, memberships (a card with the member's name and a numbered Member ID: "first fan to join is #1"), post-purchase add-ons, transfers, resale and upgrades [seen]. Organisers can also get **custom-branded iOS apps** (for example Houston Dynamo and San Jose Earthquakes team apps) [seen].
- **Payment plans** (layaway) [seen + source]. **Waitlist**: a pre-authorised card holds the fan's place in the queue [seen]. **Follow** an event, and interest capture [seen].
- **Rewards**: each buyer gets a personal share link and earns points when friends buy through it, redeemed for **cash back**, merchandise, upgrades or experiences in tiers, with an optional **leaderboard** [seen].
- **Reserved maps**: view from the seat, best available, ADA seats colour-coded, and resale listings shown on the map [seen]. Food and drink order-ahead with pickup [seen].
- **Order Protection** (Teak, formerly Protecht) is offered at checkout with three cover levels; claims are paid by Teak, not the organiser [seen].
- **Returns to credit**: the organiser can let fans return a ticket for platform credit instead of cash [seen].
- **Fan support**: the organiser chooses Tixr Fan Support (fansupport.tixr.com, 7 days a week) or handles support itself [seen].

### Organiser features (detailed)
**Onboarding**: a demo, then guided onboarding with role-based training, migration help (event setup, inventory, permissions, seating maps built from the venue manifest, data migration) and a knowledge base. "First-time users can build a GA event with tiers, content, and add-ons in one short session." Complex events go live in days to weeks [seen].

**Event creation and inventory (Studio)**
- GA, timed entry (time slots and capacity windows, enforced at scan), reserved seating and hybrid GA plus reserved on one map [seen].
- **Reserved Seating engine**: branded, 3-D, multi-level maps that Tixr builds or the organiser builds itself. It covers seats, **campsites, RV spots, cabanas, tables, suites, parking and booths** ("reserve a campsite just like selecting a seat"). Holds, kills, comps, phased releases (sections held back and released in waves), season tickets with renewals and invoices, group sales blocks, agent checkout, orphan-seat leniency, consecutive-seat enforcement, seat-hold expiry (4 to 6 minutes recommended), and pick-a-seat or best-available settings [seen].
- **Price Levels and Components**; **Price Books** (prices by section, audience or sales channel, changed live mid on-sale) [seen].
- **Memberships** (launched 2026): paid monthly or yearly plans built in Studio, offered at checkout on event pages. Perks include member presales, member pricing with no code, credit banks spent at checkout, and exclusive perks. Failed payments are retried automatically, members can cancel or resume themselves, and the membership card is recognised at the door [seen]. Good Vibez ran a "festival passport" membership: pick 3 festivals a month, or an annual pass to all of them [seen].
- Multipasses ("any three events of your choice", Zamna) [seen].

**Pricing tools**
- **Tier progression**: multiple price tiers within one ticket type that advance automatically as inventory sells ("Eager Beaver" → next tier, with the old tier disappearing). Pairing it with the waitlist means waitlisted orders are taken at the top price [seen].
- **"Behavioral Pricing" (dynamic pricing)**: rules that change prices automatically by date, inventory threshold or **sales velocity**, paired with the waitlist to read demand. Rule-based, not ML [seen]. The Universe Pro tier also lists dynamic pricing.
- **Access codes** (presales for loyalty members, partners and sponsors) and **promo codes** (targeted discounts that leave public prices untouched) [seen].
- **The waitlist as a pricing and capacity tool**: it can open when inventory is low or sold out, for held-back inventory, or "to control access from the very start of an onsale" (a pre-authorised lottery-style on-sale). New inventory **fills automatically from the queue**. Real cases: NPU Live used waitlist data to win a permit for more capacity, and 16% of tickets came via the waitlist. On BYON Combat's sale, 7,000+ fans joined for a much smaller capacity and more than 75% of purchases came via the waitlist. Waitlist use at festivals roughly doubled from 2023 to 2025 [seen].
- **"Tixr Boost"**: **not found** anywhere on Tixr's public site, sitemap or search results. It may be a retired name, a sales-deck term or a confusion with Rewards or Behavioral Pricing. Treat it as unverified.

**Add-ons and upsells**: conditional add-ons in the checkout based on the cart (including offers not shown publicly); post-purchase add-ons in the fan's Tixr wallet (parking, merchandise, experiences); self-serve **upgrades** to better seats or access levels, with eligibility rules and windows; hotel and travel packages [seen].

**Marketing**
- **Tixr Rewards** (gamified referrals): the organiser sets which ticket types earn points, how many points each tier needs, the rewards and who fulfils them (**Tixr fulfils cash back**; the organiser fulfils perks), with an optional leaderboard and automatic tracking. Platform figure for 2025: **$12 in referred sales per $1 of rewards**. LIB 2026 made about $30 per $1. At Riot Fest 2024, 1,800 fans drove about 2,800 tickets, 6% of net ticket revenue, across 5 tiers. At music festivals a referred order averages about $500 net [seen].
- **Promoter System**: partners, affiliates and sub-promoters get their own logins, landing pages and tracked links and see their own sales, while the organiser sees everything [seen].
- **Conversion tracking**: pixels for Google Ads, Meta, TikTok, Snapchat and more; **Meta CAPI natively** (server-side); other server-side tracking via Segment and CDPs; a GA4 integration; funnel tracking [seen].
- **Syndication**: listings pushed automatically to Spotify, Songkick, Bandsintown and JamBase, updating themselves [seen].
- Follow and forms (capture interest and checkout questions). Cart-abandonment retargeting through Hive, Cymbal and Tradable Bits [seen].

**Data and reporting**
- **90+ data points per buyer**, the organiser owns them, ungated and exportable. "Tixr never sells or shares fan data." Its pitch is against "legacy platforms [that] cannibalize your data to promote your competitors" [seen].
- Zero-lag reporting (live transactions, inventory, "what's in escrow"). Marketing intelligence that links campaigns to sales spikes. **Geographic cluster and penetration maps**. Audience segments (top spenders, loyalty) [seen].

**Payouts**: Stripe processing. **Connect your own Stripe account** for direct settlement, or use Tixr processing for managed payouts [seen].

**APIs and integrations**: open APIs and webhooks (event listings, purchase data, CRM). Native connectors to **Hive, Cymbal, Mailchimp, Laylo, Tradable Bits, Segment** (on to Salesforce, Klaviyo and others), **AnyTrack, Prism** (booking), Vimeo (livestream) and Fortress (access control), plus RFID partners. Integrations cost nothing extra on the Tixr side [seen].

**White-label**: white-label event pages, branded seat maps, custom mobile apps. Organisers control how fees are presented [seen].

### On-sale / queue / anti-bot
- **Deliberately no traditional waiting room**: "Because the system manages traffic dynamically rather than funnelling fans through a traditional waiting room, it avoids the bottlenecks…". Demand is shaped through tier progression, the waitlist and access codes instead. Tixr claims it "processes thousands of transactions per second" [seen].
- Its layered anti-fraud [seen]:
  - real-time bot monitoring (repeat logins, fast refreshes, unusual IP behaviour), with stricter checks that switch on automatically during demand spikes;
  - order and user purchase limits (recommended 2 for big underplays, 4 for clubs, 6 for festivals);
  - a **CAPTCHA toggle per event**;
  - waitlists with card pre-authorisation as a deterrent;
  - **post-on-sale fraud scrubs** before tickets are delivered;
  - **delayed delivery**, **rotating QR codes** (refresh continuously, cannot be screenshotted) and **NFC tickets** in Apple and Google Wallet;
  - temporarily non-transferable tickets (Stateside);
  - verified transfers and face-value-only resale.
- tixr.com itself sits behind **DataDome** [seen].

### Payments, fees, instalments, split pay
- **Organiser fees are not published.** They are "flexible and based on event type, volume, and service tier", and the organiser controls how fees are shown to fans [seen]. Third-party estimates (unverified) put them around 13.5% + $1.50 per ticket [source]. Fan reviews cite "10% per ticket" and £8 booking fees [source: Trustpilot].
- **Payment plans (native, "automated layaway")**: a deposit, then scheduled instalments. "No third parties, no credit checks, no interest, <0.01% breakage" [seen]. The fan pays a **one-time fee per order** and can pay off early. The organiser sets the first payment timing and the number of instalments [seen]. Riot Fest 2026: 20% down plus a **$9.99** non-refundable fee, the balance split evenly to a fixed date, failed payments retried at +5 and +10 days, no refund of previous payments on cancellation [source]. Plans also cover season renewals and invoices [seen].
- **Split pay between friends / group buying: not found.** "Group sales" in Tixr means organiser-side seat blocks with invoices for corporate or school buyers, not fans splitting a cart. Fan support describes plans as "split your order into 2 equal payments" [source]. The closest social mechanics are Rewards (referral cash back) and verified transfers.
- Payment methods: cards, Apple Pay and Google Pay; at the door also cash, BOCA printing and Stripe Terminal [seen].

### Post-purchase
- **Fan Transfers**: the organiser picks which ticket types are transferable. The fan sends to an email address and the recipient has 7 days to accept; the old ticket is then voided and a new one issued, so the organiser captures the new attendee's data [seen].
- **Tixr Resale** (official, in-platform): for GA, resale listings appear only once a ticket type is sold out, replacing "Sold out" in the picker. For reserved seating they show on the seat map. Rules for eligibility by type, section or price; a **minimum, a maximum or face value only**. The original ticket is voided and reissued. **Sellers are paid after the event.** Tixr handles payments, payouts, disputes, support and tax. NPU Live: more than 75% of resale volume stayed on Tixr. Stateside: 10% of tickets went through face-value resale [seen].
- **Upgrades, post-purchase add-ons, returns to credit, order protection claims** [seen].

### On-site (Door)
- An iOS app that does scanning and selling. A **box office on a laptop or iPad** uses the same checkout as fans (agent checkout) with no shortcodes. It can look up and reissue tickets, and handle exchanges, upgrades and transfers [seen].
- **Scanning**: NFC tap-to-enter (needs NFC sleds: Linea Pro, Socket Mobile, JANAM XT4), rotating QR, static QR and PDF. Group check-in of a whole order, order search, **check-in by scanning government ID**, **offline scanning on iOS** (queued and synced later; Android needs a connection), real-time sync across gates, and timed-entry rules enforced at scan [seen].
- Hardware is supplied by Tixr: iPhones and iPads, Stripe card readers, Infinite Peripherals printers, BOCA printers and Stripe Terminal countertop readers [seen].
- **Integrates with Fortress access control and RFID partners**. It does not run its own cashless system; food and drink is order-ahead and pickup [seen].

### AI / data features
- No named AI product. There is an opinion blog post on AI and roles in live events [seen]. The "intelligence" is rule-based demand tools (waitlist signals, behavioural pricing, velocity) plus 90+ data points, geo maps and CDP pipes [seen].

### Observed tech and APIs [seen]
- tixr.com is protected by **DataDome**. The marketing site runs on **Webflow**, the knowledge base on **Intercom**. Payments run on **Stripe** (Connect, Terminal). Protection comes from **Teak**. Integrations: Segment, Meta CAPI and GA4.

### Distinctive
1. **The waitlist with card pre-authorisation** is used as demand measurement, an anti-bot filter, automatic fulfilment and even the whole on-sale mechanism. **This is the single most copyable idea for an EPS festival platform.**
2. **Native payment plans run by the platform** (not a BNPL partner), with a flat per-order fee.
3. **Tixr Rewards**: referrals with cash back, tiers and a leaderboard, at a published 12:1 ROI.
4. **Unified commerce** in one cart: hotel, camping on a map, parking, merchandise, food and drink, multipasses. Built for destination festivals.
5. **Controlled secondary**: official resale with price floors and caps, shown only after sell-out, sellers paid after the event; plus verified transfers.
6. **Memberships** with a numbered member card and credit banks.
7. Its stance on data ownership: no marketplace that pushes competing events.

### Weaknesses / criticism
- **Fees are opaque** (demo only) and fans see high fees (about 10% per ticket) [source].
- Fan complaints about checkout errors and refund disputes on cancelled or rescheduled events; Tixr answers that the organiser decides [source]. Support responsiveness is praised, though [source].
- **Not self-serve for most**: enterprise sales and onboarding, and the self-serve tier is downplayed.
- **The Door app is mainly iOS** (Android only on JANAM handhelds and online-only). No native cashless or RFID; it relies on partners.
- **No discovery marketplace of note**; demand is the organiser's own plus syndication.
- No public developer portal was found (APIs are "available", but the docs are not public).
- The bot site blocks automated access outright (good for anti-bot, bad for transparency). Its EU presence is young (London since 2024) and EU payment methods or local acquiring are not evident.

### Sources
- Crawled: creators.tixr.com (/, /products, /products/{tixr, studio, door, platform, integrations, reserved-seating, memberships}, /tixrapp) and posts: how-tixr-resale-works, tixr-rewards-the-built-in-referral-program, tixr-waitlist-how-organizers-turn-demand, 9-5-tips-high-demand-onsale, high-demand-onsales-6-ways, how-to-protect-your-event-from-ticket-scalpers, order-protection, fan-transfers, tixr-memberships-guide, sell-more-without-slashing-prices, think-bigger-ai, rolling-stone-stateside, integrating-travel, lightning-in-a-bottle, tixr-arena, london-office, zamna, good-vibez-membership.
- [Riot Fest payment plan (2026)](https://riotfest.org/2026/06/15/the-riot-fest-payment-plan/), [Riot Fest payment plans](https://riotfest.org/payment-plans/)
- [Trustpilot Tixr](https://www.trustpilot.com/review/tixr.com?page=2), [G2 Tixr](https://www.g2.com/products/tixr/reviews)
- [Pollstar: VENU / Tixr](https://news.pollstar.com/2025/09/04/venu-tixr-ink-4-venue-deal/), [CB Insights](https://www.cbinsights.com/company/tixr)
- [CheckThat Tixr fee estimate](https://checkthat.ai/brands/tixr-creators), [TickPick vs Tixr](https://www.tickpick.com/blog/organizer/tickpick-vs-tixr) (competitor-written)

---

## 3. Universe (universe.com, part of Ticketmaster / Live Nation)

### Overview
- Universe was founded in Toronto and acquired by Ticketmaster in 2015 [source]. It is Ticketmaster's **self-serve platform for general admission and timed entry**: clubs, promoters, attractions, tours, food and drink, and mid-size festivals (Lollapalooza Berlin, Boogietown and Whiskey Live Dublin are featured) [seen].
- Its pitch is **"your event, now on Ticketmaster"**: self-serve tools plus Ticketmaster's distribution (100M+ monthly web visitors, 14M app users, 800+ affiliate partners, 70+ OTAs, the Discovery API) and Ticketmaster's fan database [seen].
- Explore lists about 10,300 events, mostly in the UK, Canada, Germany, the Nordics, the Netherlands, Spain, Australia and New Zealand [seen]. The UI is in 13 languages: EN, CS, DA, DE, FR, ES, ES-MX, NL, NL-BE, NB, PL, FI and SV. Romanian is not one of them [seen].

### What was crawled vs blocked
- **Crawled [seen]:** about 25 pages. These were the home page, features, features-promote, features-convert, features-scan, pricing (and its FAQ HTML), explore, discover, clubs, attractions, partner-with-us, two event pages (a Montreal comedy show and Saoko Sauna in Helsinki) with **the ticket picker opened** (the `widgets.universe.com` iframe), developers.universe.com (intro, GraphQL, reference, changelog) and the sitemaps.
- **Blocked:** `support.universe.com` (Zendesk) serves a **Cloudflare "Just a moment…" challenge**, so it was skipped and its content comes only from search snippets. `/create` redirects to account registration (no account made). `/embed/`, `/host/`, `/dashboard/` and `/graphiql` are disallowed in robots.txt and were not fetched.

### Fan features
- **Event page** [seen]: host name, date, place, price range ("€7.90 – €150.00"), "GET TICKETS", share, report event, age limit, map and directions, "About the host" with **business seller information** (company, phone, email: the EU DSA trader disclosure), "Contact Host" and interests.
- **Ticket picker** [seen, Saoko Sauna]: a three-step flow ("Select Tickets → Ticket Info → Payment Info"). Each ticket type shows the all-in price with a breakdown ("€7.90 = €6.93 + €0.97 fees, including taxes"), a **"Best Deal" label**, descriptions and "More Info", and a **table package for 8 guests**. Below that sit **"Access Key or Discount Code"**, a language switcher and Stripe frames.
- **Timed entry** [seen]: a calendar of available dates (`monthInventory`, `timeSlotsForDate`), filtered by group size. Buyers can **change their own date and time** under the organiser's rules [seen].
- Urgency indicators, ticket labels that nudge buyers to higher tiers, a waitlist on sell-out, artist play buttons (embedded music) on event pages, and "Pay What You Want" tickets [seen].
- 135 currencies, 9+ languages, cards, Apple Pay, Google Pay and **Alipay** and other local methods [seen]. QR tickets in the wallet, the Universe app, reminders [seen].
- Payment plans for buyers (see below) [source].

### Organiser features (detailed)
**Onboarding and creation**: self-serve sign-up, "create an event listing in minutes, go live, and start selling tickets the same day". Event Manager (edit, enable or disable tickets, change prices, duplicate listings). Account teams for larger clients. Bulk-edit timed-entry schedules and extend long-running schedules. Inventory and price per time slot [seen].

**Ticket types and pricing**: unlimited ticket types and add-ons (Starter); **tiered ticketing**, **cardholder presales** (for example Amex-style card-bin presales), **payment plans**, team permissions (Standard); **dynamic pricing**, RFID, sponsorship network and partner affiliate channels (Pro) [seen]. Access keys and discount codes in the picker [seen]. Pay What You Want [seen].

**Checkout optimisation**: custom checkout questions; add-ons, merchandise and package upgrades in a single transaction; checkout filters; preset ticket labels ("Best Deal"); inventory-based urgency indicators; re-engagement emails for drop-offs (cart abandonment); redirects to secondary selling points after checkout [seen].

**Marketing and distribution**
- **Listing on Ticketmaster** (genre pages, artist and venue pages, TM Local, TM New Music), the **Discovery API and partner data feed** to hundreds of affiliate sites, **Channel Partners** (70+ OTAs with synced inventory and central pricing and distribution), and **"buyer records"** from Ticketmaster's database for affinity audiences, with the client marketing team running campaigns [seen].
- Integrations: Google Analytics, Facebook Pixel, Tradable Bits, Audience Republic and Mailchimp, plus **Zapier (5,000+ apps)** [seen].
- No built-in referral or ambassador programme, and no in-house email or SMS campaign tool was seen [seen, absence].

**Payouts**: Universe Payments (managed; processing included in the fee) or **Stripe Connect** (the organiser's own account, "advanced payouts", with the processing fee charged by Stripe). The organiser can pass fees on, add its own extra fee ($ or %) as a line item, and add tax as a line item; Universe does not remit tax [seen].

**Reporting**: real-time analytics; custom **scheduled recurring reports**; check-in times and volume to place staff; revenue by ticket type; per-attendee answers; CSV and Excel exports [seen].

**APIs and integrations**: **developers.universe.com**, with OAuth 2 (auth code and client credentials) and a **GraphQL API (beta)** plus REST: events, orders, guest lists with answers, attendee QR codes, and the `embed2.js` widget. The changelog has not changed in about 9 years and the docs were "updated over 1 year ago", so it is stagnant [seen]. **Embeddable widgets** ("a single snippet of code", no developer needed) and an API for a fully custom native purchase flow [seen].

**White-label**: a "brand-first" white-label, mobile-first checkout on the organiser's own site; customer data owned by the organiser [seen].

### On-sale / queue / anti-bot
- **A Universe Queue exists**: the help article "How a Universe Queue Works" (support.universe.com/hc/en-us/articles/31264144315021) [source; we could not read it behind Cloudflare]. On large on-sales it can lean on Ticketmaster know-how; the "pre-sale strategy / on-sale strategy" services are listed as account-team offerings [seen].
- Cardholder presales and access keys act as gates [seen]. No public detail was found on Verified Fan or Smart Queue being available inside Universe [gap].
- Support pages sit behind Cloudflare; Ketch (Live Nation) consent [seen].

### Payments, fees, instalments, split pay
- **Published, capped fees** [seen, /pricing]:
  - **Starter**: 2% + $0.79 per ticket, **capped at $19.95 a ticket**;
  - **Standard**: 2.5% + $1.99, capped at $19.95;
  - **Pro**: custom.
  - Processing and VAT come on top. Free events cost nothing, with no setup or termination fees. Outside the US the service and processing fees are shown as **one all-in price**. Nonprofit rates are available. Tiers can be changed at any time.
- **Payment plans** (Standard tier) [source: Universe help snippets]:
  - the event must be **at least 30 days away**;
  - the buyer pays a non-refundable plan fee per currency (**$15** USD and CAD; €0 / £0 for EUR and GBP);
  - with Universe Payments the service fee is spread across the instalments, while with Stripe Connect the fees are taken on the first payment;
  - a missed instalment has 10 days' grace, after which the order is **cancelled and refunded minus the plan fee**;
  - the processor cannot be switched once a plan is on.
- No split pay between friends was found.

### Post-purchase
- Organiser-side: resend or transfer tickets, upgrade, full or partial refund, automatic reminders [seen]. Buyers can change date and time themselves (timed entry) [seen]. Fan resale: not native in Universe; Ticketmaster's resale is not shown for Universe events [gap/absence]. Review complaints mention that "exchanges were not possible" [source].

### On-site
- **The BoxOffice app** (free): door sales by card, Apple Pay, Google Pay or **cash**; refunds, resends and receipts; door-only ticket types and prices; sales across several events; scanning with **re-entry** (multiple scans); search by name or email; **devices locked to a ticket type** (separate lanes); "join team" volunteer onboarding with custom permissions; real-time sync; **BOCA printing** [seen].
- **RFID access control** is offered at the Pro tier, as are on-site support and contact-centre support [seen].

### AI / data features
- No AI features were seen. Its data asset is **Ticketmaster's fan database** ("buyer records"), used by the client marketing teams [seen].

### Observed tech and APIs [seen]
- `www.universe.com/graphql` (POST, used by the site itself), `flags.universe.com` (feature flags), `telemetry.universe.com/collect`.
- The checkout is a **separate Next.js app at `widgets.universe.com/{listingId}`**, with `/api/isListingSoldOut`, `/api/monthInventory`, `/api/timeSlotsForDate`, `/api/orderItemTypes` and `/monitoring`. Stripe.js runs in the checkout.
- **Server-side tagging on `tagging.moshtix.com`** (Moshtix is a Ticketmaster or Live Nation property in Australia; the tagging stack is shared). Ketch consent under the `live_nation` tenant, Contentsquare, Google Maps.

### Distinctive
1. **Self-serve with a Ticketmaster pipe**: list on Ticketmaster.com and its app, the Discovery API, 800+ affiliates and 70+ OTAs, all from a self-serve tool.
2. **Transparent, capped fees** (2% + $0.79, capped at $19.95), cheaper than Eventbrite for small organisers.
3. **Strong timed entry and attractions tooling** (bulk slot edits, self-service rebooking, group-size filters).
4. Local payment methods (Alipay), 135 currencies, an EU DSA-style seller disclosure on event pages.
5. Its tier ladder (Starter, Standard, Pro) upgrades organisers into Ticketmaster services (RFID, dynamic pricing, sponsorship).

### Weaknesses / criticism
- Product momentum looks low: a dated developer portal (a changelog over 9 years old, GraphQL "beta"), few visible new features, and support behind Cloudflare. Reviews are small in number and mixed. Trustpilot notes the company "hasn't replied to negative reviews" [source].
- No native fan resale or referral programme, and no email or SMS tool of its own. Advanced features (dynamic pricing, RFID, affiliates) are locked behind Pro and sales.
- **It is Ticketmaster**: some independent organisers and fans distrust Live Nation (antitrust cases, fees). Organisers' data and audiences end up in Ticketmaster's ecosystem. No Romanian locale.
- Payment plans are fairly rigid (30-day minimum, a $15 fee in USD, cancellation on a missed instalment).

### Sources
- Crawled: universe.com (/, /features, /features-promote, /features-convert, /features-scan, /pricing, /explore, /discover, /clubs, /attractions, /partner-with-us, /events/…-LSXF60, /events/saoko-sauna…-TR1LZG with the picker), widgets.universe.com, developers.universe.com (/docs, /reference, /docs/graphql, /changelog) and the sitemaps.
- [Universe help: payment plan set-up](https://support.universe.com/hc/en-us/articles/360002388191-Set-up-payment-plans-for-your-event), [buyer payment plan FAQ](https://support.universe.com/hc/en-us/articles/360003054991-Buying-a-ticket-on-a-payment-plan-FAQ), [How a Universe Queue Works](https://support.universe.com/hc/en-us/articles/31264144315021-How-a-Universe-Queue-Works) (titles and snippets only; Cloudflare-blocked)
- [Trustpilot Universe](https://ca.trustpilot.com/review/universe.com), [G2 Universe](https://www.g2.com/products/universe/reviews), [Capterra Universe](https://www.capterra.com/p/132555/Universe/reviews/)

---

## 4. Cross-platform takeaways for an open, multi-organiser EPS

| Capability | Eventbrite | Tixr | Universe | Priority for EPS |
|---|---|---|---|---|
| Self-serve sign-up and event creation | Yes, AI-assisted | Demo-led (a self-serve tier exists) | Yes | **Must**: open to any organiser |
| Published, transparent fees | Yes (3.7% + $1.79 + 2.9%) | No | Yes, capped (2% + $0.79, max $19.95) | **Must**, plus the option to absorb or pass on fees and all-in display in the EU |
| Tier progression / rule-based dynamic pricing | Yes (Aug 2026, 10 rules) | Yes (date, inventory, velocity) | Pro only | **Must** (date, quantity sold, velocity) |
| Waitlist with card pre-authorisation and auto-fulfil | Basic waitlist | **Yes, a core feature** | Basic | **High**: anti-bot, demand signal and capacity planning |
| Native payment plans | PayPal BNPL only | **Native layaway, flat fee** | Native, $15 fee, 30-day minimum | **High** for festivals (see Riot Fest: 20% down + $9.99) |
| Referral rewards (cash back, tiers, leaderboard) | No (tracking links) | **Tixr Rewards, 12:1 ROI** | No | **High**: cheap growth channel |
| Promoter and affiliate logins with tracked links | Tracking links | Promoter System | Pro, partner channels | High |
| Unified cart (camping, parking, hotel, merchandise, food and drink) | Add-ons only | **Full, including campsite maps** | Add-ons, packages | **Must** for NIBIRU's resort and festival offer |
| Official resale with caps, verified transfer | No (Tixel partner) | **Yes, after sell-out, seller paid after the event** | No | **High** (EU price-cap laws are coming) |
| Delayed or rotating QR, NFC wallet | Delayed QR (Oct 2026) | Rotating QR, NFC, delayed delivery | Static QR | High |
| Waiting room / queue | Yes (own queue_rpc, live position) | No queue by design | Yes (Universe Queue) | Medium: needed at peak, but pair it with waitlist and lottery |
| Pixels plus server-side CAPI | Pixels, Seeka | Pixels, **native Meta CAPI**, GA4, Segment | GA and FB pixel | High |
| Email and SMS built in | **Yes** (Pro for volume, SMS paid) | No (Hive, Cymbal, Laylo) | No (Mailchimp, Zapier) | Medium: integrations first, native later |
| Discovery marketplace | **Yes, a core asset** | No (syndication) | Ticketmaster listing | Medium: a multi-organiser EPS could become a Romanian or CEE marketplace |
| Open API and webhooks | REST v3, webhooks | APIs and webhooks (docs private) | GraphQL and REST, stale | **Must** (public docs, webhooks, OAuth) |
| Embeddable or white-label checkout | Widget, not stylable | **White-label pages, custom apps** | Widget, plus API for a native flow | **Must** for organiser branding |
| Instant or pre-event payouts, own Stripe | Instant and scheduled | Own Stripe or managed | Stripe Connect or managed | High (escrow rules for festivals) |
| On-site app: offline, box office, door POS | Organizer app, Tap to Pay | Door (iOS, offline, ID lookup, NFC) | BoxOffice (re-entry, lane lock) | **Must**, offline first |
| RFID and cashless | No | Partners (Fortress, RFID) | Pro (RFID) | **Must** for NIBIRU: EPS can differentiate here |
| Memberships and season passes | Basic | **Memberships, numbered cards, credit banks** | No | Medium-High (resort season passes) |
| Order protection (insurance) | No | Teak, at checkout | No | Medium (revenue share) |
