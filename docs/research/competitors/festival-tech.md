# Festival and enterprise ticketing + festival tech: competitive research

Prepared 27 Nov 2026 for the NIBIRU CTO interview (74-day seasonal resort with festivals of up to ~280k people, promenade, venues, beer garden, padel, camping, tables, cashless card, mobile app; EPS ticketing to become a "next-gen" platform open to any organiser).

Legend: **[seen]** = observed by our own Playwright/curl crawl on 27 Nov 2026 (raw JSON in `raw/festival-tech/`). **[source]** = from public docs, case studies, press (listed per section).

Crawl method: Playwright Chromium, default UA, robots.txt checked first, one page at a time, 1.2 s between pages (10 s for solutions.axs.com, which sets `Crawl-delay: 10`), under 25 pages per site. Recorded visible text, buttons and XHR/fetch calls. No accounts created, no pre-registration, no orders, no payment details. Bot walls were not bypassed: when blocked we stopped.

| Site | Result |
|---|---|
| axs.com | **Blocked** [seen]: Cloudflare bot page "Ensuring a fair fan experience" on every page (home, about, Mobile ID). Only solutions.axs.com (B2B WordPress) loaded |
| seetickets.com | **Blocked** [seen]: 403 even on robots.txt and via fetch tool |
| paylogic.com | Crawled 6 pages [seen]; customerservice.paylogic.com behind Cloudflare challenge (403) |
| weezevent.com (incl. PlayPass) | Crawled 16 pages [seen] |
| tomorrowland.com family | Crawled 16 pages (belgium/winter/brasil/thailand sales info, My Tomorrowland shell) [seen]; ticket shop itself is behind login, not attempted |
| queue-it.com | Crawled 13 pages [seen] + GitHub connector docs [source] |
| developers.cloudflare.com | Waiting Room docs fetched as Markdown (robots/content signals allow) [seen] |
| intellitix.com | 8 pages [seen]; guest help centre behind Cloudflare challenge |

---

## 1. AXS (AEG)

**Overview.** AEG's ticketing arm (US, UK, Sweden, Germany, Japan, ANZ, Singapore). Primary + resale + venue tech for arenas (The O2, Crypto.com Arena), stadiums, some festivals; ABBA Voyage. Recently bought a German white-label e-commerce ticketing company and is expanding in Europe [seen, solutions.axs.com/uk news list].

**Crawled vs blocked.** axs.com: every page returned a Cloudflare challenge page with "To ensure a fair experience for all fans, we actively prevent automated bots" plus a Request ID and IP; XHR showed `api.axs.com/proxy/v2/users/country?access_token=...` and a `cdn-cgi/challenge-platform` POST [seen]. B2B site solutions.axs.com loaded (capability tiles: Data & Analytics "real-time data & reporting", Consumer Experience, Marketing Services, Venue Operations, Venue Experience, Partnerships & Integrations "sell tickets with official partners and platforms where customers are") [seen].

**Fan features.** AXS Mobile ID: one account-level ID in the app, rotating barcode (AXS/O2 say every 59 s) that can't be screenshotted, scans offline [source]. Transfer and relist in app [source]. "Contactless, secure and personalised venue experiences" [seen].

**Organiser features.** Data & analytics, marketing services, venue ops, distribution partners (official marketplaces) [seen]. Mobile ID used to "know and reward fans for their loyalty and behaviours" [seen].

**On-sale / queue / anti-bot.** AXS Waiting Room opens ~30 min before on-sale on busy events; AXS explicitly calls it "a holding area, not a queue"; at on-sale fans are **randomly selected after bots are filtered out** [source, AXS UK features page via search]. Cloudflare bot management in front of the whole site [seen]. AXS has sued companies selling reverse-engineering services for non-transferable tickets [source].

**Payments.** Standard card / wallet; no notable instalment products found.

**Post-purchase.** AXS Official Resale; in the UK resale price is capped at max 10% above face [source]. Transfers built in.

**On-site.** Mobile ID rotating barcode at the gate; venue ops and venue experience modules [seen]; no own RFID/cashless product visible.

**Data/AI.** "Real-time data & reporting for real-time decision making" [seen]; Mobile ID ties every entry to a known account [seen].

**Tech/APIs.** Partner integrations; no public developer portal found.

**Distinctive.** Rotating, identity-bound barcode at account level; AEG owns venues + promoter + ticketing (closed loop of data); resale cap.

**Weaknesses.** Arena/stadium DNA, not multi-day festival or resort (no cashless, accommodation, packages); "holding page with random selection" is opaque to fans; bot wall is aggressive (blocked a plain browser on every page, which also hits real users on VPNs/iCloud Private Relay, per its own advice to "disable your VPN or Private Relay").

**Sources.** solutions.axs.com/uk, /us [seen]; axs.com/uk/features, axs.com/faq, theo2.co.uk AXS Mobile ID guide, slashgear.com (AXS lawsuits) [source].

---

## 2. See Tickets (now CTS Eventim)

