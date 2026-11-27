# NIBIRU / EPS: teardown of the current ticketing platform

Crawled on 27 Nov 2026 with Playwright (headless Chromium, logged out, robots.txt allows everything). 87 pages were crawled: every non-article sitemap route, 45 event pages of each type, and the hidden routes found in the JS bundles. Every purchase flow was driven up to the checkout form. **No order was placed and no payment was started.** The 72 EPS GraphQL operations were extracted from the 58 JS bundles.

Raw data (in this scratchpad): `out/pages/*.json` (text, links, buttons, network calls), `out/shots/*.png` (full-page screenshots), `flow/` and `probe/` (step-by-step purchase-flow screenshots and text), `gql_ops.txt`, `probe/faq_full.txt`.

---

## 1. What NIBIRU is (the business the platform serves)

- A **74-day seasonal entertainment resort** between Costinești and Tuzla: 24 Jun to 5 Sep 2027, open daily 17:00 to 03:00. It is not one festival but a resort with many venues.
- **Venues:** NIBIRU Arena (festivals), Center Stage (free concerts on the Promenada), Berăria NIBIRU / Beer Garden (1,000 seats, powered by Ursus), The Fool (open-air stand-up amphitheatre), Hype Play (kids), Muzeul Planetelor, Parc de distracții, MINA immersive shows, padel courts, basketball, and 182 stands on the map (food, bars, shopping, beauty).
- **2027 festivals (pre-sale):** Galaxia (15 to 18 Jul, from 539 RON), Nebula X (22 to 25 Jul, 199), Rock Days (31 Jul to 1 Aug, 239), K-Pop Days (6 to 8 Aug, 439), Stardust (13 to 15 Aug, 239), Retrograde (27 to 29 Aug, 469). Claimed 2026 attendance: Galaxia 282k+, Retrograde 230k+, K-Pop 175k+, Stardust 170k+.
- **Recurring series:** Astral Nights, Beach Please! Party, CHEF, Hip-Hop Takeovers, Hot Boyz Summer, all sold via an "All Summer Bundle".
- Beach, Please! uses a **separate** ticketing site (`bilete.beach-please.ro`, referenced in the bundle).
- **"EPS" = Event Platform Systems** (event-platform-systems.ro), the ticketing operator, credited in every footer. The legal entity behind the site is Live Events Operations SRL.
- The careers page already says they are hiring for "the ticketing platform that will bring together every event in the country".

## 2. Sitemap and routes

| Area | Routes |
|---|---|
| Discovery | `/`, `/events`, `/events/<series>`, `/event/<slug>` (144 in the sitemap), `/calendar` (11 weeks, per day), `/program` (hour by hour per stage, "coming soon"), `/azi` (today), `/artist/<slug>`, search overlay |
| Places | `/location/<venue>` (6), `/places/<place>`, `/promenada`, `/harta` (interactive map, **powered by OMNIS** on OpenStreetMap, 12 categories, 182 stands) |
| Commerce | `/promenade-anyday`, `/pachete` (bundles, "coming soon"), `/presale`, `/checkout`, `/order-confirmation`, `/confirm-tickets`, `/ticket-data`, `/tickets-data` |
| Account | `/auth`, `/login`, `/profile?section=tickets…` |
| Info | `/despre`, `/intrebari-frecvente` (58 Q&As), `/cum-ajungi`, `/transport-autocar` (5 coastal shuttle routes, 20 RON), `/transport-autocar/bucuresti` (airport coaches, 150 to 180 RON, booked with an outside operator), `/cazare` |
| Other | `/noutati` + `/article/*` (39), `/cariere`, `/contact`, `/contact/business`, `/chestionar` (survey that gives 2 free Promenada tickets), press accreditation, disability-access form, parental-consent form, `/legal/*` (rules, campaign rules, cookies, returns) |
| Languages | RO by default, EN under `/en/...` with translated slugs (`/harta` → `/en/map`, `/pachete` → `/en/bundles`) |
| Apps | iOS (`id6787697821`) and Android (`com.nibiru.nibiru`) apps |

## 3. Products and pricing models seen

