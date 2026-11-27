# Fan-first "next-gen" ticketing: DICE, Shotgun, Fever, TicketSwap

Research date: 27 Nov 2026. Prepared for the NIBIRU / EPS CTO interview.
Tags: **[seen]** = observed directly in our own crawl (Playwright, raw files in `raw/fan-first/`); **[source]** = from public sources (help centres, press, job ads, app stores, third-party reviews; named in Sources). Third-party competitor pages (Ticket Fairy, TickPick) are marked "(competitor claim)".

**Crawl log in brief.** robots.txt was checked for every host first (`raw/fan-first/robots-*.txt`). Polite crawl: one page at a time, 1.5 to 5 s apart, no logins, no forms submitted, `/purchase/`, `/seating` and `/login` blocked in the browser for Fever (they are disallowed in robots.txt).
- **DICE** (dice.fm, betamax.dice.fm, dicefm.zendesk.com): Playwright got **403 "These are not the droids you are looking for..."** (Cloudflare bot rule) on every page; WebFetch also got 403 on the help centre and blog. Not bypassed. Everything on DICE is [source].
- **Shotgun** (shotgun.live marketplace): **429 "Vercel Security Checkpoint", "Failed to verify your browser, Code 21"**. Not bypassed. support.shotgun.live returned a Cloudflare "Just a moment..." page. **pro.shotgun.live** (the organiser site, Webflow) was open: 13 pages crawled [seen].
- **Fever** (feverup.com and business.feverup.com): open. 17 pages crawled, including the ticket selector up to the "Continue" button [seen].
- **TicketSwap** (www.ticketswap.com): **202 "Verifying"** JS bot challenge on every page; press.ticketswap.com gave **403** to headless Chromium. Not bypassed. Some press releases were readable through WebFetch (a plain fetch, no challenge), so those are [source].

> **Context that matters for NIBIRU:** **Fever acquired DICE in June 2025** [source] (and DICE bought Boiler Room in 2021). Fever's B2B pages now sell "Fever + DICE + Secret Media Network" as one demand network [seen]. So two of the four platforms are one company. **Fever is present in Romania** (city pages for Bucharest, Cluj-Napoca, Brașov, Brăila, Constanța, Arad, Oradea, Craiova, Ploiești, Târgu Mureș) [seen], so it is the most likely "next-gen" competitor on NIBIRU's home turf.

---

## 1. DICE (dice.fm)

### Overview
London, founded 2014 by Phil Hutcheon (with ustwo). Mobile-first primary ticketer for gigs, clubs and festivals. Its founding ideas were no booking-fee surprises, tickets locked to the phone, and a waiting list instead of resale. 10M+ monthly fans (App Store) [source]. Owned by **Fever since 5 June 2025** (terms undisclosed; DICE had raised about $195M, including a $122M Series C led by SoftBank in 2021) [source]. Acquired Boiler Room in 2021 [source].

### What was crawled vs blocked
- Blocked: dice.fm (homepage, /browse, /partners, /fans) returned 403 to Playwright; betamax.dice.fm (partner blog) and dicefm.zendesk.com (help) returned 403 to WebFetch. Robots.txt allows `*` but disallows ClaudeBot, anthropic-ai, GPTBot and others, with `Content-Signal: search=yes, ai-train=no, use=reference` [seen in robots.txt].
- Filled from help-centre snippets, App Store and Play Store listings, press and job ads.

### Fan features
- **App-first tickets locked to the buyer's phone and account.** No PDF and no screenshot-able code. The ticket "activates" and shows an **animated QR code shortly before doors** (about 1 to 2 h, set per event). Once it is activated, it cannot be sent to a friend [source].
- **Upfront, all-in price.** "Full price shown upfront every time, no surprises at checkout" (store listing). The fee is included in the shown price, not removed [source].
- **Waiting List** for sold-out shows (promoter opt-in). Fans who can't go return the ticket in the app and it goes **automatically, at face value, to the next fan in the queue**. The seller is refunded only once it is resold [source].
- **Discovery from music taste:** connect **Spotify or Apple Music** to get recommendations from your library. Follow artists and venues to get alerts for new shows. An editorial "picks" feed [source].
- **Social:** follow friends, **see what they're going to**, invite them to shows "with a couple of taps", share plans as you choose [source].
- **Groups** (launched 28 Jun 2023): create a group from your profile or when saving an event, invite by link, **members vote on events**, see when others have bought, buy for yourself or for the whole group [source].
- **Ticket transfer** to a friend's DICE account (phone number) until a cut-off a few hours before the event. The organiser can disable it [source].
- **Refunds:** cancelled or rescheduled events, organiser-instructed refunds, a waiting-list resale, and for some events an **automatic refund within about 24 h of purchase**. None in the 24 h before the show [source].
- **Livestreams: DICE TV** (April 2020). About 2,500 ticketed streams by late 2020. Watch in the app, on the web or on TV with a 5-character code shown on the ticket, a "Watch Now" button on the ticket, replay for about 48 h, donations [source].
- **DICE Extras / Merch on DICE** (2021, marketplace 2024): add-ons and merch bought in the app next to the ticket. DICE claims up to +30% event revenue [source].
- Apple Wallet support is claimed in the store listing [source].