**Overview.** UK-born ticketer (Glastonbury's exclusive agent for years), owned by Vivendi until CTS Eventim bought Vivendi's ticketing + festival business (~$323M, closed 2024; See Tickets US rebranded EVENTIM in 2025) [source].

**Crawled vs blocked.** seetickets.com returns HTTP 403 for robots.txt and all pages, to both curl and the fetch tool [seen]. Not pursued further.

**Fan features.** See Tickets Wallet app: digital tickets with "tamper proof" barcodes against touts; Fan-to-Fan resale inside the wallet, resold ticket moves to buyer's account and can't be revoked by the seller [source].

**Organiser features.** Event-specific storefronts (e.g. glastonbury.seetickets.com), registration-gated sales, deposits/balance, coach + ticket bundles [source].

**On-sale / queue / anti-bot (Glastonbury, the reference case).**
- Demand: ~2.4 to 2.5 million people try for ~210k capacity (incl. staff) [source].
- **Registration first**: everyone 13+ must be registered (historically with a photo printed on the ticket) to buy; lead booker can buy for up to 6 registered people, each identified by registration number + postcode [source].
- Until 2024: "holding page" that fans refreshed; repeated crashes ("processing the maximum possible number of transactions per second", 2015) and a 2023 technical delay [source].
- **2025 (Nov 2024 sale)**: new queue. Everyone already on the page at opening is **randomly assigned** a queue position, late arrivals go to the back, progress bar, "don't refresh, don't use multiple tabs/devices, one session per IP or you may be blocked as a bot" [source, official FAQ].
- Session timers: 10 min on the details page, separate timer on payment; tickets only allocated when payment succeeds (basket is not a reservation); failed orders released back to sale; SOLD OUT shown only after all in-flight purchases processed [source].
- Outcome: coach+ticket sold out in < 1 h; GA in ~35 to 37 min [source]. Resale (spring) sold out in 22 min in 2024 [source].

**Payments.** £75 deposit at booking, balance in first week of April (≈ 5-month payment plan) [source]; Apple/Google Pay; resale tickets paid in full within 24 h (2023) [source].

**Post-purchase.** Official resale of cancelled/returned tickets only via the festival's See Tickets store [source].

**On-site.** Printed photo tickets (Glastonbury) / app barcodes; no own cashless.

**Data/AI.** Not documented publicly.

**Tech/APIs.** No public API found.

**Distinctive.** Registration + deposit model: identity before sale, money spread over months, which kills most touting and lowers the payment-time peak.

**Weaknesses.** Public history of on-sale failures; payment card breach affecting ~323k customers over 2019-2022 [source]; fans called the 2025 queue "horrendous" (long, little information) [source]; site blocks even plain browsers.

**Sources.** glastonburyfestivals.co.uk/news/2025-ticket-sale-faq/; 2025 resale FAQ; NME, ITV, Euronews, Mixmag coverage; musicbusinessworldwide.com (Eventim deal); pollstar (rebrand); ticketnews.com (breach); support.seetickets.com Fan-to-Fan [source].

---

## 3. Paylogic (EVENTIM Benelux; Tomorrowland's ticket shop)

**Overview.** Dutch festival ticketing (Amsterdam/Groningen/Berlin/Antwerp offices). Footer now reads "Working at EVENTIM Benelux" [seen]; Eventim describes it as its "self-service ticketing solution… market leader for large-scale festivals" [source]. (Not part of CM.com: CM.com's ticketing is its own platform plus GUTS Tickets, bought Feb 2025 [source].) Clients: Tomorrowland (personalisation e-mails come "from Paylogic" [seen]), Awakenings, Dour, ADE, Dekmantel.

**Crawled.** paylogic.com home, features, 3 case studies, purpose [seen]. XHR: only analytics + a Craft CMS `_dynamic` include; no shop APIs on marketing pages [seen].

**Fan features.** Branded shop inside the organiser's identity; account per fan (Awakenings: "100% of ticket buyers create an Awakenings account", optional Facebook login) [seen]; Send 2 Friend; waiting lists for sold-out events [source].

**Organiser features** [seen unless marked].
- "No stress during peak traffic… just like we do for Tomorrowland, which has the world's biggest peak sales."
- Shops in your visual identity; template duplication [source].
- Reporting suite: filter and compare multiple events, easy exports.
- E-commerce for the entire fan journey: "tickets to travel services, activities, merchandise, consumption, transportation" add-ons. Dour (240k visitors): camping + hotels + merch + shuttles in one shop, record sales 2017. **ADE: 221 events from 74 promoters in one shop with one account; hotels; event holders get a share of hotel bookings; loyalty platforms linked** — this is the closest existing model to "open platform for any organiser inside one destination".
- Upsells before/during/after purchase: merch, tokens/digital cash, camping, parking, lockers, VIP upgrades [source].
- Pre-registration, waiting lists, 27+ tracking pixels, server-side tracking, CRM export [source].

**On-sale / queue / anti-bot.** Own queue: "Talk to (digitally) queuing visitors": message people in the queue (bad news when sold out, "extra tickets available", congratulations) [seen]; collect personal data during the queue [source]. For Tomorrowland the shop is only reachable via a personal link in the logged-in Tomorrowland Account (see §5). Exact ordering algorithm not published.

**Payments.** Cards to iDEAL, multi-currency [seen].

**Post-purchase.** Personalisation flow (link in confirmation e-mail, deadline, then e-ticket) [seen via Tomorrowland]; fan-to-fan resale with controlled pricing [source].

**On-site.** Paylogic Entrance scanning app (iOS/Android, real-time reporting) [source]; cashless via partners through API [source].

**Data/AI.** Account-based data; "personalised ticket sales and secondary ticketing to targeting of certain interests" [seen].

**Tech/APIs.** "Open API" to marketing tools, access control, cashless, CRM [source]; not public.

**Distinctive.** Proven at the world's biggest festival peak; multi-promoter single-shop (ADE); account-first data capture; queue messaging.

**Weaknesses.** Enterprise/sales-led, docs closed; marketing site case studies date from ~2017; no native cashless/RFID; now inside Eventim (independent organisers may worry about data going to a competitor-promoter group).

**Sources.** paylogic.com/en, /features, /stories/* [seen]; eventimb2b.nl/en/our-ticketing-technology/paylogic, cm.com press (GUTS) [source].

---

## 4. Weezevent (incl. PlayPass, WeezPay, WeezAccess, WeezCrew, Gigz)

**Overview.** French "Europe's leading independent ticketing platform", now in the UK; "1 million events/organisers"; "European leader in cashless" (600+ events/year); PlayPass (Belgium) joined in Sept 2020 [seen]. Testimonial from "Event operation at Tomorrowland" on the wristband page [seen]. Boomtown, Splash, Hellfest, AC/DC use Weezevent [seen/source].

**Crawled.** 16 marketing pages [seen]. XHR: only marketing pixels (Reddit, HubSpot, Snitcher, YouTube) [seen].

**Fan features.** Branded mini-site / widget; e-tickets with passbook (wallet); cashless wristband or card linked to ticket; top-ups on website/app, QR on wristband, self-service terminals; pre-order / click & collect (even "in front of stages"; resort example: "order without leaving their mobile home"); smart reusable cups with RFID deposit refunds only to the original buyer [seen].

**Organiser features** [seen].
- Self-service ticketing: 2.5% + £0.99 per ticket, payouts every 15 days, free for free events, no data exploitation ("Weezevent will never contact your attendees").
- Price tiers, weekend pass, **camping packages**, custom forms, branded tickets, badges/accreditations.
- Add-ons on quote: dedicated project manager, secure resale platform, numbered seating 2D/3D, on-site booths, **virtual queue**.
- WeezAccess: free; phone app, pro NFC terminals, turnstiles, paper list; access lists by event/ticket type/timeslot.
- **NFC intra-site access**: zones (backstage, VIP, fan areas), groups (public, VIP, partners, press, staff), live check-in counts "to accurately determine crowding levels at any given moment".
- WeezPay cashless: £1.20 per participant (list price); comp tickets and catering credits loaded onto staff/volunteer wristbands automatically; partner activations.
- WeezCrew: volunteer recruitment forms, shifts, role assignment, accreditation per area, segmented SMS/e-mail (from £1,000; "10 to 10,000 volunteers").
- Gigz: CRM + ad campaigns (separate brand).
- Verticals include tourism, theme parks, campsites, food courts [seen].

**On-sale / queue.** Integrated virtual queue: "from the moment tickets go on sale visitors are placed in a virtual queue and directed to the buying stage at a controlled rate… dynamically adjusts throughput"; AC/DC and Hellfest stories [seen]. No published randomisation detail.

**Payments.** Bank fees included; card + cashless EPTs that combine bank card and closed-loop credit [seen]. Payment partners Adyen/Monext [source].

**Post-purchase.** Resale: validity checked, **old barcode invalidated and a new one generated** (re-personalisation), seller refunded automatically on sale; ticket insurance option ("secure their ticket") [seen].

**On-site (cashless detail)** [seen]: "an average of **15% of cashless balances left unused** at the end of an event" → convert into merch or early access next edition instead of refund; refunds automatic or on request, or donation; offline-capable terminals ("network independence"); real-time sales dashboards; staff catering credits; activation and incentive campaigns with partners.

**Data/AI.** Attendee habits for next edition; real-time dashboards [seen]. No AI claims.

**Tech/APIs.** REST API (api key + access token; participants, checkpoints, widgets, promo codes, webhooks) per third-party listing [source]; docs not public.

**Distinctive.** Only vendor in this group covering ticketing + resale + queue + NFC access + cashless + cups + pre-order + staff in one stack, with public self-serve prices.

**Weaknesses.** Cashless and enterprise modules are quote/project-based; fragmented sub-brands (Weezevent, PlayPass, Weeztix, Gigz); cashless refund fees/deadlines draw consumer complaints in Belgium (Test-Aankoop) [source]; no public API docs; queue fairness unspecified.

**Sources.** weezevent.com/en-gb/* [seen]; jentic.com Weezevent API listing; brusselstimes.com/test-aankoop.be (festival payment complaints); boomtownfair.co.uk/cash-free; splash-festival.de [source].

---

## 5. Tomorrowland's own ticketing (account + Paylogic + own travel shop)

**Overview.** ~400k+ visitors over two weekends in Boom, plus Winter (Alpe d'Huez), Brasil, Thailand. The best-documented festival sale model in the world.

**Crawled.** belgium/winter/brasil/thailand sales-info, how-to-order, tickets, add-ons, exchange desk, payment methods, personalisation, sales dates, official channels, travel partners; My Tomorrowland shell [seen]. Shop and account are behind login, not attempted.

**Stack observed [seen].**
- `my.tomorrowland.com` is an SPA whose public `env.js` lists `LOGIN_URL login.tomorrowland.com`, `BASE_API_URL api.tomorrowland.com`, **AWS Cognito** app id, **AWS WAF Bot Control CAPTCHA SDK** (`*.edge.captcha-sdk.awswaf.com/.../jsapi.js` + WAF API key), feature flags `EXCHANGE_DESK_V2`, `COMPANY_ACCOUNT`, `NOTIFICATION_CENTER`, `TICKETS_TO_EVENTS`; Winter bookings in a separate `managemybooking.tomorrowlandwinter.com`; Brasil tickets via **Inti** (`tomorrowlandbrasil.byinti.com`). Bugsnag tracing, Tolgee i18n CDN, server-side GTM on Cloud Run (europe-west4).
- Festival sites (Next.js SSG) fetch `fallback.tomorrowland.com/<host>/fallback.json?cacheBust=…` on every page: a **kill-switch/static fallback** they can flip at peak (currently `{}`).
- Web components from `components.tomorrowland.com` (navigation, footer, live line-up) shared by all festival sites.

**Fan features / sale mechanics (Belgium 2027)** [seen].
- **Tomorrowland Account + Pre-Registration** (2 Dec 2026 15:00 CET to 12 Feb 2027 15:00 CET; "does not guarantee a ticket"). Shop reachable only through a **personal link inside the account**.
- **Staggered sale moments**: Global Journey Travel Package sale (30 Jan 2027 17:00 CET) → WorldWide Pre-Sale (6 Feb) → WorldWide Ticket Sale (13 Feb). Plus an NFT sale (max 2 people) and **"First 20 Sale": the first 20 pre-registrants per country of residence can buy 4 tickets** (a cheap, fair "early bird" for every country).
- **Caps per account, cumulative across all sale moments**: 4 people for tickets (Friendship Garden Packages excepted), 6 for Global Journey (Friendship Travel Packages excepted).
- **Global Journey**: package builder (choose package → weekend → departure city, accommodation, regular/Comfort ticket); "Simulate your Global Journey Package" before the sale; packages tied to a weekend (Discover Belgium/Europe/Ibiza on W1; Youth, Surprise, F1 Belgian Thrill on W2); DreamVille camping only as package with a Full Madness pass; official travel partners per continent who ship the **Bracelet + Treasure Case** that the fan links to the account.
- Winter: festival + ski pass bundles; add-ons (activities, lockers, ski lessons, equipment, slope insurance, shuttles from European cities, transportation pass, towels) on their own sale date; hospitality (Exclusive, Business) packages; travel/cancellation insurance. Thailand: food experiences, lockers, parking add-ons; "My Bracelet" section.
- **Groups/friends**: Friendship Garden / Friendship Travel packages.

**Payments** [seen].
- Global Journey: 3.4% booking fee; **credit card only, no debit**; "failed payments will not be reprocessed and your application will automatically be rejected".
- **Payment plan for packages ≥ €3,000**: €2,500 deposit + 3 monthly instalments auto-charged to the same card; below €3,000 pay in full.
- Treasure Case fee refundable only if cancelled before 29 Apr; €15 ticket refund fee.

**Post-purchase** [seen].
- **Mandatory personalisation**: every ticket to one named person before a deadline (link in the Paylogic e-mail or the account); unpersonalised orders may be cancelled and resold; no name changes after; name on bracelet must match ID at entry; Global Journey personalised at purchase.
- **Exchange Desk** (official resale): opens after the WorldWide sale (13 Feb), main buyer registers tickets until 29 Apr, irreversible, no guarantee; tickets are re-sold through the **Waiting List Sale**; seller refunded to original payment method within 8 weeks minus €10/ticket; not for Global Journey.
- Belgian law of 30 Jul 2013 forbids resale at profit (fines up to €60,000); illegitimate tickets cancelled.

**On-site.** Each bracelet has a unique barcode or RFID code, ID check at entry [seen]. Cashless currency **Pearls** on the bracelet: online top-ups refunded automatically after the festival (fee 2 Pearls; bonus Pearls excluded), on-site top-ups need a manual request with the 12-character bracelet ID + 6-digit security code before a deadline [source]. Historically Intellitix ran Tomorrowland's fully cashless RFID [source]; Weezevent now quotes a Tomorrowland operations testimonial [seen].

**Data/AI.** Every buyer is a known account with country of residence; notification centre flag [seen].

**Distinctive.** Identity before demand (account + pre-registration + per-account caps), demand spread over 3 to 5 sale moments, travel packages as a first sale (highest margin, sells to the most committed), payment plan, re-sale only via waiting list (never a free market), bracelet mailed in advance.

**Weaknesses.** Fan complexity (many dates, rules, fees: 3.4%, €15, €10, Treasure Case, 2 Pearls); credit-card-only package sale excludes many young fans; cashless refund deadlines generate complaints; several systems stitched together (Paylogic, own travel shop, Winter booking system, Inti for Brasil).

**Sources.** belgium.tomorrowland.com/en/sales-info/* (how-to-order, sales-dates, payment-methods, personalization-process, exchange-desk, official-ticket-channels, official-travel-partners), winter.tomorrowland.com/en/tickets|add-ons|sales-info, brasil.tomorrowland.com/en/tickets/global-journey, thailand.tomorrowland.com/en/add-ons, my.tomorrowland.com/assets/env.js [seen]; indies.es Pearls guide, test-aankoop.be complaints, intellitix.com/hub Tomorrowland cashless photoset, edmtunes.com (2026 sold out 31 Jan 2026 incl. Global Journey) [source].

---

## 6. Festival cashless / RFID vendors: Intellitix (and PlayPass, Weezevent above)

**Overview.** Intellitix: "Pioneers in RFID since 2010, 2,000+ events", North America focus; "5,000 or 500,000 attendees"; did Tomorrowland's first fully cashless edition [seen/source]. PlayPass: Belgian cashless/access vendor (Time Warp etc.), part of Weezevent since 2020 [seen].

**Crawled.** 8 Intellitix pages [seen]; guest help centre (Zendesk) behind Cloudflare challenge.

**Positioning.** "We don't replace your ticketing. We complete it… Your ticketing sells the ticket. We power everything after the scan"; integrates with "20+ ticketing platforms" [seen].

**Fan features.** One wristband for entry + spend; mailed pre-loaded credentials (ticket type, access tier, cashless balance); auto top-up; online and on-site top-ups [seen].

**Organiser features** [seen].
- Access control: RFID tap "in milliseconds", dozens of gates/lanes, **live entry counts by hour and access point**, unique RFID ID per credential, **real-time pass-back detection**, entry **and exit** logging for capacity, unlimited zones (GA, VIP, artist, media, staff, sponsor), change permissions mid-event, zone-level attendance and congestion data.
- Three POS tiers: ITX POS (card/Apple/Google Pay, no RFID), ITX POS+ (RFID tap-to-identify, per-guest analytics, card payment), ITX RFID POS (closed loop, top-ups, auto top-up, no cash). Claims: 15 s average transaction (vs 90 s), 100+ transactions/min, **offline mode with auto sync**, multi-vendor menus with unified settlement, "35 to 70% increase in per-guest spending", "40% higher average guest spend", "up to 60% faster transactions".
- Payments: organiser as **merchant of record**, funds during or right after the event, automatic multi-vendor settlement split by vendor, zone and payment method, PCI.
- Fulfilment: encoding, labelling, kitting, shipping, returns of wristbands.
- Giveaway and redemption per credential (no double redemption), meal redemptions, sample activations, F&B and merch counts.

**Data/AI and sponsorship** [seen]. Per-head spend by ticket type ("GA may spend more than VIP"), entries by time/location, **product sales by demographic "to create more compelling sponsorship packages"**, spend by postcode, sales by hour and location, live customisable dashboards.

**Case numbers** [seen]. Pro golf: 30k+ credentialed across zones; Seabreeze Jazz Festival: $180k+ cashless revenue with one wristband.

**Distinctive.** Ticket-agnostic "operational layer"; pass-back and exit logging; merchant-of-record and settlement.

**Weaknesses.** Depends on someone else's ticketing and identity; marketing numbers are vendor claims; North-American focus (USD, zip code); no fan app of its own visible; RFID hardware and fulfilment logistics cost.

**Sources.** intellitix.com, /itx-festival, /accesscontrol, /itxpos, /data-and-insights, /payments, /ticketing, /fulfillment [seen]; intellitix.com/hub Tomorrowland [source].

---

## 7. Queue-it (virtual waiting room)

**Overview.** Danish SaaS waiting room; "500+ ticketing companies", "100 billion+ visitors", "100,000+ peak events"; Ticketmaster "blocked 13+ billion ticket bots over 17,000+ events with the help of Queue-it" [seen/source].

**Crawled.** 13 pages incl. developer "How Queue-it works", connectors, bots, invite-only, scheduled, pricing, agentic AI [seen]; GitHub KnownUser docs [source].

**How it works (mechanism)** [seen unless marked].
- Not a reverse proxy: a waiting page "inserted between two web pages" on Queue-it's infrastructure, so no sensitive data passes through it.
- **Triggers → Actions** (URL, request body, user agent, cookie, HTTP header, JavaScript): protect the whole site, a landing page, or only "Add to cart".
- **Pre-queue**: from a set time early arrivals see a countdown; at sale start **all are randomised like a raffle** and given positions; later arrivals are FIFO at the back. Waiting room auto-switches off when empty.
- **Invite-only**: list of unique identifiers (e-mail or generated IDs) → signed, encrypted links; optional 2FA; single-use; tiered rooms per loyalty level; "queue fast pass" to drive sign-ups.
- **24/7 protection**: always on, activates only above thresholds, FIFO.
- Integration: (1) simple link, (2) client-side JS (skippable by tech-savvy users), (3) edge connectors (Akamai EdgeWorkers, CloudFront, Cloudflare Workers, Fastly…, unskippable), (4) server-side KnownUser SDKs (.NET, Java, PHP, Lua/NGINX, Ruby, Python, Node; "under a day"). v5 "Direct Pass" skips the 302 in non-queue situations.
- **Token validation** [source, GitHub]: on return the URL carries `queueittoken` with `q` (queue ID), `ts` (expiry), `h` (hash, HMAC with a 72-char secret key). Connector validates timestamp + hash, then sets cookie `QueueITAccepted-SDFrts345E-V3_[WaitingRoomId]` with configured TTL; invalid → back to queue. Validate only page/AJAX requests, strip the token from the URL to stop sharing. Queue Token (enqueue token): JSON header + AES-256-encrypted payload, SHA-256 signed, optional event ID and client IP binding, unique token ID against replay [source].
- Position is tied to the session and survives a dropped connection or closed tab; e-mail notification when it's your turn; real-time messages to the room [seen].
- Outflow set in visitors/minute and changed live (Megatix raised it gradually while watching the DB) [seen].

**Anti-bot** [seen].
- Traffic Access Rules on 12 condition types (IP, ASN, UA, classifiers, room type): block, challenge, bypass.
- Visitor reputation scale Genuine → Malicious; native **Netacea** AI detection; **Akamai Bot Manager** inside the room.
- "Hide detection until it's too late": mitigate only at sale start, after randomisation, either hard-block or send bots to the back of the FIFO queue (prevents retooling).
- **One place in line per e-mail, account or promo code**; end session validity after purchase (anti-hoarding); alerts when one IP holds many positions or a data centre surges.
- Case numbers: European ticketing "hottest ticket of the year": **37.7M bots and uninvited visitors blocked, 28k verified visitors reached the site, 99.9% less load**; one concert: 3.3M requests, only 138k (4%) legitimate; one scalper opened a presale link 31,325 times and got one spot; US ticketer 8.3M bots over 50+ on-sales; "bad bots are 40% of ticketing traffic" [seen/source].
- Agentic AI: "visitor MCP server" for AI agents to queue under the same rules [seen].

**Other numbers** [seen/source]. Beyond the Valley (Megatix): 100k+ expected, ~50 ticket types (multi/single day, camping, bus, car passes), **inflow > 1,000/min for 30+ min**, outflow raised step by step; 3M+ visitors, 100% uptime over 50+ on-sales, 40% fewer support requests. Rock in Rio (Ingresso): > 10,000/min before the 19:00 sale, > 1M visitors, peaks ~25,000/min. Sympla: peaks 18,000/min, 50% lower scaling cost. Survey: 37% conversion increase quoted, 69% "sales fairer".

**Visitor engagement** [seen]: polls, quizzes, e-mail sign-up and cross-sell widgets inside the waiting room (zero-party data).

**Pricing** [seen]: Standard 15k queued visitors/month (1 active room), Pro 100k/month (2 active), Enterprise custom; all include scheduled room, 24/7, 25+ connectors, real-time metrics, FIFO, 50+ languages.

**Distinctive.** Randomised pre-queue + FIFO, signed tokens, invite-only, bot-vendor integration, live outflow control.

**Weaknesses.** Extra redirect hop and third-party dependency at the most critical minute; client-side mode is skippable; per-visitor pricing gets expensive at Glastonbury scale; protects the door, not the inventory DB (you still need atomic reservations); "randomised" fairness still rewards people with many devices/accounts unless you enforce one place per identity.

**Sources.** queue-it.com/developers/how-queue-it-works, /how-does-queue-it-work, /bad-bot-protection, /product/invite-only, /product/scheduled, /fair-access-inventory-allocation, /ticketing, /hype-event-protection, /developers/connectors, /pricing, /agentic-ai, /visitor-engagement, /success-stories/megatix [seen]; queue-it.com/blog/online-ticket-queue; github.com/queueit/Documentation (serverside-connectors), npmjs.com/package/@queue-it/queue-token [source].

---

## 8. Cloudflare Waiting Room

**Overview.** Edge waiting room built on Workers + Durable Objects, running in every Cloudflare data centre (300+ cities); Business/Enterprise plans, advanced features Enterprise [seen/source].

**Mechanism** [seen].
- Two thresholds: `total_active_users` (> 200; "set to 75% of origin capacity") and `new_users_per_minute` (> 200, ≤ total; "100% of peak").
- `session_duration` 1 to 30 min (default 5): active users and re-entry without queuing; **Session Revocation** via origin response header `Cf-Waiting-Room-Command: revoke` (e.g. after checkout).
- Queue page auto-refreshes every 20 s with estimated wait; `json_response_enabled` for native apps (Accept: application/json); custom HTML templates; `queueing_status_code`.
- Methods: FIFO (cookie timestamp), Random, Passthrough (analytics only), Reject (static page).
- **Scheduled events**: up to 5 per room; `prequeue_start_time` ≥ 5 min before `event_start_time`; `shuffle_at_event_start` = lottery; event-only overrides of thresholds, method, template.
- **Turnstile** inside the room: `invisible` / `visible_managed` etc., fail action `log` or `infinite_queue` (bots wait forever).
- Architecture [source, blog]: each data centre keeps a local state snapshot; Durable Objects aggregate globally "within a few seconds"; slots split per data centre by last minute's traffic share plus an "Anywhere" pool; newer design uses per-DC Durable Object counters (< 10 ms in-DC, 60 to 70 ms cross-region for Anywhere). Encrypted cookie holds `bucketId`, `acceptedAt`, `lastCheckInTime`; invalid cookie = new user at the back.

**Distinctive.** Zero extra hop (runs at the edge you already use), cheap, API-first, lottery mode, session revocation.

**Weaknesses.** Capacity is people-based, not inventory-based; FIFO only on lower plans; 5 events per room; 20 s refresh is coarse; no invite-only/identity features out of the box (build with Workers); bot quality depends on Turnstile/Bot Management.

**Sources.** developers.cloudflare.com/waiting-room/about, /reference/configuration-settings, /reference/queueing-methods, /additional-options/create-events [seen]; blog.cloudflare.com/how-waiting-room-queues, /building-waiting-room-on-workers-and-durable-objects [source].

---

## 9. Cross-vendor feature matrix (festival/resort use cases)

| Use case | Best example | Notes for NIBIRU |
|---|---|---|
| Registration / lottery on-sale | Tomorrowland pre-registration; Glastonbury registration; Queue-it/Cloudflare pre-queue shuffle | Account + pre-registration window weeks ahead |
| Verified fans / invite-only | Queue-it invite-only (signed links, 2FA, single-use); First 20 Sale per country | Loyalty tiers: past attendees, resort guests, locals |
| Queue + anti-bot | Queue-it + Akamai/Netacea; AWS WAF Bot Control (Tomorrowland); Turnstile infinite queue | Hide detection until sale start |
| Travel / accommodation packages | Tomorrowland Global Journey + DreamVille; Dour camping/hotels; ADE hotels with revenue share | Camping, hotel, bus/flight, padel slots, tables |
| Payment plans | Glastonbury £75 deposit; Tomorrowland €2,500 deposit + 3 instalments ≥ €3,000 | Deposit lowers the peak on the payment gateway |
| Group / friends | Lead booker for 6 (Glastonbury); Friendship packages; Paylogic Send 2 Friend | Group invite, each member personalises |
| Cashless top-up / refund | Weezevent (15% unspent), Intellitix auto top-up, Pearls | Convert leftover into next-season credit |
| RFID access / re-entry / zones | Intellitix pass-back + exit logging; Weezevent intra-site zones | Resort = many zones over 74 days |
| Upgrades | Paylogic VIP upgrade upsell; Tomorrowland says no upgrades after purchase (gap) | Opportunity |
| Official resale + re-personalisation | Weezevent new barcode; Tomorrowland Exchange Desk → Waiting List Sale; AXS 10% cap | Legal context: Belgium bans profit resale; check Romanian law |
| Organiser dashboards | Paylogic multi-event compare; Intellitix live; Queue-it live bot dashboards | Multi-organiser platform needs per-tenant views |
| Real-time attendance / crowd | Weezevent check-in counts per zone; Intellitix entry/exit, congestion | Feed into app and safety ops |
| Sponsorship / activation data | Intellitix product-by-demographic, giveaway redemption; Weezevent partner activations | Sponsor dashboards, tap-to-redeem |
| APIs | Queue-it/Cloudflare fully public; ticketing vendors closed | Public API is a differentiator for "open to any organiser" |

---

## 10. Lessons for a 100k-people-in-10-minutes on-sale

**Size the problem first (numbers).**
- 100k buyers in 10 min = **~170 completed orders/s** if everyone buys; assume 3 to 10x that in requests per order (seat/ticket pick, add-ons, payment, 3-DS) → 500 to 2,000 dynamic req/s at the core, but **10 to 50x more on the front door**: Rock in Rio saw ~25k arrivals/min, Glastonbury ~2.5M people for 210k places; one concert had 96% bot requests. Design the edge for **1M+ concurrent sessions and ~50k new arrivals/s at T0**, the core for a controlled ~200 to 500 checkouts/s.
- The real bottlenecks are the inventory DB row locks and the payment provider (3-DS, issuer limits), not web servers (Megatix: "database reaches its limits long before" load-test numbers).

**Pattern 1: identity before demand.** Account + pre-registration window (Tomorrowland: ~10 weeks), with e-mail/phone verification, country, and optional ID. Enforce **caps per account cumulative across sale moments** (4/6 people) and **one queue place per verified account** (Queue-it "one place in line per e-mail/account"). This turns the bot problem into an account-farm problem, handled earlier and slower.

**Pattern 2: spread demand over several sale moments.** Packages first (most committed, highest basket), then presale for past attendees / resort guests / locals, then general sale, then waiting-list resale. Add a small **"First N per country"** or per-segment allotment so every market gets a fair shot. Glastonbury does coach + ticket three days before GA.

**Pattern 3: an edge waiting room with a randomised pre-queue.**
- Pre-queue opens ≥ 30 to 60 min before (AXS ~30 min; Cloudflare minimum 5 min); at T0 **shuffle** everyone present, then FIFO for late arrivals (Queue-it, Cloudflare `shuffle_at_event_start`, Glastonbury 2025).
- Serve the room as static HTML/JSON from the CDN; position polled (Cloudflare 20 s) or pushed; position bound to a signed cookie/token so tab closes and reconnects don't lose it; advise "one tab, one device"; **e-mail/push when it's your turn** so people don't babysit.
- Admission via **signed short-lived token** (HMAC: queue ID + expiry + account ID + event ID, optionally IP), validated at the edge on every dynamic request, stripped from URLs, single-use, **revoked after purchase** (Cloudflare `Cf-Waiting-Room-Command: revoke`, Queue-it "end session validity after purchase").
- Outflow is a dial, not a constant: start at a safe rate (e.g. 300 to 500 admissions/min per backend unit), raise it while watching DB p95 and payment error rate (Megatix, Ingresso). Cap active sessions to ~75% of measured capacity (Cloudflare guidance).
- Keep a **static fallback switch** for every public page (Tomorrowland's `fallback.json` pattern) and a reject/maintenance mode.

**Pattern 4: bot defence in layers, invisible until T0.** WAF + bot management at the edge (AWS WAF Bot Control, Turnstile, Akamai/Netacea), challenge before entering the pre-queue, **apply blocks only after the shuffle** so bots can't retool, send suspects to the back or to an infinite queue rather than a clear error; alerts for many places per IP/ASN/data centre; device and payment-card velocity limits at checkout; personalised, non-transferable tickets make bot purchases worthless.

**Pattern 5: inventory as the source of truth, not the session.**
- Pre-split inventory into **buckets/shards** (per ticket type × N shards) with atomic decrement (Redis Lua / Postgres `UPDATE … WHERE remaining >= n RETURNING`), never a single hot row.
- Reservation with TTL (Glastonbury: 10 min details page + payment timer); idempotency key per checkout; release expired holds back automatically; show SOLD OUT only when holds are drained (Glastonbury does this).
- Write orders to a durable queue/log first (Kafka/SQS/pg-boss style), confirm asynchronously, send e-tickets and personalisation links later. Separate read path (catalogue, line-up, static) entirely onto the CDN.

**Pattern 6: take the payment peak off the critical path.** Deposits (Glastonbury £75; Tomorrowland €2,500 + 3 instalments) or "reserve now, pay within 24 h" (Glastonbury resale) cut gateway load and widen access; pre-tokenise cards during pre-registration; use two acquirers with failover; decide policy for failed payments up front (Tomorrowland: not reprocessed, application rejected; Glastonbury: released back to sale).

**Pattern 7: post-sale fairness keeps the market clean.** Mandatory personalisation with deadline, name = ID at gate, official resale only through a **waiting list** with barcode reissue (Weezevent, Tomorrowland Exchange Desk), price cap (AXS +10%) or face value, small fee (€10), refunds to original payment method.

**Pattern 8: communicate inside the queue.** Messages to the room (Paylogic, Queue-it), honest "sold out" as early as possible, polls/sign-ups to capture zero-party data while people wait, a status page. Reported effect: 40% fewer support requests (Megatix).

**Pattern 9: rehearse and observe.** Load tests lie (Megatix); run a real smaller on-sale through the same path first, have live dashboards for arrivals/min, outflow, holds, DB p95, payment success, bot classifications; a runbook with dial positions; feature flags and kill switches (Tomorrowland env flags).

**Pattern 10 (resort-specific).** Over 74 days the same stack runs many small on-sales (padel slots, tables, headliner nights) and a few huge ones: use an always-on 24/7 room that activates on thresholds (Queue-it 24/7, Cloudflare passthrough → event), and one account/wristband that links ticket, accommodation, cashless wallet and zone rights (Weezevent/Intellitix model), with leftover cashless balance turned into season credit (Weezevent: ~15% left unspent).