| Product | Example | Mechanics |
|---|---|---|
| **Festival passes** | Galaxia GA 539 / GA Plus 659 / Golden Circle 839 / VIP 939 / Ultra VIP 1,239 RON | Tiered by **zone** (Arena, Golden Circle, VIP platform, Ultra VIP) plus perks (fast lane, VIP toilets, table reservation). Struck-through "full price" (1,000 → 539). Day tickets "unavailable for now". Named tickets. |
| **Beer Garden concerts** | BDLP, Cat. C 79 / B 99 / A 149 RON | Category = **table zone** (seat guaranteed, view quality). About 40 artist-nights. |
| **Stand-up (The Fool)** | General Arena 42.5 / Front Stage 47.5 RON | Big discount display (94.99 → 42.5), **scarcity counter ("Only 24 left!")**, first-come seating, shared tables. Disability: 2 free invites per show via email. Under-16s not admitted. |
| **Center Stage / Promenada evening** | 25 RON | The ticket **is a 25 RON drink voucher** (minimum spend). |
| **Promenada access** | Pick a day 10 RON or Any-Day 15 RON, rising to 35 RON | **Time-stepped price ladder**: 10 → 11 → 12 → 13 (1 Jan) → … → 35 (24 Jul), with a "you are here" indicator and the hook "price rises in 4 days". Steps for the number of people (max 10, under-5s free) and "when are you coming?". Not named. |
| **Bundles** | All Summer Bundle | "Save up to 15%", coming soon, named, non-transferable. |
| **Camping / glamping** | `AvailableCampingStructures(dates, guests)` | A tab in every ticket picker. Empty for 2027 so far. |
| **Tables** | `AddTablesToCart`, "Rezervă masă" in the calendar | Table booking for Center Stage and the Beer Garden. |
| **Padel courts** | `AddPadelReservationToCart`, `EpsReservations(dates)` | Time-slot booking (date, start, end). The API exists but isn't shown publicly now. |
| **Upgrades** | "Ai deja bilet? Fă upgrade" on every event | `CreateTicketUpgrade(barcode, to_mapping)`, paying the difference plus a fee. Needs login. |
| **Upsells** | `GetUpsellProducts(sku)` | Magento product upsells. |
| **Accommodation cross-sell** | valiza.ro at checkout | "50 RON off any stay with your NIBIRU ticket", with property cards and prices. |
| **Bus transport** | 20 RON per ride | Information only. Airport coaches are booked with an outside operator. |

## 4. Purchase flow (driven up to the point before payment)

