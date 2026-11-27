# Feature matrix and standout shortlist

Built from the seven research reports in this folder (`nibiru-eps-teardown.md` and the six reports in `competitors/`), November 2026.
Legend: ✅ native · 🟡 partial, via a partner, or limited · ❌ none found · ❓ not verifiable (blocked or unpublished).
"RO market" means iaBilet, eventim.ro, Entertix, LiveTickets, UNTOLD and Electric Castle together. A ✅ there means at least one of them does it.

## Matrix

| Capability | EPS (NIBIRU + Beach, Please!) | Ticketmaster | Eventbrite | Tixr | DICE / Fever | Shotgun | TicketSwap | Paylogic / Tomorrowland | pretix | RO market |
|---|---|---|---|---|---|---|---|---|---|---|
| **Open, self-serve organiser onboarding** | ❌ (staff-run, 2 brands) | 🟡 (TM1, Universe) | ✅ | 🟡 (sales-led) | 🟡 (DICE partners) | ✅ | n/a | 🟡 | ✅ | 🟡 (LiveTickets, small events) |
| **White-label depth** | 🟡 (separate hand-built storefronts) | 🟡 | 🟡 (embed not styleable) | 🟡 | ❌ | ❌ | n/a | 🟡 | 🟡 (colours, font, CSS) | 🟡 (UNTOLD = white-label iaBilet) |
| **Visual storefront builder** (sections, live preview, publish/rollback) | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | n/a | ❌ | ❌ | ❌ |
| Capacity pools shared across products | 🟡 | ✅ | 🟡 | ✅ | ❓ | ❓ | n/a | ✅ | ✅ | 🟡 |
| Plugin/rules engine, webhooks, public API | 🟡 (internal GraphQL) | 🟡 (polling partner API) | ✅ | ✅ | 🟡 (Fever MCP) | 🟡 | ✅ | 🟡 | ✅ | ❌ |
| **All-in price from the first screen** | ❌ (2.5% added at checkout) | ✅ US since 2025 | 🟡 | ❓ | ✅ DICE / ❌ Fever UK | ❌ (criticised) | n/a | 🟡 | org choice | ❌ (nobody) |
| Time-step / rule-based dynamic pricing | ✅ (price ladder) | ✅ (Platinum, criticised) | ✅ (rules, "not based on you") | ✅ (sales velocity) | ✅ Fever (weather) | 🟡 | n/a | 🟡 | 🟡 | ✅ (phases) |
| **Fair on-sale queue** (pre-queue + shuffle) | ❌ (Cloudflare bot checks only) | ✅ Smart Queue | ✅ | 🟡 | ❓ | ❓ | n/a | ✅ (Queue-it) | ❌ | ❌ (Metallica complaints to ANPC) |
| Identity-first sale: pre-registration, lottery, verified fan | ❌ | ✅ Verified Fan, Ticket Request | ❌ | ✅ pre-authorised waitlist | ✅ | ✅ pre-register | ✅ Auto-buy | ✅ (10-week registration) | ❌ | ❌ |
| **Instalments / deposits** | ✅ (deepest in RO, self-run) | 🟡 (Flex Pay, outside lender) | 🟡 (PayPal) | ✅ | ✅ Fever | ❓ | n/a | ✅ | 🟡 | ✅ (bank cards) |
| **Group buy, each friend pays their share** | ❌ | 🟡 (Venmo, after the fact) | ❌ | ❌ (not found) | 🟡 (DICE Groups, Fever split) | ❌ | n/a | 🟡 (group caps) | ❌ | ❌ |
| Payment provider choice and failover | 🟡 (BT iPay + Netopia) | ❓ | 🟡 | ❓ | ✅ Fever routing | ❓ | n/a | ❓ | ✅ (many) | ✅ (about 20 methods at iaBilet) |
| Clear payment-failure UX and resume | ❌ (no declined-card state) | ❓ | ❓ | ❓ | ❓ | ❓ | n/a | ❓ | 🟡 | ❓ |
| **Official resale / transfer with new barcode** | ❌ (named, non-transferable) | ✅ (face-value caps in UK/IE) | ❌ | ✅ (floors and caps) | ✅ DICE waitlist returns | 🟡 (15% fee) | ✅ (+20% cap, FairShare) | ✅ | 🟡 | 🟡 (dăBilet, EC marketplace) |
| **Wallet passes / rotating QR** | ❌ (PDF + app) | ✅ SafeTix | 🟡 | ✅ | ✅ (QR appears near doors) | 🟡 | ✅ | 🟡 | 🟡 | ❌ (nobody) |
| Personalisation + online check-in | ✅ | 🟡 | ❌ | 🟡 | 🟡 | ❌ | n/a | ✅ | 🟡 | ✅ (UNTOLD) |
| Upgrades | ✅ | 🟡 | ❌ | 🟡 | ❓ | ❓ | n/a | ✅ | 🟡 | 🟡 |
| **Trip bundle** (camping, hotel, transport, tables, parking in one cart) | ✅ (ticket + camping + hotel) | 🟡 | ❌ | ✅ (camping map, hotels) | ❌ | 🟡 (merch via Shopify) | n/a | ✅ (Global Journey) | 🟡 | 🟡 |
| Cashless / RFID native | 🟡 (NIBIRU card, separate) | ❌ | ❌ | ❌ | ✅ Fever | ❌ | n/a | 🟡 (partners) | ❌ | ✅ (via vendors, e.g. Rhuna) |
| Taste-based discovery and social (Spotify, friends going) | ❌ | 🟡 (Spotify-reserved) | 🟡 | 🟡 | ✅ | ✅ | 🟡 | ❌ | ❌ | ❌ |
| Organiser-owned CRM and marketing | 🟡 (newsletter consent) | ✅ | ✅ | ✅ (referrals) | 🟡 | ✅ | n/a | 🟡 | 🟡 | 🟡 (iaBilet Spotlight) |
| Referral / promoter rewards | ❌ | ❌ | 🟡 | ✅ Tixr Rewards | ❓ | ✅ cashback | ❌ | ❓ | 🟡 | ❌ |
| **AI-agent ready** (MCP, ChatGPT/Gemini apps, AI concierge) | ❌ | ✅ (ChatGPT, Alexa+, AI Mode) | 🟡 (ChatGPT discovery) | ❌ | ✅ Fever MCP + Gemini | ❌ | 🟡 (WhatsApp bot) | ❌ | ❌ | ❌ |
| Offline door / scan app | ❓ | ✅ | ✅ | ✅ (iOS) | ✅ | ✅ (Xpress Door) | n/a | ✅ | ✅ | ✅ |
| Live organiser dashboards (sales, demand, crowd) | ❓ | ✅ (TM1 Demand) | ✅ | ✅ | ✅ | ✅ | n/a | ✅ | ✅ | 🟡 |

