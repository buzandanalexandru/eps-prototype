# Romanian ticketing market: competitor research

Researched 27 Nov 2026 for the NIBIRU / EPS CTO interview.

**Method.** Playwright crawls checked robots.txt first, loaded one page at a time and went no further than the public ticket picker. I did not log in, open a cart or checkout, or pay. bilete.beach-please.ro disallows `/cart` and `/checkout`, so those pages were not crawled. The raw output (JSON per page with text, buttons, XHR/fetch calls and script hosts, plus screenshots) is in `raw/romania/`. The scripts are in `scripts/romania/` (`crawl.js`, `seatmap.js`, `show.js`, `kw.js`).

**Tags.** **[seen]** means I saw it on the site during the crawl. **[source]** means it comes from public press or docs (URL given).

**Blocked or skipped:**
- **eventim.ro:** every request failed (HTTP/2 stream reset through the proxy; 502 in Playwright; 503 through WebFetch). The site appears to use bot protection, so I moved on.
- **iabilet seat-map step:** "Alege locuri" sits behind a Cloudflare Turnstile ("Nu sunt robot!" / "Doar o mică verificare"). I did not try to get past it.
- **iabilet FAQ** (`/faq/`) redirects to a Cloudflare Access login for the staff knowledge base.
- **bilete.ro:** robots.txt disallows ClaudeBot and anthropic-ai. I did not crawl it and used entertix.ro (same group) and public sources instead.
- **neversea.com** showed a maintenance countdown ("back online very soon").

---

## 1. iaBilet.ro, the market leader

Facts at a glance:
- **Operator:** iaBilet (founded 2012).
- **Claims [seen]** on https://www.iabilet.ro/servicii/: 4.4k B2B clients, 4M tickets a year, 4M monthly unique visitors, 1M newsletter subscribers, 600k app installs, "2.422 evenimente din 159 orașe" (homepage).

**Discovery [seen]**
- Browsing by city (159 cities), venue, artist and style (about 30 genre pages), plus Teatru, Festivaluri, StandUp, Sport, Turnee and Expo & Muzee.
- "Promovat" (paid) slots, "Cele mai vândute", recommendations, favourites with reminders, "Follow" on artists, a newsletter and dark mode.
- An accessibility widget (large cursor, font size).

**Ticket types [seen]**
- **UNTOLD:** price phases ("Phase 3/4/5"), Under-25, a "Risk Free" tier (refundable up to 30 days before, but the Risk Free fee is kept), and a "DALE Duo Pack" (two passes plus a merch kit).
- **Feli at Sala Palatului:** Early Bird and Under-23 tiers, and free tickets for people with disabilities plus a companion.
- **Theatre:** "2 + 2 Gratis" bundles.
- **Other:** voucher codes and gift vouchers.
- **Struck-through prices:** on UNTOLD the strikethrough is "prețul final estimat al abonamentului la momentul festivalului" (the estimated final price), a dynamic-phase anchor.

**Seat maps [seen]** "Hartă locuri" / "Alege locuri" on Ateneul Român and Sala Palatului events, behind Turnstile.