1. **Event page**: date, time, venue, rules (age, parental consent, non-refundable), line-up, last year's recap (attendance, aftermovie, gallery), schedule tabs per day, share (WhatsApp, copy link), "from X RON", "or instalments from Y RON/month".
2. **Ticket picker** (in-page panel, step 1 "Choose ticket"): tabs for Day tickets / Passes / Camping, ticket cards (zones, perks, issuing fee included in the price, instalment price), a +/− quantity stepper, "Continue to checkout".
3. **Checkout** `/checkout?from=<event>` (step 2 "Enter details"):
   - **A 5:00 countdown** ("Finish your order in 04:54") for the cart/inventory hold.
   - Payment choice: **pay in full** (552.48 RON now) or **pay in instalments**.
   - **Account or guest**: email + password, "Create account", "Forgot password", **Google** sign-in (the code also has **eMAG OAuth** via `auth.emag.ro`, plus OTP sign-up by **email + WhatsApp**).
   - Contact: email, first name, last name, phone with a country picker, country, city, county.
   - 3 required consents (event rules + privacy; ticketing operator's rules and sharing data with organiser and operator; non-refundable) plus 1 optional marketing opt-in (email, SMS, WhatsApp).
   - Order summary with a qty stepper, the line "Named ticket · needs personalisation and online check-in", the **platform fee shown only here** (13.47 RON), the total, a promo-code field, and "🔒 Payment processed by **BT iPay**".
   - A valiza.ro accommodation cross-sell.
   - "Continuă la plată" was **not clicked** (that would create an order and redirect to the bank).
4. **What the backend did** (seen on the wire):
   `CreateEmptyCart` → `AddProductsToCart{sku:GALAXIA_2027_GA, qty:1}` → `GetCart` ×2 → `InstallmentOptionsForCart`.
   The cart came back as unit price 439 + `booking_fee` 100 + `platform_fee` 13.475 (2.5%) + `installment_fee` 0 = **552.475 RON**. The price shown everywhere until checkout is 539.

## 5. Ticket lifecycle after purchase (from the API; needs an account to see the UI)

- **Customer tickets**: barcode, status, `selection`, `mapping_name` (zone), `allow_personalization` with `personalize_deadline`, `must_pick_day` (flexible passes where you pick the day later), `allow_checkin` / `checkin_done`, `can_download`, `available_upgrades[]`, `linked_event`.
- **Personalisation**: `CreatePersonalization` (input type is called `FestipayPersonalizationInput`, which suggests a **Festipay** heritage). It records name, DOB, gender, city, region, country and phone per ticket. `PickPersonalizationDay` handles flexible tickets.
- **Online check-in**: `CreateCheckinOnline`, a pre-arrival step that can return a `redirect_url`.
- **Downloads**: ticket PDF by barcode, or **by order number + signature** (a magic link, no login needed), and a **drink voucher PNG** by signature. Guest order lookup by number + email + last name.
- **Instalment plans**: `CustomerInstallmentPlans` (schedule, paid count, masked card), `PayInstallmentNow`, `PayAllRemainingInstallments`, `UpdateInstallmentCard`, `CancelInstallmentPlan`. The marketing hook is "10 RON now locks the price, then 9 monthly instalments". This is a big conversion lever for a young audience.
- **Orders**: `CustomerOrders`, `CustomerOrderByNumber`, `DownloadOrder`.
- **Account**: profile and addresses, social accounts (connect/disconnect Google and eMAG), password reset.
- **Transfer and resale: none.** The FAQ says tickets are named, non-transferable and can't be resold. Some events are said to be unnamed, depending on the event.
- **On site**: card payments at the stands, or a **NIBIRU card loaded with cash at top-up points** (closed-loop cashless for people paying in cash), plus a Helpdesk and **Resolution Points** for ticket, bracelet and access issues.

## 6. Payments (seen, not used)

- `PlaceRedirectPaymentOrder(paymentCode)` → bank redirect (**BT iPay**, Banca Transilvania).
- `PlaceRedirectPaymentOrderWithDiscountCard` (`use_discount_card: true`) is a card-linked discount (tied to the "Pay with BT and you could win a car" campaign).
- `PlaceNetopiaOrder` → **Netopia** (encrypted `enc_data`/`env_key`/`cipher`/`iv` form post): a second card processor, i.e. a fallback acquirer.
- `PlaceOrder` (no payment, for free or voucher orders), `SetPaymentMethodOnCart`, `GetPaymentMethods`, coupons (`ApplyCouponToCart` / `RemoveCouponFromCart`).

## 7. Architecture (inferred)

```
Browser / iOS / Android app
   │
Cloudflare (WAF + Bot Management JS challenge, Turnstile; reCAPTCHA also wired in)
   │
nibiru.net: Next.js App Router (Turbopack build, RSC, next-intl style /en routing)
   ├─ /api/eps   ← BFF proxy (CSRF token from /api/csrf, session via /api/auth/session)
   │      │
   │      └─► EPS backend = Magento 2 / Adobe Commerce GraphQL
   │             (createEmptyCart, addProductsToCart, cart uid = base64 id, products(filter:{sku}),
   │              custom modules: nibiruEvents/Happenings, camping, padel, tables, upgrades,
   │              installments, Festipay personalisation, check-in, signed ticket download)
   ├─ /api/time  ← server clock for countdowns and on-sale openings
   └─ CMS-ish content from `nibiruSettings` (homepage blocks, promo cards, special offers)
Payments: BT iPay (redirect), Netopia (form post)   Map: OMNIS   SSO: Google, eMAG OAuth
Marketing stack: GTM, GA4, Google Ads, Meta Pixel + Conversions API, TikTok Pixel,
                 Microsoft Clarity, Contentsquare, Cookiebot (consent mode v2)
```

Notes:
- Inventory, pricing and fees live in Magento SKUs (`GALAXIA_2027_GA`). Events are a custom entity linked to products. Fees are per product (`fee_amount`, `fee_type: fixed`) plus a platform % at cart level.
- **No waiting room or queue** in the code (no Queue-it or equivalent). On-sale protection appears to rely on Cloudflare bot management, Turnstile/reCAPTCHA tokens on API calls, and the 5-minute cart hold.
- Distribution: **eMAG sold NIBIRU festival tickets in 2026**, with eMAG account login on NIBIRU and a Genius-member perk (free entry before 19:00). That makes it a marketplace and partner channel already.

## 8. Weak spots and inconsistencies (useful for the "next-gen" pitch)

1. **Drip pricing.** The price is 539 everywhere, but checkout shows 552.48 (a 2.5% platform fee appears only at the last step). EU consumer law (and ANPC) expects the full price up front.
2. **Inconsistent instalment figures**: the homepage says "10 RON now, then 9 × 64.77", the event page "from 59.29/month", and the picker "from 59.29/month" for GA but 181.22 for GA Plus.
3. **Contradictory content**: the Promenada costs 10 to 15 RON in 2027 pricing, while the FAQ still says "25 RON online / 35 at the gate". Several "from" CTAs (bundles, padel, `/events/padel`, `/presale`) lead to generic pages or "coming soon".
4. **Discovery is a list, not personal**: no recommendations, no "plan my evening", no group planning, and search only covers the catalogue.
5. **No social or group buying**: no split payment, no "buy for friends, each pays their own", no shared itinerary. Groups are the core of a resort audience.
6. **Rigid tickets**: named, non-transferable, no official resale. That creates support load and a black market, and wastes inventory when people can't come.
7. **No wallet passes** (Apple/Google Wallet): PDF and app only.
8. **Two ticketing stacks** (Beach, Please! is separate). Not multi-tenant: NIBIRU branding and Magento SKUs are hard-wired.
9. **Peak-load resilience**: no visible virtual queue or fair-allocation mechanism for a 100k-person on-sale.
10. **Fragmented on-site journey**: the cashless card, top-ups, drink vouchers, table bookings, padel slots, bus and accommodation are separate experiences, with no single "my trip" timeline.

## 9. Not covered (needs your account, or deliberately skipped)

- Logged-in screens: `/profile` (tickets, orders, instalment plans, upgrade UI, personalisation, check-in, social accounts), the `/chestionar` survey, and upgrade options for a real barcode.
- "Continuă la plată", and everything after it (order creation, BT iPay or Netopia page, order confirmation). It needs real contact details and would create an order in their production system.
- The mobile apps. Padel and table UIs (the APIs exist; no public screen right now).
- Load and security probing of any kind (not appropriate against production).

---

## 10. Payment failure handling (from their client code and the interface text in the page payload; no card data was submitted)

**Payment paths**
- Card via **BT iPay** (BT Pay, a redirect to the bank). Also **"Card BT −10%"**: a payment method that gives 10% off and is *refused if you pay with another bank's card* ("Cu alt card, plata va fi refuzată").
- **Netopia** as a second processor. The client throws a generic `"Netopia payment initialization failed"` if the init payload is incomplete.
- `PlaceOrder` with no payment, for free or voucher orders.

**What the client handles explicitly**
- The cart holds stock for 5 minutes (countdown). Tables give a partial failure: "Nu am putut rezerva toate mesele selectate… nicio masă nu a fost adăugată" (all-or-nothing, with rollback).
- Promo code races ("Coșul tău încă se actualizează…"), instalments unavailable ("Opțiunea de rate nu este disponibilă momentan"), and a security check failing (Turnstile/reCAPTCHA: "Verificarea de securitate nu a trecut").
- After payment: "Nu am găsit biletele pentru această comandă. **Dacă tocmai ai plătit, așteaptă puțin și reîncearcă.**" Tickets are issued **asynchronously** after the bank callback, so the user sees an eventual-consistency gap.
- Instalments: the card is saved via BT iPay for automatic charges. Statuses are `Programată`, `Achitată`, `Restantă` (overdue) and `grace_period`. Recovery actions are "Pay now", "Pay all remaining" and "Change card" ("Actualizarea cardului a eșuat…"), plus a permanent "Cancel plan".
- Paid late check-in: after the free check-in deadline, check-in costs a fee ("te ducem la plată").

**What is missing or weak**
- **No "payment declined / failed" state on the site at all.** There is no copy for a declined card, 3-D Secure failure, timeout or cancelled-at-bank in the checkout or confirmation bundles. Failure is handled entirely on the bank's page (or by silently returning to `/checkout`). It's not clear what happens to the order and to the 5-minute hold after a declined card.
- **`/order-confirmation` shows a success screen without an order** ("Comanda — / NE VEDEM CURÂND!" with no order number). With a made-up order number it asks you to sign in to "claim the ticket". The success page doesn't check payment state before rendering. It shows nothing private, but it's poor UX and a support-ticket generator ("I saw the confirmation, where are my tickets?").
- Generic messages everywhere ("Ceva nu a mers bine. Te rugăm să încerci din nou."), with no error codes the user could quote to support.
- No idempotency or "don't refresh" guidance around the bank redirect, and no "resume payment" for an order that was placed but not paid.

**Other things found along the way:** a **NIBIRU gift card** (249 → 199 RON); Black Friday popups (Promenada 15 → 1 RON, i.e. 93% off); a "Galaxia price rise" countdown popup; an advent "open a present" prize game on the confirmation page; padel at **50 RON per 30 min** (sessions of 30 to 120 min, with LIDL); Ultra VIP tables booked by WhatsApp or email only; table bookings that must be **confirmed by phone within 72 h** or they're cancelled; "Abonament de zi"; Berăria payable in up to 4 equal monthly instalments; a "-X% code if you leave your email" exit offer.

**Pitch angle:** treat a payment as a durable state machine (`reserved → payment_pending → authorised → issued`, with `declined / expired / refunded`). Use an idempotency key per checkout attempt, plus a "resume payment" link that keeps the reservation alive. Fall back automatically to a second acquirer (they already have Netopia) when the first one errors, and issue tickets only from the bank webhook. Show the user one honest status page that polls the order state instead of a blind "see you soon".