Caveats: Ticketmaster, Tixr, DICE, Shotgun, TicketSwap, AXS, See Tickets and eventim.ro blocked crawling. Their rows rest on help centres, organiser sites and press. A few dated legal claims about Ticketmaster (2026) and some fee figures come from secondary sources and should be checked before they appear in the vision page.

## What nobody does (white space)

1. **A real visual white-label builder.** Everyone stops at logo, colours and a font, or jumps to code templates and a headless API.
2. **Group buying where each friend pays their own share, with a shared hold.** Only partial or after-the-fact versions exist (Venmo, DICE Groups).
3. **An honest checkout under failure:** an all-in price, an explicit payment state machine, resume-payment, and failover between providers that the fan can see.
4. **In Romania:** all-in prices, wallet passes, a fair and transparent on-sale queue, and self-serve onboarding for big organisers. Nobody offers any of these.
5. **One engine across ticketing, trip bundle and cashless.** Competitors are strong in one or two and partner for the rest. EPS already holds tickets, camping, hotel, upgrades and instalments.

## Standout shortlist (candidates for planning, not decided)

| # | Idea | Why it stands out | Evidence |
|---|---|---|---|
| S1 | **Storefront builder** (themes as JSON sections and blocks, live preview, draft, preview link, publish and rollback; checkout locked to design tokens and fixed slots) | No competitor has one; it delivers the "open to any organiser" brief | whitelabel.md |
| S2 | **Fair on-sale engine** (pre-registration, pre-queue + shuffle, signed tokens, silent bot scoring, sharded stock, pre-authorised waitlist) with a live simulator | Answers the 100k-in-10-minutes question with a working demo; the RO market has nothing like it | festival-tech.md, ticketmaster.md, organiser-platforms.md |
| S3 | **Honest checkout** (all-in price everywhere, payment state machine, idempotency, provider failover, resume-payment, status page) | EPS's own gaps (drip pricing, blind success page) plus EU and ANPC rules | nibiru-eps-teardown.md |
| S4 | **Squad buy** (shared hold, each friend pays their own share or instalment, then personalises their own ticket) | Nobody does it fully; groups are the core resort audience | all |
| S5 | **Trip wallet** (ticket, camping, table, padel, bus, cashless balance on one timeline; wallet pass with rotating QR; face-value transfer and resale with a new barcode) | Joins up what EPS already sells; nobody in RO has wallet passes | romania.md, fan-first.md |
| S6 | **AI layer** (organiser copilot that builds the event and storefront from a brief; fan concierge; an MCP server so ChatGPT and Gemini can sell EPS events) | Ticketmaster and Fever are already moving to AI agents; nobody in RO is | ticketmaster.md, fan-first.md |
| S7 | **Organiser growth tools** (owned CRM, referral rewards, promoter links, rule-based pricing that says it isn't based on you) | Tixr and Shotgun prove the value; EPS has none | organiser-platforms.md, fan-first.md |
| S8 | **Multi-organiser city or festival pass** (several organisers in one cart, revenue split; Amsterdam Dance Event model) | Shows an open platform beating one-organiser shops | festival-tech.md |