**Payments.** Source: [seen] https://www.iabilet.ro/site-ul-tau/ and the T&C at https://pages.iabilet.ro/policies/iabilet/termeni-si-conditii/ro-RO.
- **20 methods:** card, Apple Pay, Google Pay, PayPal (also the route for Amex cards), RevolutPay, instalments ("rate fără dobândă" banner on every event, with a per-month price under each tier), culture and meal cards (Edenred, Up, Pluxee), bank transfer, and cash on delivery by Fan Courier.
- **Carrier billing:** charge to a Vodafone or Orange phone bill, or Orange prepaid credit (max €30 per transaction, €100 or €200 a month).
- **About 2,000 physical points:** Flanco stores, about 1,000 to 1,500 SelfPay kiosks, Hard Rock Cafe and others ([seen] https://www.iabilet.ro/puncte-de-vanzare/).
- **Processors** named in the T&C: LibraPay and PayU. One-click "plăți rapide" uses a stored card plus CVV only.

**Fees and price display [seen, T&C]**
- A separate "taxa de operare platformă online" is "listată separat în sumarul de plată", so prices are **not all-in** on the event page.
- After redirect to the processor, the buyer has 10 minutes to pay.
- "Asigurare Flex" (ticket insurance) is an upsell.

**Resale [seen]** dăBilet.ro (https://dabilet.ro/ro) is iaBilet's own official resale:
- Listings are checked in real time against the primary system.
- The buyer gets a **new QR code**.
- The seller is paid within 72 hours.
- There is a **price cap tied to the original price**.
- The T&C says resale is allowed only through dabilet.ro, and only where the organiser permits it.

**Membership [seen]** iaBilet Elite (https://elite.iabilet.ro/) costs 5 RON a month or 50 RON a year:
- 1% cashback into a Wallet, and presale prices.
- Tickets by SMS free; −10% on the operating commission and on Flex insurance.
- Full refund where the organiser allows it, and a profile badge.
- 12 months free after 50 tickets a year.

**Delivery [seen, T&C]** PDF e-ticket, mobile ticket in the app (iOS, Android and Huawei AppGallery), SMS, or a physical ticket by courier.

**Organiser tools.** Source: [seen] /servicii/ and /site-ul-tau/.
- **White-label "propriul tău site de bilete"**, free with a commission only. **tickets.untold.com is one of these** (it loads static.iabilet.ro assets). Beach, Please! 2025 also ran on tickets.beach-please.ro on iaBilet ([source] https://tickets.beach-please.ro/bilete-beach-please-festival-2025-99644).
- Real-time sales dashboards, seat-map mapping done by iaBilet, and the ability to block seats.
- Vouchers, zero-value tickets with data capture, and reservations with later payment.
- Time-slot tickets (eXibit, for museums), a free iOS/Android scanner app with online validation, weekly payouts, and local-tax paperwork.
- Google Tag Manager pixels.
- **Spotlight** (https://spotlight.iabilet.ro/) is self-serve promotion: Meta and Google ads with iaBilet first-party lookalike audiences ("AI audience matching"), targeted newsletters from 0.06 RON per email, homepage slots, push, and event services (security, staff, logistics) bought through the platform. This is the most advanced organiser growth tool in the market.

**Anti-bot and queue**
- **[seen]** Cloudflare Turnstile before seat selection or ordering; the site sits behind Cloudflare (RUM beacon).
- **[source]** A virtual queue for big on-sales: about 140k people queued for Metallica 2026, and more than 100k for Coldplay 2024.

**On-sale incident [source]** Metallica pre-sale, 28 May 2025:
- ANPC registered 108 complaints (and counting).
- Complaints: pre-sale holders could not buy; only tickets from 1,400 lei remained; the app was intermittently down and showed different prices for the same type; queue numbers were refused as "already used"; resellers were active; and a "fals sentiment de urgență" because stock was not shown.
- Sources: https://www.digi24.ro/digieconomic/consumer/anpc-sesizeaza-nereguli-in-procedura-de-achizitie-a-biletelor-pentru-concertul-metallica-evenimentul-va-avea-loc-in-2026-56161, https://economedia.ro/anpc-face-verificari-dupa-cozile-virtuale-uriase-la-bilete-pentru-concertul-metallica.html, https://zilesinopti.ro/2026/04/28/bilete-metallica-bucuresti-2026-sold-out/, https://www.antena3.ro/actualitate/inedit/val-glume-ironii-romanii-coada-bilete-concerte-coldplay-bucuresti-681255.html

**Stack [seen]** Server-rendered PHP-style pages (`/site/event/dabilet.tickets?id=`), jQuery 2.1.1, Bootstrap, underscore and typeahead, behind Cloudflare.

**Festivals sold [seen]** UNTOLD, Electric Castle, Beach, Please! 2027 (GA shown as "179 EUR" and 976 lei), Rockstadt and more. A footer link reads "Bilete Nibiru".

## 2. Entertix.ro, with myticket.ro and bilete.ro (one group)

- **Group [seen]:** myticket.ro now redirects to entertix.ro. The entertix footer lists the group's sites:
  - bilete.ro, shop.bilete.ro and entertix.bilete.ro;
  - white-label shops such as bilete.frf.ro (the Romanian Football Federation), bilete.operacomica.ro, tickets.minamuseum.com and bilete.emagic.ro;
  - sister sites in the Baltics: piletilevi.ee, bilesuserviss.lv and bilietai.lt.
- **Operator:** PLG ROMANIA SRL.
- **Discovery [seen]:** tags (Teatru, Concert, Sport, Festivaluri, Circ, Copii), "Trends", venue and organiser pages (`/g/<id>`), gift vouchers and OneSignal web push.
- **Ticket picker [seen]** (`/bilete/<id>/…`): a quantity stepper per category and a discount-code box.
  - "*pretul afisat nu include taxele de servicii", so prices are **not all-in**.
  - A pre-registration product type (Thirty Seconds to Mars).
- **Seat maps [seen]:** an interactive SVG plan with zoom and pan, click to add a seat, and category legends (Sala Palatului, BT Arena). Fed by `POST do=getmapdata` and `do=getcart`, a simple first-party PHP handler.
- **Payments [seen]** (https://www.entertix.ro/metode-de-plata.html):
  - card through **NETOPIA mobilPay**, and bank transfer;
  - **3 interest-free instalments** with partner credit cards: Card Avantaj, **Star BT (Banca Transilvania)**, Bonus Card (Garanti), Alpha Card and BRD Finance;
  - cash or card at Myticket points (a 72-hour reservation), at Cărturești bookshops and at Carrefour ([seen] /magazine-partenere.html).
  - **[source]** bilete.ro has run 4-instalment plans (https://www.bilete.ro/media/25794/saga-festival-2021-termeni-si-conditii-plata-rate-ro-en.pdf).
- **Refund insurance [seen]:** "Caretix" costs 5% of the ticket value and allows withdrawal up to 10 days before, with 100% of the ticket price back (fees excluded). The buyer must actively choose Yes or No at checkout.
- **Delivery [seen]:** print-at-home e-ticket ("nu este suficient să prezentați email-ul") or courier.
- **Organiser services [seen]:** consultancy (pricing, allocations, on-sale), basic marketing, access control and box office, printing, real-time reports, fast settlement and local-tax filings. This is a managed service; there is no self-service portal on show.
- **Not seen:** resale, wallet passes, an app or a queue.
- **Electric Castle** is sold here in lei, e.g. GA at 1,715 lei plus fees.

## 3. Eventim.ro (CTS Eventim), blocked

- I could not crawl it: the TLS/HTTP2 stream was reset, so it is likely behind bot protection.
- **[source]** Revolut Pay was added as a one-click checkout on eventim.ro and in its app in April 2026. The company claims more than 1M tickets a year in Romania. https://zilesinopti.ro/?p=830468, https://www.revistabiz.ro/eventim-alege-revolut-pay-pentru-solutiile-de-plata-a-biletelor-la-concerte/
- In other markets the group runs EVENTIM.Pass (in-app-only, scalper-proof tickets) and fanSALE resale (https://www.eventim.de/campaign/en/pass/). Whether either exists in Romania is **not confirmed**.

## 4. Bilete.beach-please.ro, Beach, Please! (EPS, NIBIRU's sister shop)

**Ticket types [seen]**
- 5-day passes: GA, GA Plus (fast lane), Golden Circle, VIP, Ultra VIP and Ultra VIP + Golden Circle. The 2027 range runs from €179 to €559, each with a struck-through "value" price.
- One-day passes, and "Fă-ți upgrade acum din contul tău" (upgrades from the account).
- Camping add-ons (operated by LUN.R CAMP): Camping Pass €49, pre-pitched tent, glamping, cabins, containers, tiny houses (by Viewbox Houses) and a campervan pass.
- **Hotels in the same shop:** "Cazări în Costinești cu reducere de 50 lei cu bilet", 5-night packages.
  - The data comes from `/api/accommodations`: valiza.ro / touringit hotel listings.
- **Bundles:** this is the only Romanian shop seen that sells festival, camping and hotels in one basket.

**Instalments [seen]**
- "sau rată de la 22.38 EUR/lună", up to **8 instalments**. The last payment date is 21 Jun 2027, and the instalment fee is 0 (from the product data).
- The beach-please.ro banner reads "PAYMENT IN INSTALLMENTS AVAILABLE".

**Payments [seen]** The footer reads "Ticketing oferit de Event Platform Systems · Securizat de Banca Transilvania", i.e. **BT iPay**. The GraphQL schema also carries the Magento possible-types `ApplePayConfig` and `GooglePayConfig`, but these may be schema-only.

**Named tickets [seen]** (https://beach-please.ro/faq):
- "Every Beach, Please! ticket is issued in the name of one person and can't be transferred."
- No name changes. Wristbands are named too.
- **No official resale.** "Organizatorul nu permite returul biletelor."

**Cashless [seen]** (https://beach-please.ro/refund-policy):
- Cashless wristband with online top-up from the account ("Wristbands, Top-up, Transactions"; `/account/cashless` route).
- On-site refunds at Top-Up Points (cash capped at 1,500 RON for card top-ups).
- Online refunds of the online-topped amount between 13 and 18 Jul, paid by bank transfer by 15 Aug.

**App [seen]** "GET THE APP" (the beach-please.ro footer; assets on beachpleaseapp.b-cdn.net).

**Distribution [source]**
- eMAG partnership: Black Friday drops ("100 tickets at 10 lei sold out in the first second"; more than 10,000 passes in 72 hours), eMAG-exclusive 50%-off campaigns, and a dedicated fast-entry gate for eMAG Genius members.
- Sources: https://www.g4media.ro/beach-please-face-show-total-de-black-friday-10-000-de-bilete-vandute-si-primul-headliner-anuntat-pentru-2026-don-toliver.html, https://zilesinopti.ro/2026/03/31/bilete-beach-please-50-reducere-emag/, https://startupcafe.ro/selly-emag-proiecte-nibiru-beach-please-beneficii-abonati-genius-99563
- Beach, Please! is also listed on iaBilet (GA at 976 lei).

**On-sales in practice [source]**
- No reported crash in the sources found. Black Friday drops sell out in seconds.
- 2026 drew about 600k participations over 5 days. Early wristband pickup points around the country were used to avoid entry queues.
- https://www.g4media.ro/beach-please-intra-in-istorie-peste-600-000-de-participanti-cea-mai-mare-editie-de-pana-acum-si-inceputul-unei-noi-ere-la-nibiru.html, https://www.gandul.ro/actualitate/ce-este-strict-interzis-sa-aduci-in-incinta-festivalului-beach-please-regulamentul-oficial-de-acces-20942298

### Stack: same EPS as nibiru.net? Yes for the backend; the frontend is a different generation.

| | bilete.beach-please.ro | nibiru.net |
|---|---|---|
| Frontend | Next.js **Pages Router** on **GraphCommerce** (`@graphcommerce/graphql`, Apollo, MUI); `_next/data/<build>/ro.json` SSG [seen] | Next.js **App Router** (RSC `?_rsc=` requests), `/api/time`, `/api/csrf`, `/api/auth/session`, Cloudflare challenge script, Cookiebot, Contentsquare, Clarity [seen] |
| API path | Apollo link `uri:"/api/graphql"` (Next proxy) to a Magento backend at **account.beach-please.ro** (Magento sitemaps, `magentoStoreCode` ro/en, `x-magento-cache-id`) [seen] | `/api/eps` proxy (per brief; not hit on anonymous pages) |
| Schema | Custom **`eps_` namespace**: `eps_cart`, `eps_addProductsToCart`, `eps_removeItemFromCart`, `eps_customerCart`, `eps_RefundableCartItemInput`; products typed **`NibiruExternalProduct`** [seen] | same EPS |
| Shared catalogue | The BP store's SSG data contains **Nibiru Beer Garden products** (e.g. `BG_ADRIANNOROCEL_1908_CATA`) next to `BEACHPLEASE_2027_*` SKUs, so it is one Magento catalogue across stores [seen] | |
| Product fields | `fee_type`, `fee_amount` (fixed 100 on BP 2027 passes, currency not shown), `installments_enabled`, `installments_max_count` (8), `installments_last_payment_date`, `installments_fee_type` / `installments_fee_amount`, `allow_personalization`, `checkin_deadline`, `checkin_done`, `can_ask_for_refund`, `was_upgraded`, `upgrade_order_id`, `upgrade_price`, `upgrade_code`, `available_payment_methods` [seen] | same feature set (per brief: instalments, personalisation, online check-in) |
| Service SKUs | `TICKET-UPGRADE` (fee 30%), `personalization-processing-services`, `checkin-processing-services` [seen] | |
| Account routes | `/account/installments`, `/account/installments/[planId]`, `/installments/success`, `/account/cashless` (wristband register, top-up, balance, history) [seen] | |
| Payments | BT ("Securizat de Banca Transilvania") [seen] | BT iPay plus Netopia (per brief) |

**Conclusion.** Beach, Please! runs on the same EPS Magento-2 GraphQL backend, with the same catalogue and type names (`NibiruExternalProduct`) and the same instalment, personalisation, check-in and upgrade model. Only the storefront differs: an older GraphCommerce / Pages Router PWA with `/api/graphql`, against nibiru.net's newer App Router with `/api/eps`. **EPS already runs multiple tenants and stores today (NIBIRU plus Beach, Please!), which is the seed of a multi-organiser platform.** The gaps are self-serve onboarding, seat maps, resale and wallet passes.

## 5. UNTOLD (untold.com, tickets.untold.com) and Neversea / Kapital

**Two systems**
- **[seen]** **tickets.untold.com is a white-label iaBilet shop** (static.iabilet.ro assets, Cloudflare). It sells GA phases, U25, Risk Free, the DALE Duo Pack and "Galaxy Ba…" tiers.
- **[seen]** **untold.com is a Next.js site** "Powered by **Rhuna** – Festival Ready Infrastructure".
  - Calls: `api.b2b.extasy.com/web/festivals/accepted-locale`, `untold.com/api/tickets/get-bag` and `/api/get-festival`.
  - It holds the account: **My Wallet** (tickets, online check-in, top-up), **My Balance**, `/account/gamification`, and an "Official Accommodation" shop from 806 lei.
  - reCAPTCHA, HubSpot, Hotjar and Clarity are loaded.

**Rhuna [source]** A Romanian white-label platform (payments, ticketing, access control, data). The 2024 launch talked of NFT tickets, and it has piloted walk-through facial entry at UNTOLD. https://rhuna.io/rhuna-for-organisers, https://chainwire.org/2024/02/19/rhuna-launches-to-revolutionize-the-events-and-entertainment-industry-with-fintech-innovation/, https://www.securityworldmarket.com/uk/News/business-news/pioneering-walk-through-facial-entry-at-top-music-festival

**Rhuna in the T&C [seen]** (untold.com/info/7b60b18a…): "RHUNA" is defined as a multi-purpose value coupon across the group (Untold RO, Untold Dubai, Neversea, Massif). The account balance converts to Rhuna, and loyalty "Rewards" can be spent there too.

**Ticket rules [seen]**
- Tickets can be non-nominal or nominal.
- **Online check-in** in My Wallet is free for 30 days after purchase, then €15 online or €25 at the gate.
- Wristband pickup at **BT branches** and malls before the festival.
- Instalments, with the count set by time left to the event. Instalment products **cannot be transferred**, and check-in or QR codes stay locked until fully paid.
- The displayed price may bundle "Base Price + System Fees + Top-up amounts + Premium Services + VAT", so it is **partly all-in**.
- **Lifetime Pass** (25 future editions) and **One for all Pass** products (T&C annexes).
- Accommodation and packages through partners.

**Cashless [seen]**
- The wristband is the wallet: top-up online, in the app or at Credit Points (cash or card).
- Online top-ups must be activated before use.
- Refunds online through the account or app in a set window, or on site.

**App [seen]** UNTOLD app (iOS id1019269879): "Manage your passes".

**Partners and payments [source]**
- Banca Transilvania co-branded "BT Visa UNTOLD" card: https://www.bancatransilvania.ro/news/en/white-papers/recordurile-cardului-bt-visa-untold
- Stripe for UNTOLD Dubai's international sales: https://stripe.com/en-ch/customers/untold

**On-sales [source]**
- Passes sell out in minutes (5k in 5 minutes in 2015; 15k in under 3 minutes in 2019; day tickets in minutes in 2023).
- Priority for people registered on untold.com, waitlists after sell-out, and repeated resale **scams** on social media (no official resale).
- https://www.bugetul.ro/untold-a-dat-lovitura-biletele-s-au-epuizat-in-3-minute-organizatorii-pregatesc-insa-o-mare-surpriza/, https://alba24.ro/untold-2023-este-sold-out-ce-solutii-ofera-organizatorii-pentru-cei-care-nu-au-prins-bilet-cum-se-face-check-in-online-989721.html, https://www.antena3.ro/actualitate/noua-schema-inselaciune-online-bilete-untold-648115.html

**Neversea [seen]** The site was in maintenance mode.
- **[source]** Neversea paused Constanța in 2025, then ran as Neversea Kapital or **Kapital Festival** at Arena Națională, Bucharest (3 to 5 Jul 2026; kapitalfestival.com), on the same model (€25 on-site check-in, online top-up and refunds).
- https://www.romania-insider.com/life/neversea-festival-romania-break-2025, https://zilesinopti.ro/2026/05/20/festivaluri-litoral-2026/, https://www.bucharest.ro/news/first-edition-of-neversea-kapital-three-days-at-the-national-arena-recommendations-payment-system-and-access-info-64

## 6. Electric Castle (electriccastle.ro)

**Ticket types [seen]** (https://electriccastle.ro/tickets):
- GA €320, "GA + Insurance" €339, Youth 25 €139, VIP €499 (plus an insurance variant), Ultra VIP €699, and the Black Ticket.
- Camping Pass €69, and accommodation: pre-pitched tent €109 up to a Panorama Tent €690, shown per night too.
- Merchandise in the same cart.
- **"Billboard codes"** gamified promo: "74 BILLBOARD CODES LEFT", codes found on outdoor billboards.
- A Black and Yellow Friday countdown.

**Fees [seen]** "**+ Booking fee 8%**" is shown under every price. That is transparent but **not all-in**.

**Named tickets [seen]** (https://electriccastle.ro/tickets-info):
- Name and address are printed and checked against ID at the wristband swap.
- Name changes cost a fee (typos are fixed free by email).
- **"The only possible way to transfer or sell a ticket to someone else is from your account, My Orders section and choosing List to Marketplace"**: an **official fan-to-fan exchange**. A U25 ticket can only go to a buyer who is also eligible.

**Instalments [seen]** "Split your order into monthly payments (if available)". Upgrades from My Orders (any pass to a higher one).
- **[source]** Earlier editions ran 3-instalment plans with an 8% commission (https://lawinsider.com/ro/contracts/cdGThS1ye4H).

**Cashless [seen]** Wristband top-ups ("do not expect to use cash inside"). The app has "MANAGE YOUR WRISTBAND CREDIT".

**App [seen]** (https://electriccastle.ro/ec-app): own schedule, stages, artist discovery, notifications, **personalised aftermovies**, map and vendors.

**Transport and VIP [seen, source]**
- VIP and Ultra VIP shuttles; the Black Ticket adds a 5-star hotel and a private shuttle.
- **[source]** A festival train with Romanian Railways (CFR), for which you need a train ticket plus a festival ticket (https://electriccastle.ro/fyi), and recurring shuttle-queue complaints in 2022 (https://actualdecluj.ro/electric-castle-ziua-3-toate-drumurile-au-dus-la-coada-participant-am-stat-mai-mult-la-coada-decat-in-festival-val-de-ironii-pentru-proasta-organizare/).

**Innovation [source]** Wristbands delivered to your door by **Wolt** ("Skip the line", 2026, Bucharest and Cluj, up to 5 per order; a €50 exchange fee if you turn up without one). https://spotmedia.ro/stiri/timp-liber/wolt-si-electric-castle-livreaza-bratarile-de-acces-in-festival-direct-la-usile-festivalierilor

**Distribution [seen]** The festival's own shop plus iaBilet and Entertix. It also runs partner locations (TED's Coffee and others) as community hubs.

**Stack [seen]**
- jQuery front end with its own bundle.
- Catalogue API on an obfuscated subdomain: `a7f39c2e81bd460f952ac603.electriccastle.ro/productsv2?category_id=22&limit=150&store=1`, Magento-like store and category IDs.
- Behind Cloudflare, with Cookiebot.
- No queue or anti-bot widget seen on the ticket page.

## 7. Other players

**Ticketstore.ro (Ticketstore Group) [seen]**
- Sites: Ticketstore.ro, Biletlateatru.ro, Dynamicticket.ro and Startickets.ro.
- Operator: SMART TICKETING & LOGISTICS SRL, "cea mai mare platformă de ticketing 100% românească".
- Focus on theatre, operetta, ballet and tours. Box offices at the Operetta theatre.
- **Seat maps** with an available count ("2046 locuri disponibile", "Locurile gri sunt ocupate") via `POST cdo=getseatingplan` and `cdo=getcart`.
- Pupil and student discounts, and price-rise notices ("Prețurile vor fi majorate începând cu 15 decembrie").
- WhatsApp and Facebook share buttons, and a server-side GTM (gtm-cloud.ticketstore.ro).

**LiveTickets.ro [seen]** The self-service platform ("Inscrie-te acum… vinde bilete online la evenimentele tale").
- Angular single-page app with a REST API at api.livetickets.ro (`/public/organizer/getbyurl?url=…`, so it runs **multiple tenants on custom domains**; `/public/events/get-tickets`).
- **Stripe** (js.stripe.com).
- Multi-currency (`/public/currencies`) and a language/currency switcher.
- Follow organisers, Help Scout chat, and a vendor stack at flowconcept.io (SignalR notifications, translations).
- Club, party and NYE long tail; table reservations by WhatsApp with a private payment link.
- The closest local analogue to a "Shopify for events".

**dăBilet.ro** is the iaBilet resale site (see section 1).

**Other names (not crawled):** iTicket (Moldova plus large Romanian events) and 2nite (app). Seen in app-store results only.

---

## Market summary table

| Site | All-in price shown | Instalments | Seat maps | Resale / transfer | Wallet passes | Cashless | App | Queue / anti-bot | Payment processors | Organiser self-service |
|---|---|---|---|---|---|---|---|---|---|---|
| iaBilet | No: operating fee added in summary [seen T&C] | Yes, "rate fără dobândă" per tier [seen] | Yes, behind Turnstile [seen] | **dăBilet official resale, price-capped, re-issued QR** [seen] | Not seen (app plus SMS tickets) | No (scanner app only) | Yes, 600k installs [seen] | Cloudflare Turnstile [seen]; virtual queue (Metallica, Coldplay) [source] | LibraPay, PayU, PayPal, Apple/Google Pay, RevolutPay, Vodafone/Orange billing, meal cards, Flanco/SelfPay kiosks [seen] | Partly: white-label site, dashboards, scanner app, **Spotlight self-serve ads**; events set up by iaBilet staff [seen] |
| Entertix / myticket / bilete.ro | No: "nu include taxele de servicii" [seen] | Yes, 3 rates with partner cards incl. Star BT [seen] | Yes, interactive SVG [seen] | No (Caretix 5% refund insurance) [seen] | No | No | Not seen | None seen | NETOPIA mobilPay, bank transfer, cash at Myticket, Cărturești, Carrefour [seen] | No: managed service; white-label shops for venues [seen] |
| Eventim.ro | ? (blocked) | ? | ? | fanSALE / EVENTIM.Pass elsewhere, RO unconfirmed [source] | ? | No | Yes [source] | Bot protection blocked crawl [seen] | Revolut Pay added 2026 [source] | ? |
| bilete.beach-please.ro (EPS) | Unclear: separate `fee_amount` field per product; cart not crawled (robots) [seen] | **Yes, up to 8, 0% fee** [seen] | No | **Named, non-transferable, no resale**; upgrades [seen] | Not seen | **Yes: wristband, online top-up and refund** [seen] | Yes [seen] | None seen on shop (Cloudflare on nibiru.net) | **BT (iPay)** [seen]; Netopia on nibiru.net (brief) | No public onboarding (EPS multi-store backend) |
| UNTOLD (tickets.untold.com on iaBilet plus Rhuna untold.com) | Partly: T&C says price may include system fees and top-up [seen] | Yes, count by time left; not transferable until paid [seen] | No | Nominal or non-nominal; **no official resale**, scams reported [seen/source] | Not seen | **Yes: Rhuna wallet, top-up, refunds** [seen] | Yes [seen] | reCAPTCHA on untold.com [seen]; registration priority, waitlists [source] | iaBilet stack; BT co-brand card; Stripe for Dubai [source] | Rhuna is sold as white-label infrastructure [source] |
| Electric Castle | No: "+ Booking fee 8%" shown under the price [seen] | Yes, "if available" [seen] | No | **Official "List to Marketplace"** (only legal transfer) [seen] | Not seen | Yes, wristband credit in the app [seen] | Yes, rich [seen] | None seen | Own shop (processor not visible) plus iaBilet and Entertix [seen] | No |
| Ticketstore group | Not determined | Not seen | Yes [seen] | No | No | No | No | None seen | Not determined | No (managed) |
| LiveTickets | Not determined | Not seen | Not seen | No | No | No (events may be card-only) | No | None seen | **Stripe** [seen] | **Yes: sign-up self-service, multi-tenant domains** [seen] |

---

## What is standard, what nobody does, what it means for EPS

**Standard in Romania**
- **Instalments.** Interest-free bank-card plans (Star BT and the like) or the platform's own monthly plans are expected for festivals and big shows.
- **Price phases, U25 and student tiers, and VIP ladders** (GA, GA Plus or fast lane, Golden Circle, VIP, Ultra VIP).
- **Named, non-transferable festival tickets**, swapped for an RFID wristband after online check-in. The check-in itself is often charged after a free window.
- **Closed-loop cashless wristbands** with online top-up and windowed refunds.
- **Camping and accommodation upsells.**
- **Fees added on top.** An 8% booking fee or an "operating fee" is the norm, not all-in pricing.
- **Offline cash channels** (kiosks, retail chains, carrier billing) are distinctive to Romania. iaBilet's 20 payment methods set the bar.
- **Distribution partnerships:** eMAG, Wolt, BT branches.

**What nobody does yet (or does poorly)**
1. **All-in pricing by default.** Every site shows fees separately, so this would be a consumer-trust and EU price-transparency differentiator.
2. **Wallet passes** (Apple/Google Wallet with rotating or dynamic QR). Not seen anywhere.
3. **Official resale at festivals that are also named and price-capped.** Only EC (a marketplace) and iaBilet (dăBilet) offer it. UNTOLD and Beach, Please! have none, which feeds scams.
4. **Fair on-sale tech.** Virtual queues exist (iaBilet), but the Metallica on-sale produced 108 ANPC complaints. Nobody offers transparent stock, verified-fan or lottery presales, or bot scoring beyond Turnstile and reCAPTCHA.
5. **Self-service onboarding for large organisers.** iaBilet and Entertix are staff-run; LiveTickets is self-serve but long-tail only.
6. **One basket for ticket, transport and stay.** Only Beach, Please!/EPS has hotels in the basket; UNTOLD and EC sell them in separate flows.
7. **Seat maps on the festival stacks.** None of the festival shops has them, so EPS needs them to sell theatre and arenas.

**EPS / Beach, Please! versus the market**
- **What EPS has:**
  - 8 instalments at 0%, the deepest in the market;
  - personalisation and online check-in as first-class product fields;
  - upgrades with a 30% fee SKU;
  - cashless wristband top-up and refunds;
  - camping plus hotels in one cart, and BT iPay;
  - **a multi-store Magento catalogue already shared by two brands.**
- **What EPS lacks next to the leaders:**
  - seat maps (iaBilet, Entertix, Ticketstore);
  - official resale (dăBilet, EC Marketplace);
  - alternative payments (Apple/Google Pay shown, PayPal, RevolutPay, kiosks, carrier billing);
  - wallet passes;
  - a fair-queue and anti-bot layer;
  - organiser self-service and marketing tools (iaBilet Spotlight).
- **Note:** two storefront generations (GraphCommerce Pages Router against the nibiru.net App Router) mean two frontends to maintain.