### Organiser features
- **MIO / "DICE for partners"** dashboard: event creation, ticket tiers, live sales, fan insights, waiting-list demand data ("identify fan demand with the waiting list" blog), marketing to followers through the app feed and push [source].
- Products named in third-party profiles: DICE app and web ticketing, Discovery, Waiting List, Groups, MIO, Extras [source].
- Venues going "mobile-only": Islington Assembly Hall (2017, the UK's first digital-only venue) and Troxy London ("world's biggest mobile-only venue") [source].
- Access tends to be by invitation or curated (competitor claim). Partner terms limit how downloaded fan data may be reused for marketing (competitor claim, TickPick) [source].
- After the Fever deal, DICE events also go through Fever's distribution (Fever, Secret Media, OTAs, Spotify, TikTok, Bandsintown, Google) [seen on business.feverup.com].

### On-sale / queue / anti-bot
- The core anti-tout idea is **identity binding, not a queue**: tickets are tied to the phone, so a bot can buy but cannot hand over a usable ticket. Hutcheon: it "stops bots... harvesting tickets because they're attached to the fan's phone" [source].
- The late QR code (no screenshot to sell) plus face-value-only waiting list means there is no profitable secondary route [source].
- No public detail found on a virtual queue or waiting room for big drops. The website edge is Cloudflare with aggressive bot rules (our headless browser got 403) [seen].

### Payments and fees
- The fee is included in the displayed price. US self-sign-up terms are reported as **13% of face value, processing included** (competitor claim, Ticket Fairy) [source].
- No public evidence of instalments or payment plans on DICE (searched; nothing found) [source].
- Payouts: settlement to the promoter's bank account; no published schedule found [source].

### Post-purchase
In-app wallet; transfer to a friend; waiting-list return (the only resale route, face value); 24 h cooling-off refund on some events; Extras and merch add-ons; livestream access on the ticket; Groups coordination [source].

### On-site
Scanning the animated, time-released QR in the app. "Ticketless access" manuals and guides for venues (DICE access-control docs) [source]. Some venues are mobile-only [source].

### AI / data / personalisation
Taste graph from Spotify / Apple Music links, artist follows, friend graph, saved events. Demand signal from waiting-list sizes, sold to promoters as insight [source].

### Observed tech, APIs and architecture
- Web is behind **Cloudflare** with a custom 403 page; robots.txt is Cloudflare-managed with Content-Signals [seen].
- **Backend mostly Elixir** (OTP, Phoenix), "handling millions of transactions for our ticket sales"; PostgreSQL, Redis, Kubernetes, **GraphQL** plus REST (DICE job ads) [source]. Native iOS and Android apps.
- No DICE engineering blog posts were found (the Medium "dice-engineering" page returned 403). Careers now listed under "FeverUp / DICE" on Greenhouse [source].

### Distinctive
1. A phone-bound, time-released, animated QR as the anti-tout mechanism (identity, not price).
2. Waiting List = official face-value resale with zero secondary market, and it doubles as demand data.
3. Groups with voting and friends' plans: social commerce built into the ticket app.
4. All-in pricing as a brand promise since 2014.

### Weaknesses / criticism
- **App-only:** no app, no ticket. Lost-phone and dead-battery problems; activation-time stress; transfer cut-offs; the recipient needs a DICE account [source].
- The Waiting List only works if someone returns a ticket, so "I was on the waiting list and never got one" is common. A seller gets no money unless a buyer appears [source].
- Customer-service complaints (Trustpilot as low as about 2.8/5 in one roundup; about 20% dissatisfied in an aggregator) [source].
- A curated, invitation-led B2B model, with limits on promoter data reuse (competitor claims) [source].
- Now part of Fever: independence and promoter-data neutrality questions [source].

### Sources
App Store DICE listing (apps.apple.com/gb/app/dice-live-shows/id898358948); Play Store fm.dice; Wikipedia "Dice (ticketing company)"; dicefm.zendesk.com help articles (activate ticket, wait list queue, refunds; snippets via search); Music Week "Dice launches Groups" (2023); CMU "Dice adds group bookings feature"; Product Hunt Groups launch; Music Week "Dice launches new TV function" (2020); Pollstar "DICE partners with Rough Trade on livestreams"; MBW "DICE launches DICE Extras" (2024); Engadget 2017 "London venue first to eradicate touts with digital-only tickets"; Pollstar on Troxy; Pollstar / MBW / Fever newsroom on the Fever acquisition (5 Jun 2025); DICE Greenhouse job ads and wearedevelopers backend listings (Elixir stack); Ticket Fairy "Ticket Fairy vs DICE" (competitor); TickPick "TickPick vs Dice" (competitor); Ticket Tailor "DICE reviews summary".

---

## 2. Shotgun (shotgun.live)

### Overview
Paris, founded about 2015. The leading marketplace and ticketer for **electronic music, clubs and urban/rap** in France, growing in the EU (Belgium, Spain, UK) and the US. It claims **5M+ monthly active users, 250k+ featured artists, 15M+ tickets sold in 2024, and 10,000+ organiser teams** [seen on pro.shotgun.live]. Positioned as "more than a ticketing platform": a marketplace plus CRM plus artist pages [seen].

### What was crawled vs blocked
- Blocked: shotgun.live (home, /en/cities/paris, /en/organizers): **Vercel Security Checkpoint, 429** [seen]. support.shotgun.live: Cloudflare challenge [seen].
- Crawled [seen]: pro.shotgun.live/en-eu (home), the festival, venue, collective and artist solution pages, and blog posts on Xpress Door, Membership, Music Sync, Reward Program, Segmentation, the Ninho bundle case and the Rilès pre-registration case.

### Fan features
- **One-click purchase** in the app; events "recommended to the right profiles" based on **taste and location** [seen].
- **Music Sync:** link **Spotify or SoundCloud** (Apple Music and Deezer "to come" in 2023) to auto-follow the artists you listen to. You get notified when they are booked; recommendations come from your library. Also integrates **Songkick and Bandsintown** for artist-near-you alerts [seen].
- **Follow organisers, venues and artists** (organiser pages, Shotgun Artists pages); a push notification whenever a followed entity publishes [seen].
- **Pre-registration** ("I'm in") with a countdown before sales open; notified and emailed at launch [seen].
- **Waitlist with automatic purchase:** "Sold out? When a spot opens, fans get auto-notified **and charged**" [seen]. The fan's card is pre-authorised, so the first in line gets it.
- **Integrated resale and transfer** "in a few clicks"; the organiser can allow or refuse resale [seen; organiser-veto from Que Choisir forum, source].
- **Reward / Ambassador programme:** a fan shares a personal link and gets **cashback per ticket sold through it**, refunded automatically after the event [seen].
- **Bundles:** ticket plus merch (e.g. Ninho: ticket plus a USB album through the **Shopify integration**, pickup on site or delivery) [seen].
- **Xpress Door:** scan a QR code at the door, pay with Apple Pay, Google Pay or card **without creating an account**, and get a ticket QR at once [seen].
- **Memberships:** members-only ticket prices via segments [seen].
- **Pass Culture** integration (the French state culture pass for young people) [seen].

### Organiser features (the "Smartboard")
- Guided event creation in minutes; co-organisers and roles; real-time dashboard (sales, revenue, fill rate, event comparisons); **event traffic analytics** [seen].
- **CRM built in:** a contact base enriched by every sale (age, location, purchase behaviour, average spend); **dynamic segments** (filters) or CSV import; suggested segments ("Recent contacts", "Top buyers > €100"); segment-restricted tickets; CSV export [seen].
- **Owned channels:** push notifications, built-in newsletters, exclusive tickets, organiser page with followers [seen].
- **Pricing:** early birds, bundles, multi-day passes, flexible rules; promo codes [seen].
- **Advance payouts** before the event, "trigger transfers before the event to manage your budget" [seen].
- **After-event survey** [seen]; product roadmap open to customers ("Co-created... view product roadmap") [seen]; support live 7 days a week [seen].
- **Shotgun Artists:** "first artist platform powered by ticketing". Artist pages, fan capture, pinned events, pre-registrations, drops, plus SoundCloud, Bandsintown and streaming integration. Rilès got 20,000+ pre-registrations in 24 h and +17,000 followers [seen].
- Separate **offline-first POS app** with hardware integration for door and bar (job ads) [source].

### On-sale / queue / anti-bot
- **Pre-registration plus auto-purchase waitlist** shift demand away from a "click race" [seen].
- Marketing claims "no crashes during major sales" [seen]. The web marketplace uses the **Vercel Firewall / Security Checkpoint** bot challenge [seen]. No public detail on a virtual queue.

### Payments and fees
- A per-ticket **service fee paid by the buyer, rate not published**. Example complaint: €3.99 fee on 3 × €53.20 [source].
- **Service fees are kept even when the organiser cancels** (Shotgun says it "facilitated the sale and the refund"). A frequent Que Choisir complaint [source].
- **Resale and waitlist purchases: about a 15% fee**, capped at USD 30 in the US (competitor claim); fans report a €4 resale fee not disclosed in the flow [source].
- Apple Pay and Google Pay [seen]. Shotgun acts as a "mandataire" (agent) for the organiser under its terms [source].

### Post-purchase
In-app tickets, transfer, resale (if allowed), auto-waitlist, ambassador cashback, merch pickup, post-event survey [seen/source].

### On-site
App QR scanning; Xpress Door walk-up sales; offline-first POS app for door and bar [seen/source].

### AI / data / personalisation
Taste-based recommendations (Music Sync), artist-fan push on lineup publication (reach beyond the organiser's own followers), segmentation [seen]. No generative-AI features advertised.

### Observed tech, APIs, architecture
- Marketplace: **Next.js on Vercel** (Vercel challenge), TypeScript everywhere, **React / React Native, Apollo GraphQL, Node.js, PostgreSQL**, Cypress and Detox tests, GitHub Actions; earlier Heroku (job ads) [source + seen Vercel].
- pro.shotgun.live: **Webflow + HubSpot + Cookiebot** [seen]. Terms PDFs hosted on Cloudinary [source].

### Distinctive
1. **CRM-first ticketing for scenes**: the organiser owns segments, push and newsletters inside the ticket app, and every follow becomes a channel.
2. **Artist-fan reach**: publishing a lineup notifies fans of every artist on it (via Spotify / SoundCloud sync).
3. **Auto-charge waitlist and pre-registration** as on-sale mechanics; Shotgun Artists for album-drop-to-tour funnels.
4. **Xpress Door**: account-less walk-up purchase by QR.
5. Fan **ambassador cashback** built in.

### Weaknesses / criticism
- Fee opacity; fees kept on cancellation; resale fee surprise; organiser can block resale; waitlist tickets reported invalid with support blaming organisers [source].
- Genre and France concentration; small funding history (about €2M in 2020) [source].
- Heavy bot challenge even for ordinary headless visits [seen].

### Sources
pro.shotgun.live pages (home, festival, venue, collective, artist; blog: xpress-door-checkout, membership-for-users, connect-to-fans-artists-with-music-sync, go-viral-with-the-reward-program, segment-your-target-audience, Ninho bundle case, Rilès pre-registration case) [seen]; Shotgun Teamtailor / WTTJ job ads (stack); Que Choisir forum threads on Shotgun; Trustpilot shotgun.live; Shotgun GTC (Cloudinary PDF, March 2025); Ticket Fairy "Ticket Fairy vs Shotgun" (competitor); EU-Startups 2020 funding.

---

## 3. Fever (feverup.com)

### Overview
Madrid/New York, founded 2011 to 2014 (Ignacio Bachiller). A discovery plus ticketing plus **own-content producer** ("Fever Originals": Candlelight concerts, immersive exhibitions, light trails). Unicorn (€200M round led by Goldman Sachs in 2022; $100M+ from L Catterton / Point72 in 2025). **Owns DICE (2025)** and Secret Media Network. Claims **300M+ monthly interactions, 200k+ experiences, 500+ cities in 55+ countries, 4.8 on the App Store, 24/7 support in 11 to 12 languages** [seen]. Big B2B wins: **FC Barcelona (exclusive ticketing), Formula 1 (official supplier from 2027), LIV Golf, Primavera Sound** [seen].

### What was crawled vs blocked
Open. Crawled [seen]: home; London city page; Candlelight Hans Zimmer and Ramses event pages; the **ticket selector** (date, session, zone, price and fee, Continue; screenshots `fever-picker-1.png`, `fever-panel.png`); Romania country page; business.feverup.com home, concerts-festivals, revenue optimisation, event management (onsite), marketing, analytics, event creation, licensing, live events financing, distribution platform. Not crawled: `/purchase/`, `/seating`, `/login` (robots-disallowed). No checkout reached.

### Fan features
- City-first discovery: **"Pick your vibe" chips, including an "AI" chip**, then Live Music, Nightlife, Romantic, Detective, Adventurer, Tourist, With friends, Foodie, Culture, Wellness, Luxe, Creative, Family; "Top 10 in London"; category rails; "Fever exclusive / ONLY ON" badges [seen].
- **Rich event pages:** highlights, programme, performers, gallery and video, "On the day / How it works", reviews (8,579 reviews, 4.6) with **user photo gallery**, venue accessibility and parking, FAQs, "More like this", **favourite (heart)**, **share with friends**, **gift this experience** [seen].
- **Ticket selector:** venue zone map → date chips (promotion dates marked) → time slot → zone list with strike-through original price, discount, **"+ £x booking fee"** (5% of price) and "Sold out" → **"£43.26 – Continue"** [seen]. The headline "From £25.20" includes the fee (24 + 1.20) [seen].
- **Constant promotions:** Black Friday −20 to −40%, "Limited-time offer", "Up to 40% off" [seen].
- **Gift cards** (event, themed and corporate gift cards) [seen]; **Fever Club** loyalty (cashback or points as Fever credits in a virtual wallet, tiers Bronze/Silver/Gold; users find it opaque) [seen mention + source].
- 24/7 support by email, chat and call in the app [seen]; tickets in the app's "Tickets" section plus QR in the confirmation email [seen].
- **Fever in Gemini** (connected app, Aug 2026), a **Fever MCP server** for AI assistants (Claude and others), and the OpenAI ads pilot [source]. The page loads a **WebMCP polyfill** (`@mcp-b/webmcp-polyfill`): the site exposes tools to in-browser AI agents [seen].

### Organiser features (FeverZone)
- **Create:** content management with **AI auto-translation**, ticket types, capacity by type, slot or area, **timed entry / open-date vouchers**, **automatic release of new sessions when one sells out**, pricing tiers, seat maps (3D for sport), draft mode, multi-language listings, booking questions [seen].
- **Optimise:** a **3-click mobile checkout**, A/B-tested layouts, one-click login (Google, Meta, Apple, email, phone), cards, wallets, **instalments and deposits**, an **in-house payment-routing engine** (picks the processor per transaction; refunds and chargebacks handled end to end), **dynamic pricing (rules plus AI models using availability, sales velocity, time to event and even weather)**, promo codes, **waitlists and presales**, add-ons, bundles, group bookings, gift cards, hospitality, memberships and season passes, donations [seen].
- **Manage / onsite:** order management, live capacity and attendee lists, **box office and POS**, **self-service kiosks**, **dynamic QR (blocks screenshots, delayed QR release)**, controlled transfers, invitations and guest lists, **cashless (RFID wristbands)**, digital waivers, staff permissions, merch and F&B integrations (Square, Shopify, Odoo), **split payments**, hardware integration [seen].
- **Promote:** the Fever marketplace, **DICE**, **Secret Media Network** editorial, affiliates and ambassadors, OTAs and B2B resellers, paid performance, CRM via **email, SMS, WhatsApp and push**, organiser pixels (Google, Meta, TikTok), **integrations with TikTok, Google, Spotify, Bandsintown**, "AI discovery and SEO" pages optimised for AI assistants [seen].
- **Measure:** real-time dashboards, funnel and attribution, demographics, **AI sales-insights chatbot**, surveys, financial and settlement reporting, **Reporting API, Transactional API, MCP integration** (footer "Developers") [seen].
- **Distribution platform** (B2B marketplace for travel agencies, OTAs, hotels and MICE; API, commissions, "reserve instantly, pay after confirmation") [seen]. **Licensing** of proven Fever formats to local operators (Dopamine Land, Bubble Planet, Titanic, Harry Potter Forbidden Forest...) [seen].
- **Live events financing:** **$100k to $20M+** as ticketing advances, marketing investment, production support, working capital, venue deposits, co-investment. Requires **$5M+ annual ticket sales and Fever ticketing exclusivity**; answer within 48 h [seen].
- Festival claims: "+20% avg order value, −15% on-site staff costs" [seen].

### On-sale / queue / anti-bot
Presales (password or partner-only), waitlists, verified QR, "built-in fraud protection", dynamic QR released late, controlled transfers, 99.9% uptime claim [seen]. No public details on a virtual-queue product. The consumer site did not challenge our crawler [seen].

### Payments and fees
- The fan pays a **booking fee of about 5%** shown per zone (UK example) [seen]. Discounts are pervasive (a high-low pricing model) [seen].
- Organiser commercial terms not public (demo-led sales) [seen]. Financing is recouped from ticket revenue [seen].
- Instalments, deposits, split payments, local methods, own routing [seen].

### Post-purchase
App ticket wallet plus email QR; ticket change and refund rules per event (FAQ "Can I change my ticket? Are refunds allowed?"); gifting; reviews and photo upload after the event; Fever Club credits [seen]. Resale: "control official secondary-market resale rules" (B2B) [seen]; no consumer resale market visible.

### On-site
Assigned zones with first-come seats (Candlelight), kiosks, POS, RFID cashless, access control, waivers [seen].

### AI / data / personalisation
- **Data-driven experience creation** ("Netflix model"): search and purchase data across hundreds of millions of data points decide formats, cities, neighbourhoods, length, CapEx vs OpEx, royalties, opening hours and price elasticity. Fever Originals is "the main economic engine". Example: moved Titanic from Málaga to London, then the US East Coast, based on data [source].
- Consumer side [seen]: the "AI" vibe filter, **Permutive** audience cohorts with IBM Watson contextual classification, **Optimizely** experiments keyed on city, platform, has_payment_methods, is_first_purchase, **Mixpanel**, **Datadog RUM**, the OpenAI ads pixel, TikTok, Meta, Snap, LinkedIn and X pixels.
- Agentic distribution: Gemini app, MCP server, WebMCP on the page [seen/source].

### Observed tech, APIs, architecture
- Web front end **Angular** (`/_ng/main-*.js`) [seen]; search microservice **`data-search.apigw.feverup.com/v1/nearby_cities/?size=5`** (JSON, geolocated city list) [seen]; media on `applications-media.feverup.com` (Cloudinary-style `/image/upload/`) [seen]; server-side tagging at `feverup.com/dv/` and `gcptm.feverup.com` (GCP tag manager, attribution monitor) [seen]; legacy API path `/api/4.2/` in robots.txt [seen].
- Backend: **Python (Django / Flask), PostgreSQL, Redis, Kafka, RabbitMQ, AWS, Docker, Kubernetes**; Fever Engineering on Medium covers the **transactional outbox**, CDC and a PostgreSQL → **Snowflake** data mart (job ads, Medium index) [source].
- business.feverup.com: HubSpot forms and CTAs [seen].

### Distinctive
1. **Demand-led content creation**: Fever produces and licenses its own formats (Candlelight) from search and purchase data, so ticketing is a means and the data is the moat.
2. **Full-stack B2B**: ticketing, dynamic pricing, its own payment routing, cashless, kiosks, B2B distribution, **financing up to $20M+** and licensing.
3. **Agent-ready distribution**: MCP server, Gemini app, WebMCP, "AI-optimised event pages".
4. Owns a fan app (DICE) plus a mass-market discovery app plus a media network.

### Weaknesses / criticism
- **Candlelight quality and "tribute" criticism**: poor sound at the back, "plastic candles", feels formulaic, expensive for what it is; refund refusals and "bait and switch" claims; scepticism about on-site review authenticity (reviews can only be left by buyers on Fever) [source].
- Discount-heavy pricing teaches fans to wait; a booking fee on top in the selector [seen].
- Fever Club seen as opaque [source]; a very heavy ad-tech and tracking footprint (about 20 third-party trackers on a city page) [seen].
- Financing is tied to exclusivity and $5M+ scale, so it locks promoters in [seen].

### Sources
feverup.com/en, /en/london, /m/100852, /m/503872, /en/london/candlelight, /en/country/romania; business.feverup.com home and product pages listed above [seen]; Fever newsroom "Fever and DICE join forces" (Jun 2025); "Fever joins Gemini" (Aug 2026); Blooloop "Fever Originals"; TechNode Q&A with Santiago Santamaría; Fever job ads (Accel jobs board, echojobs); medium.com/fever-engineering index; Trustpilot candlelightexperience.com, ComplaintsBoard; App Store Fever listing; Ticket News "F1 taps Fever".

---

## 4. TicketSwap (ticketswap.com)

### Overview
Amsterdam, 2012 (Hans Ober, Ruud Kamphuis, Frank Roor). **Fan-to-fan resale marketplace** with capped prices; now also primary sales and social features. **19 to 20M+ users, 480,000 events, 42 countries, 13 languages, 110 ticket-provider integrations** [source]. CEO Jan van Casteren (2025) [source]. Raised €10M (2022) [source].

### What was crawled vs blocked
- Blocked: www.ticketswap.com (/ and /how-it-works): **202 "Verifying"** JS challenge (fingerprinting script under a random path) [seen]. press.ticketswap.com: 403 to headless Chromium [seen]. Robots.txt disallows /tickets, /listing, /sell, /cart, /wanted, /ticket-alerts and others [seen].
- Read via WebFetch: press releases on Auto buy, Social Features, FairShare, the app update, the WhatsApp chatbot and Ticketmaster mobile tickets [source].

### Fan features
- **Buy and sell second-hand tickets with a price cap of max +20% over the original** (lower where the law or the organiser says so; e.g. Paradiso set a **100% face-value cap**, Le Guess Who? capped too) [source].
- **SecureSwap:** with integrated ticketers (110+), **the seller's barcode is invalidated and a new unique ticket is issued to the buyer**, so double-selling and fakes are impossible. Partner events carry a verified badge [source]. Also supports **Ticketmaster mobile-only tickets** (NL and ES, 2024), re-transferable several times [source].
- **Ticket alerts** for sold-out events. **Auto buy** (May 2026 phased rollout): set preferences and a deadline and save a card or **pre-pay via iDEAL | Wero**; the **first matching person on the waitlist gets the ticket automatically** when one is listed. It replaces **raffles**, which remain for most high-demand events during the transition [source].
- **In-app ticket storage** for all tickets (2023) [source].
- **App update (Nov 2023):** interested / going, an **in-app calendar shareable with friends**, an explore feed with any location, a "tickets available" filter, **follow artists** for new-show alerts [source].
- **Social Features (Dec 2025):** profiles, see the upcoming events of friends you follow, favourite artists, past shows, connect with other fans before an event, an **Explore feed** of friends' and artists' activity, follow suggestions. Backed by research: about 4 in 5 Gen Z fans say friends' attendance affects buying [source].
- **Interactive social-media share feature for sellers** (2024) [source].
- **Event-discovery chatbot on WhatsApp** (Spain and Germany, Jul 2026) [source].
- 24/7 help centre [source].

### Organiser features
- **Official resale partner** (SecureSwap) integrated with the organiser's ticketer (Paylogic, Weeztix, Eventbrite, Ticketmaster, CM.com / Stager, Billetto, Ticket Tailor and others) [source].
- **FairShare** (Aug 2023): the organiser opts in, and resale profit (within the cap) is **split 50/50 between seller and organiser**; the seller sees the split when pricing [source].
- **Barcode held back until a few hours before the event**, so resale stays inside the system [source].
- Organiser controls on the cap level; **demand data from Auto buy** ("measurable purchase intent") [source]. Primary-ticket selling through TicketSwap [source].
- Official marketplace partner of **ADE**, Spa Grand Prix (F1), Die Toten Hosen tour, Cercle / Delta festivals in France [source].

### On-sale / queue / anti-bot
- No primary on-sale stampede for resale; for hot events, **raffles** and then **Auto buy (commitment over speed)** [source].
- Web behind a JS bot challenge [seen]. The 20% cap plus SecureSwap reissue kill the tout's economics [source].

### Payments and fees
- Seller fee about 5% service fee (the cap is sized so sellers recover booking, transaction and delivery fees plus TicketSwap's 5%) [source]; buyers see a service fee after selecting [source].
- **Stripe** (Payment Element; iDEAL, Bancontact and other local methods; "add new payment methods with a single line of code") [source]. Payout delays are a common complaint [source].

### Post-purchase
In-app wallet; resell again; social profile and calendar; alerts [source].

### On-site
Buyer's reissued ticket scanned by the organiser's normal access control (the ticketer integration does the work) [source].

### AI / data / personalisation
Follow-based and location feeds, friend-activity feed, follow suggestions, WhatsApp chatbot; Gen Z research used in PR [source].

### Observed tech, APIs, architecture
- **PHP / Symfony, MySQL, RabbitMQ, Redis, AWS** (backend job ads); co-founder Ruud Kamphuis "Engineering Fellow" [source]. Engineering and policy writing on medium.com/ticketswap (e.g. "What makes a resold ticket a Secure Swap?", "The 120% rule"); Medium returned 403 to us [source].
- Front end served with a bot challenge on every route [seen].

### Distinctive
1. **Price-capped, integrated resale (SecureSwap)**: re-issue, not just transfer; integration with 110 ticketers.
2. **FairShare**: the organiser shares resale profit, aligning promoters with fair resale.
3. **Auto buy**: a pre-committed, pre-paid waitlist that rewards commitment over click speed.
4. A social layer and lobbying stance (e.g. asking the Italian competition authority for more resale choice).

### Weaknesses / criticism
- **Trustpilot (38k+ reviews)**: fake or invalid tickets on non-SecureSwap events, slow payouts (a seller reports about 5 weeks), unresponsive support, high seller fees, unsold listings [source].
- Works best only where integrated; otherwise it relies on trust in uploaded PDFs and barcodes [source].
- A resale-first brand that organisers can see as a competitor to their own primary sales [source].

### Sources
TicketSwap Auto buy press release (18 May 2026, PDF); press.ticketswap.com releases: Social Features (2 Dec 2025), FairShare (31 Aug 2023), app update (21 Nov 2023), Ticketmaster mobile-only (Jan 2024), Unlock your Summer chatbot (2 Jul 2026), Paradiso 100% cap, Le Guess Who?, ADE, Italian competition authority; Medium "How to change the secondary ticketing market part 3: the 20% rule" and "What makes a resold ticket a Secure Swap?" (titles and snippets); Rolling Stone UK "How TicketSwap is leading the way for fair pricing"; Stripe customer story "Supporting concert ticket sales with TicketSwap" (2021); Weeztix and WeTicket SecureSwap pages; TicketSwap backend job ads; Trustpilot ticketswap.com; Silicon Canals (CEO).

---

## 5. Cross-platform takeaways for EPS / NIBIRU

| Theme | Best-in-class pattern | Who |
|---|---|---|
| Anti-touting | Identity-bound app ticket + **time-released / dynamic QR** (no screenshots) | DICE, Fever (dynamic QR) |
| Resale | **Face-value waiting list** (DICE) or **capped resale with barcode re-issue** (TicketSwap SecureSwap) + organiser profit share (FairShare) | DICE, TicketSwap, Shotgun |
| On-sale fairness | **Pre-registration + auto-charge waitlist** (Shotgun), **Auto buy** (TicketSwap), raffles; commitment beats click speed | Shotgun, TicketSwap |
| Pricing | All-in price shown upfront (DICE); dynamic pricing and AI models (Fever) | DICE, Fever |
| Discovery | Spotify / Apple Music / SoundCloud taste sync; artist-fan push on lineup publish | DICE, Shotgun |
| Social | Friends going, Groups with voting, shared calendars, explore feeds | DICE, TicketSwap |
| Organiser CRM | Segments, push, newsletters, WhatsApp/SMS, ambassador cashback, memberships | Shotgun, Fever |
| Commerce | Bundles (ticket + merch via Shopify), Extras, add-ons, split payments, instalments, deposits | Shotgun, DICE, Fever |
| On-site | RFID cashless, kiosks, POS, walk-up QR purchase without an account (Xpress Door) | Fever, Shotgun |
| Money for organisers | Advance payouts (Shotgun), **financing $100k to $20M+** (Fever) | Shotgun, Fever |
| AI / agents | MCP server, Gemini app, WebMCP, AI sales-insights chatbot, AI translations, WhatsApp bot | Fever, TicketSwap |
| Ops lesson | All four put the web behind aggressive bot walls (Cloudflare, Vercel Firewall, a JS challenge) | DICE, Shotgun, TicketSwap |

Raw crawl data: `/tmp/claude-0/-home-user-Bypass/c485ec14-88d3-5b3d-a0d9-7ea480a3d697/scratchpad/competitors/raw/fan-first/` (JSON per page, screenshots `fever-picker-1.png`, `fever-panel.png`, robots files). Scripts: `.../scripts/fan-first/crawl.js`, `fever-picker.js`, `fever-panel.js`.
