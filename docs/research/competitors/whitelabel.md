# White-label ticketing and shop/checkout builders: competitive research for EPS (NIBIRU)

Research date: 27 Nov 2026. Markers: **[seen]** = observed directly (a public demo shop, rendered docs, generated CSS, the source tree); **[source]** = taken from the vendor's docs or pages at the URL given; **[3rd]** = a third-party or competitor page, so less reliable. No accounts were created, no orders were placed and no logins were used. Raw notes and screenshots are in `raw/whitelabel/`.

---

## 1. pretix (open source, AGPLv3, Heidelberg)

The closest structural analogue to what EPS wants to become: an open, multi-organiser ticketing engine with a plugin system, a hosted SaaS and self-hosted editions.

**Code layout [seen]** (shallow clone of github.com/pretix/pretix): `src/pretix/{base, presale, control, api, multidomain, plugins, helpers, static, locale}`. `base/models/` has one file per domain concept: `organizer, event, items, orders, vouchers, discount, seating, waitinglist, giftcards, memberships, customers, invoices, checkin, devices, tax, media, datasync, mail`. Built-in plugins: `stripe, paypal2, banktransfer, manualpayment, ticketoutputpdf, badges, sendmail, statistics, reports, checkinlists, webcheckin, autocheckin, returnurl`. `multidomain/` has separate URL confs per domain role (`maindomain`, `organizer_domain`, `organizer_alternative_domain`, `event_domain`) plus middleware: domain routing is a first-class layer. License: AGPLv3 [seen in LICENSE].

**Extensibility: Django signals [seen in `base/signals.py`, `presale/signals.py`, `control/signals.py`]**
- Registries: `register_payment_providers`, `register_ticket_outputs`, `register_data_exporters`, `register_sales_channel_types`, `register_mail_placeholders`, `register_html_mail_renderers`, `register_invoice_renderers`, `register_ticket_secret_generators`, `register_event_permission_groups`.
- Rules hooks: `validate_cart`, `validate_cart_addons`, `validate_order`, `order_fee_calculation`, `fee_calculation_for_cart`, `quota_availability` (a plugin can change availability), `allow_ticket_download`, `checkout_flow_steps` (add a checkout step), `question_form_fields` / `contact_form_fields` (+ `_overrides`), `checkout_all_optional`.
- Lifecycle events: `order_placed/paid/canceled/expired/changed/approved/denied`, `checkin_created`, `customer_created`.
- Storefront slots: `html_head`, `html_page_header`, `front_page_top/bottom`, `item_description`, `render_seating_plan`, `footer_link`, `checkout_confirm_page_content`.
- Back-office slots: `nav_event`, `event_dashboard_widgets`, `item_forms`, `voucher_form_*`, `organizer_edit_tabs`.
Plugins are enabled per event (or per organiser). This is the most complete "rules engine as hooks" design among the platforms reviewed, but it means Python code, not organiser-configurable rules.

**Theming [seen + source]**
- The Shop design tab (organiser-level, event can override) sets a primary colour, success/error colours, page background and a font from a fixed list. A contrast checker warns on low-contrast picks [source: https://docs.pretix.eu/guides/public-profile/]. Header image, front-page text per language, list/week/month event overview.
- **Implementation [seen]:** each event serves its own compiled `/{org}/{event}/theme.css` of about 8 KB, defining CSS custom properties (`--pretix-brand-primary: #8e44b3`, `--pretix-border-radius-base: 3px`, `--pretix-font-family-sans-serif`, `--pretix-link-contrast-color`, ...) on top of a shared static `presale.css`. Settings → tokens → CSS variables is exactly the pattern EPS should copy.
- No visual builder, no drag and drop, no draft/publish of designs; changes "take a few seconds to minutes" to appear [source]. "Powered by pretix" footer is visible on the demo [seen: https://pretix.eu/demo/democon/].
- Custom domains: organiser main domain, event domain, alternative domain for a set of events; hosted = CNAME [source: https://docs.pretix.eu/guides/custom-domain/].

**Widget [source: https://docs.pretix.eu/en/latest/user/events/widget.html]**: one loader `<script>` + `<pretix-widget event=…>` (or `<pretix-button items="item_12=2">` for a direct-to-checkout button). The product list renders inline as JS (no iframe); checkout opens in an iframe overlay, or in a new tab on small screens / non-HTTPS. Attributes: `list-type` (list/calendar/week), `items`, `categories`, `variations`, `voucher`, `subevent`, metadata `filter`. JS API: `pretixWidgetCallback`, `buildWidgets()`, `addLoadListener/addCloseListener`, `open(...)`. Prefill with `data-email`, `data-question-<id>`, `data-attendee-name-*`. Consent pass-through (`data-consent`). Documented gotchas with COOP/COEP headers and Apple Pay domain verification.

**Product model [source: https://docs.pretix.eu/guides/products/; seen in demo]**
- Items with variations, categories (normal / add-on / cross-sell), **quotas: one capacity can contain many products, and a product can sit in several quotas** (it sells while all its quotas have room). The best capacity-pool model on the market.
- Add-ons (category-based, with min/max), bundles (also for mixed VAT), availability windows, "only show after X sells out" (automatic tiers), voucher-only products, sales channels, memberships (product creates or requires a membership), customer accounts, gift cards, discounts (`discount.py`), free-price ("pay what you want") items, waiting list (seen on a sold-out VIP ticket in the demo), event series with sub-event dates (timed entry; seen as a week calendar in https://pretix.eu/demo/museum/), resources and scheduling.
- Seating: plans drawn in a separate web editor (seats.pretix.eu), exported as JSON and uploaded; one seat category maps to one product; capacity is still enforced through quotas [source: https://docs.pretix.eu/guides/seating/]. Hosted/Enterprise only.

**Multi-tenant and roles [source: https://docs.pretix.eu/guides/teams/]**: organiser → teams; each team holds organiser permissions plus event permissions, limited to selected events or all events; API tokens belong to a team; 2FA can be required per team. No "brand" layer between organiser and event apart from domains.

**Payments**: the organiser connects their own Stripe or PayPal (Stripe Connect on hosted) or bank transfer; pretix is not the merchant of record. **Pricing [source: https://pretix.eu/about/en/pricing]**: hosted 2.5 % of the net price, capped at €15 per ticket; free tickets €0.50 after 500 per year; invoiced monthly; self-hosted Community edition free; Enterprise paid. **Data**: full REST API plus webhooks, exporters, AGPL source.

**Distinctive**: quota model, signal-based plugin architecture, open source, EU/GDPR posture, robust widget. **Weak**: dated Bootstrap-era storefront, design limited to a few colours and a font, no builder or preview workflow, no draft/publish for design, plugin rules need developers.

---

## 2. Ticket Tailor (UK)

- **Customisation [source: https://www.tickettailor.com/features/white-label-event-ticketing]**: colours, fonts and images for the box office and event pages, checkout and confirmation emails; an "event page builder" (https://tickettailor.com/event-page-builder). The **White Label add-on** (monthly fee; about £39/month [3rd: https://tickets.chobble.com/guide/choosing-a-ticket-platform/remove-ticket-platform-branding/]) removes branding from the buyer side only: "the dashboard and check-in app cannot be white-labelled". Custom domain: a paid add-on by CNAME, included with White Label [source: https://intercom.help/ticket-tailor/en/articles/6758421-how-can-i-add-my-custom-domain-to-my-box-office].
- **Embeds**: box-office widget, single-event widget, JS widget, and a **cart widget** since April 2026 [source: https://updates.tickettailor.com/137232-our-cart-widget-is-now-live]. They use iframes, so the host site's CSS cannot reach inside [3rd: https://webflow.com/integrations/ticket-tailor].
- **Products**: ticket types, add-ons, reserved seating with custom charts (1 extra credit per seat), multi-checkout (one cart across events) [source: https://www.tickettailor.com/pricing].
- **Payments**: always the organiser's own Stripe, PayPal or Square; Ticket Tailor never holds funds. A booking fee can be passed to buyers.
- **Pricing**: flat per ticket, not a percentage: £0.60 pay-as-you-go or pre-paid credits from £0.22; free tickets free up to 5,000 a year; no subscription [source].
- **Distinctive**: a flat fee instead of a percentage (a strong message for high-price tickets), the organiser owns the merchant account, payouts are immediate. **Weak**: shallow theming (colours and fonts), the back office cannot be white-labelled, no real builder, iframe-only embeds.

## 3. Tito (ti.to), now "Tito Pro"

- **Widget v2 [source: https://ti.to/docs/api/widget/; seen on https://ti.to/home]**: `<script src="https://js.tito.io/v2">` + `<tito-widget event="org/slug">` or `<tito-button>`. Options: `releases` (filter ticket types, including secret ones), `discount-code`, `prefill` JSON, `save-metadata-parameters` (for example `utm_*`). Events: `registration:started/finished`, `widget:loaded`. iframe by default; an **`inline` plugin drops the iframe for full CSS control**, and a "without widget-css" option serves it unstyled. The developer-friendliest widget reviewed.
- **Tito Pro [source: https://pro.tito.io/docs/customization]**: brand colours, banner, **fully custom HTML templates and fully custom layouts**, custom email senders (DKIM, SPF and DMARC on your domain), **Apple/Google Wallet pass design**, an attendee hub (beta). Custom domains: up to ten per account, each for the account, one event or a series, with Let's Encrypt SSL automatic and manual allow-listing in under 48 h [source: https://pro.tito.io/docs/customization/custom-domains]. Ticket types support bundles, time slots, volume discounts and **dependencies** (ticket B requires ticket A). Also team permissions, API tokens and test mode. The homepage says "a new version of Tito is coming in January" [seen].
- **Payments**: connect your own Stripe or PayPal, with funds paid straight to the organiser. **Pricing**: 3 % per paid ticket (2.5 % for non-profits), capped at €25 per ticket, free tickets free, no subscription [source: https://ti.to/pricing].
- **Distinctive**: developer-first (inline widget, events, prefill, metadata), a custom sending domain for email, wallet-pass design, ticket dependencies. **Weak**: aimed at conferences; no seating, no festival extras (cashless, camping), templates mean code rather than a visual builder.

## 4. Eventix / Weeztix (NL, Eventix renamed Weeztix in 2024)

- **Shop [source: https://weeztix.com/help/colours-ticket-shop, https://weeztix.com/sell-tickets-online]**: a **separate "shop" object, with several shops per event or across events** (each shop chooses its tickets and products); a brand colour from a curated palette guaranteed to work in **dark and light mode** (or follows the device), plus background and foreground colours; custom ticket image per ticket type or event; embed as an iframe or hosted page. Time slots, multi-day tickets, seated plans, upsell products, a virtual queue, secondary resale (SecureSwap, "sealed tickets with fair share"), API and **custom webhooks** [source: https://weeztix.com/pricing].
- **Payments and pricing**: Weeztix collects the payments (platform merchant of record) and pays out. The booking fee is added for the buyer by default; the organiser can absorb it. Free events are free up to 5,000 tickets, then €0.20; paid events are quoted [source]. About 15 % fee on resale and waiting-list purchases [3rd: https://www.ticketfairy.com/compare/ticketing/weeztix-alternatives].
- **Distinctive**: the **shop as a first-class object separate from the event** (one event, several shops for presale, partners, VIP; one shop for many events), and colour presets safe for both themes. **Weak**: custom domains not documented, theming limited to colours, no visual builder, fees quoted rather than published.

## 5. Universe (Ticketmaster-owned)

- **embed2.js [source: https://developers.universe.com/docs/embed2js]**: one script that turns links to Universe event pages into an on-site checkout; URL params `unii-trigger-open`, `unii-discount-code`, `unii-access-key`; JS `$u.open(id, {accessKeys, currentDiscountCode, analytics})`. Universe markets a "white-label, mobile-first checkout" for attractions (timed entry); the page `kontinuum.universe.com` was blocked from this session's network, so this is unverified. A 3rd-party comparison rates its white label as "No" [3rd: https://www.ticketfairy.com/event-ticketing/eventbrite-vs-universe.md].
- **Distinctive**: access keys (unlock hidden tickets) as a primitive, and the Ticketmaster distribution network. **Weak**: little branding depth, platform merchant of record, focus on the US.

## 6. Eventbrite embedded checkout

- **[seen, rendered docs: https://www.eventbrite.com/platform/docs/embedded-checkout]**: `eb_widgets.js` + `EBWidgets.createWidget({widgetType:'checkout', eventId, modal:true, modalTriggerElementId, onOrderComplete, iframeContainerId, iframeContainerHeight:425, iframeAutoAdapt, promoCode})`. Modal or inline iframe; the docs recommend the modal on mobile. HTTPS required.
- Branding: none beyond the event's own image and description. Eventbrite stays the merchant of record (payouts on its schedule) and owns the buyer relationship (marketplace emails, discovery).
- **Distinctive**: discovery marketplace, a simple callback API. **Weak**: the opposite of white label; the buyer data relationship is shared with Eventbrite.

## 7. Paylogic (Eventim / See Tickets; NL, festival leader)

- **[source: https://www.eventimb2b.nl/en/our-ticketing-technology/paylogic, https://paylogic.com/see-tickets]**: a self-service white-label shop "built in minutes": colours, typography, logos, backgrounds, messaging and **dynamic text**, plus ticket design; embedded on the event's own site with no redirect; **event templates you duplicate** for new editions; **festival upsells** (merch, tokens/cashless, camping, parking, lockers, VIP upgrades, before, during and after purchase); Send 2 Friend (tickets in a friend's name), pre-registration, waiting lists, **data capture in the queue**, fan-to-fan resale with price caps, 27+ tracking pixels plus **server-side tracking**, custom domains, an open API, the Entrance scanning app. Clients include Dekmantel and 013.
- **Payments and pricing**: "flexible commercial models", sold through sales, not published.
- **Distinctive**: the closest match to NIBIRU's festival needs (camping, lockers, tokens, resale, queue, templates per edition). **Weak**: an enterprise sale, closed, no public builder documentation, no developer self-service.

## 8. vivenu (DE; newer API-first player) [added: highly relevant]

- **[source: https://vivenu.com/solutions/festivals, https://vivenu.com/custom-solutions-and-backbone, https://vivenu.com/private-label-solutions]**: fully white-labelled shops with custom domains and "user flows"; **customisable shop templates, iframes, or fully headless** (build your own front end on its API); custom ticket designs, first-party data capture. **A "private label" offer lets a company run its own ticketing brand on vivenu**: it sets its own fees and margins and owns customer and client relationships. This is exactly the business NIBIRU wants EPS to enter. It is strongest in sports (season tickets, resale) [3rd: https://www.ticketnews.com/2026/09/7-best-white-label-ticketing-software-platforms-in-2026/].
- **Weak**: enterprise pricing, no public builder documentation.

## 9. Shopify: the gold standard for builder UX

**Theme architecture [source: https://shopify.dev/docs/storefronts/themes/architecture]**: `layout/theme.liquid` → `templates/*.json` (a page = a list of sections) → `sections/` (with `{% schema %}` declaring settings, block types and **presets**) → blocks; `snippets/` (not editable by merchants); `config/settings_schema.json` (global theme settings: colours, type, etc.) and `settings_data.json` (their values); `locales/`. **Section groups** for the header and footer.

**JSON template format [source: https://shopify.dev/docs/storefronts/themes/architecture/templates/json-templates]**: `{ "sections": {id: {type, settings, blocks: {id: {type, settings}}, block_order, disabled}}, "order": [ids] }`. Limits: 25 sections per template, 50 blocks per section. A section can only be added in the editor if it has presets; `enabled_on`/`disabled_on` restrict where it can go. **App blocks** let third-party apps inject blocks into merchants' themes.
→ The key insight: **the page is data (JSON) referencing typed, schema-described components; the editor is generated from the schemas.** The editor needs no hand-written UI per setting.

**Theme editor UX [source: https://help.shopify.com/en/manual/online-store/themes/customizing-themes/theme-editor; plus well-known product behaviour]**: live preview iframe beside a left sidebar of sections and blocks (drag to reorder, hide, duplicate), device toggle (desktop/mobile), click-to-select inspector in the preview, undo/redo, and a theme library where unpublished themes are drafts you edit, preview through a share link, duplicate as a backup and then publish. Sidekick AI assistant in the editor.

**Checkout**: the checkout is not a theme. It is styled through the **Checkout Branding API** (`checkoutBrandingUpsert`: a `designSystem` of colours / colour schemes, typography, corner radius, plus per-component `customizations`; the newer configuration API calls them `designTokens`, with up to 20 palette colours, primary and secondary font groups with base size and ratio, and small/base/large radii) [source: https://shopify.dev/docs/apps/build/checkout/styling]. **Checkout UI extensions** [source: https://shopify.dev/docs/api/checkout-ui-extensions] add UI in a **sandbox** using Shopify's own components (remote-dom), never raw HTML: static targets (fixed spots) vs block targets (the merchant places them in the checkout editor, up to 3 per location) vs runnable targets (no UI); no access to payment data; Plus only for the information, shipping and payment steps. **Shopify Functions** [source: https://shopify.dev/docs/api/functions] are server-side business rules compiled to Wasm, deterministic, with a GraphQL input query and declarative output operations: discounts, cart and checkout validation, cart transform (bundles), payment and delivery customisation, order routing. Limits: 11M instructions, 128 kB input. Merchants configure them through metafields in an admin UI extension.
→ The key insight: **the storefront is fully open (themes), the checkout is locked down (tokens + sandboxed slots + pure-function rules)**, because the checkout is where money, trust and compliance live.

## 10. Stripe Checkout / Payment Links branding

- **[source: https://docs.stripe.com/payments/checkout/customization/appearance]**: logo, icon (favicon), background colour, button colour, font (from a list) and border style (rectangular/rounded/pill), set in the Dashboard or **per session through `branding_settings`**. With Connect, Checkout uses **the connected account's branding** (direct charges, or destination charges with `on_behalf_of`); platforms set it through the Accounts API. Hosted, embedded or Elements modes. Custom fields (up to 3: text, numeric, dropdown) and terms-of-service consent. **Custom domain for Checkout** = a paid add-on: CNAME + TXT, Stripe issues TLS automatically [source: https://docs.stripe.com/payments/checkout/custom-domains.md?payment-ui=stripe-hosted].
- **Connect charge types [source: https://docs.stripe.com/connect/charges]**: **direct charges** (the organiser is the merchant: refunds and disputes hit their balance; the platform takes an `application_fee_amount`; recommended for SaaS) vs **destination charges** (the platform is the merchant and carries refunds and disputes; `on_behalf_of` puts the organiser's name on the card statement) vs **separate charges and transfers** (one cart split across many accounts, for example a festival plus a camping operator plus a merch vendor).
- **Distinctive**: branding per session and per connected account, the cleanest token set. **Weak**: no event concepts at all.

## 11. Newer and notable players (short)

- **Humanitix** (AU/NZ/UK/US; profits go to charity) [source: https://help.humanitix.com/en/articles/8896961-choose-who-pays-the-fees-for-your-event, https://help.humanitix.com/en/articles/8889214]: **4 fee modes** (pass on, absorb, split by percentage, or the buyer tips); 3 widget types (inline checkout, pop-up button, event collection); about 4 % + A$0.99 per ticket [3rd]; platform merchant of record. Custom domains not documented.
- **Billetto** (DK/UK) [source: https://billetto.co.uk/da/l/how-to-embed-a-ticket-widget]: button, ticket preview or calendar widget; a "White label: remove Billetto branding" toggle inside the widget settings; buyer-paid fee (about 3.99 % + £0.90 [3rd]); custom pricing above 1,000 tickets a year.
- **TicketSource** (UK) [source: https://help.ticketsource.com/en/article/customising-your-ticket-shop-page-1kx7teb/]: a colour preview tool for fonts, buttons and background; **custom CSS, with example stylesheets published on GitHub**; list, card, timeline or time-slot layouts; custom shop URL.
- **Xceed** (ES; nightlife) [source: https://support.xceed.me/en/articles/9060260-how-to-install-the-xceed-widget]: a widget for tickets, **guest lists and bottle-service tables** with no redirect; look and feel set in Xceed Pro; **landing pages mapped to sales channels** for attribution; Meta pixel and GTM.

---

## Comparison table

| Platform | Custom domain | Theme depth | Live preview builder | Embed widget | Own payment account | Roles | Capacity pools | Seating | Plugins / API | Pricing model |
|---|---|---|---|---|---|---|---|---|---|---|
| pretix | Yes, 3 roles (org/event/alt), CNAME | Low-mid: colours, font, header; per-event CSS vars | No | JS inline list + iframe overlay checkout, button mode, JS API | Yes (Stripe/PayPal/bank) | Teams × org/event permissions, scoped events, 2FA | **Best: quotas shared across products, many-to-many** | Yes (JSON plan editor, hosted/Enterprise) | **Signals plugin system**, REST, webhooks, open source | 2.5 % capped at €15; self-host free |
| Ticket Tailor | Paid add-on | Low: colours, fonts | Page builder, basic | iframe widgets, cart widget | Yes (Stripe/PayPal/Square) | Basic | Per ticket type | Yes (+1 credit per seat) | API, apps | Flat £0.22 to 0.60 per ticket |
| Tito / Tito Pro | Yes, up to 10, auto SSL, plus email domain | High (custom HTML templates, wallet passes) | No (code templates) | **Best dev widget**: iframe or inline, events, prefill | Yes (Stripe/PayPal) | Team permissions | Partial (ticket dependencies, time slots) | No | API, webhooks | 3 % capped at €25 |
| Weeztix | Not documented | Low: palette safe for dark/light | No | iframe / hosted | No (Weeztix collects) | Yes | Shops × tickets, time slots | Yes | API, custom webhooks | Buyer fee, quoted |
| Universe | n/a | Low | No | embed2.js overlay | No | n/a | Timed entry | Yes | Basic | Buyer fee |
| Eventbrite | No | Minimal | No | Modal or inline iframe | No | Yes | Per ticket | Yes | Large API | Buyer fee + % |
| Paylogic | Yes | Mid-high (dynamic text, ticket design) | Self-service, not public | Embedded, no redirect | Commercial models | Enterprise | Yes (festival upsells) | Via See Tickets | Open API, 27+ pixels | Negotiated |
| vivenu | Yes | High + headless | Templates | iframe or headless | Private label: yes | Enterprise | Yes | Yes | API-first, SDKs | Enterprise |
| Shopify (reference) | Yes | **Full (themes, sections, blocks)** | **Yes: gold standard** | Buy Button / Checkout Kit | Shopify Payments or 3rd party | Staff permissions | Inventory per location | n/a | **Apps, UI extensions, Functions** | Subscription + payment % |
| Stripe Checkout (reference) | Paid add-on | Tokens: colour, font, radius | Dashboard preview | Hosted / embedded / Elements | Connect (3 charge types) | Dashboard roles | n/a | n/a | API | Payment % |
| Humanitix | No | Low | No | 3 widget types | No | Yes | Per ticket | Yes | API | Buyer fee, 4 modes |
| TicketSource | Short URL only | Mid (custom CSS) | Colour preview | Widget | Own or platform | Yes | Per ticket | Yes | API | Buyer fee |
| Xceed | No | Low | No | Widget (tickets, guest list, tables) | No | Yes | Tables, lists | Tables | Pixels | Freemium + add-ons |

---

## Blueprint for the EPS builder

### What the market teaches (the 8 ideas worth stealing)
1. **Page = JSON of typed sections and blocks, editor generated from schemas** (Shopify). Organisers rearrange; developers add new section types once.
2. **Theme = design tokens compiled to CSS variables per storefront** (pretix's `theme.css`, Stripe's `branding_settings`, Shopify's `designSystem`). Contrast check at input time (pretix) and palettes guaranteed for light and dark (Weeztix).
3. **Open storefront, locked checkout**: the checkout takes tokens and named slots only, never free HTML (Shopify checkout extensibility, Stripe).
4. **Capacity pools as a separate, many-to-many object** (pretix quotas). A product sells while all its pools have room, which covers "festival pass + day ticket share the gate capacity" and "camping spots shared by 2-person and 4-person tents".
5. **Shop ≠ event** (Weeztix): one event can have several shops (public, presale, partners, staff, hotel guests) and one shop can sell several events or dates (a resort season).
6. **Rules as declarative, deterministic functions** (Shopify Functions, pretix `validate_cart` / `quota_availability`), configured by organisers, not code.
7. **Organiser as merchant through Stripe Connect** (direct charges + application fee) as the default, with a destination / separate-transfers mode for split carts (festival + camping + partner vendors).
8. **Widget as a custom element with events and prefill** (Tito / pretix): `<eps-shop>`, `<eps-button>`, an inline mode, `on:order:completed`, UTM capture, consent pass-through.

### Data model

```
Platform (EPS operator, NIBIRU)
└─ Tenant (organiser company: legal entity, Stripe Connect account, VAT, invoicing)
   ├─ Members ── Role (owner, admin, designer, product-manager, box-office, scanner, finance)
   │             └─ scope: tenant | brand[] | event[]    (pretix-style scoped teams)
   ├─ Brand (Neversea, Untold-style sub-brand, resort): name, logo set, legal/terms, support email,
   │   │      sending domain (DKIM/SPF), default locale(s) + currency
   │   ├─ Domain[] (tickets.brand.ro → role: brand | storefront | event; status, TLS cert)
   │   └─ ThemeVersion[] (draft | published | archived; immutable once published)
   │        tokens: { color.primary, color.surface, color.text, color.accent, radius.{s,m,l},
   │                  font.{heading,body}, mode: light|dark|auto, logo, heroImage }
   │        templates: { home, event, checkout, confirmation, email.*, ticket.pdf, wallet.pass }
   │           each = { sections: {id:{type,settings,blocks,block_order}}, order: [...] }
   └─ Storefront / Shop (sales channel): brand, theme pin, events/dates[], product filter,
        │       access (public | code | link | customer group), checkout config:
        │       { fields[], questions[] (per order | per ticket), consents[], terms version }
        └─ Event (edition: "Festival 2027") ── Date/Slot[] (timed entry, days)
             ├─ Product (ticket | add-on | bundle | merch | camping | parking | locker | token top-up)
             │    variations[], price tiers (by date / by sold count / by pool fill),
             │    min/max per order, requires[] (dependencies), sales window, visibility
             ├─ CapacityPool (name, size, scope: event | date) ←→ Product (many-to-many)
             │    seated pools link to SeatMap sections
             ├─ Rule[] (declarative, ordered, versioned):
             │    when: cart | checkout | order   if: condition AST   then: action
             │    (limit, require, discount, fee, hide, block, add-on offer)
             ├─ Voucher / AccessCode / WaitingList / Resale policy
             └─ Webhook subscriptions, API tokens (per tenant, scoped)
Order → Positions (attendee data, seat, slot) → Payments (Connect charge) → Tickets (PDF, wallet, QR secret)
```

Key rules: every published artefact (theme, checkout config, rules, terms) is **versioned and immutable**, and each order records the versions it was sold under (audit, disputes, "which terms did they accept"). Tenant isolation by `tenant_id` on every row plus Postgres row-level security.

### Builder modules, in priority order for the 48-hour prototype

| # | Module | 48 h scope | Why first |
|---|---|---|---|
| 1 | **Brand and theme tokens** with live preview | Logo, 4 colours, 2 fonts, radius, light/dark/auto; tokens → CSS variables; **WCAG contrast check** with a suggested fix; preview iframe updates live through postMessage | The fastest "wow"; it proves white label |
| 2 | **Products + capacity pools** | Ticket types, add-ons, a pool shared by several products, a live "remaining" display; tiered price by sold count | The engine's real differentiator; demo "day pass + full pass share the gate" |
| 3 | **Storefront page builder** | 6 to 8 section types (hero, lineup/info, ticket picker, add-ons, FAQ, map, footer) from JSON schemas; drag to reorder, hide, device toggle | Shopify-grade UX in a small way |
| 4 | **Checkout builder** | Field and question list (per order / per attendee, types, required, conditional), consents, terms link; locked layout that only takes tokens | Every organiser needs it; shows "open front, locked checkout" |
| 5 | **Draft → preview link → publish** with version history and one-click rollback | Immutable ThemeVersion / ShopVersion; shareable preview URL | Safety for the organiser and for NIBIRU's ops |
| 6 | **Rules (no-code)** | 3 templates: max N per order, "requires product X", "discount when the cart holds X + Y"; evaluated server-side as pure functions with an explanation shown to the buyer | Shows the extensibility path |
| 7 | **Embed + domain** | `<eps-shop>` custom element (inline list + overlay checkout, an `order.completed` event); domain screen with CNAME instructions and status (TLS through Caddy on-demand or Cloudflare for SaaS, real or mocked) | Distribution |
| 8 | Roles, webhooks, emails and ticket PDF | Mocked or read-only screens | Completeness of the story |

Prototype path: one tenant with two brands (a festival and a resort) to show **multi-brand under one account**, switching brands in the preview to show the same engine wearing two looks.

### What would make EPS clearly better than everything above
1. **A Shopify-grade visual builder for ticketing.** Nobody in ticketing has one: pretix, Ticket Tailor, Weeztix and Humanitix stop at colours; Tito and vivenu jump straight to code or headless. A schema-driven sections/blocks editor with live preview, a device toggle, drafts, preview links and rollback would stand alone in the market.
2. **pretix-grade capacity modelling with a visual explanation**: pools shown as bars with the products that draw on each, plus a "why is this sold out?" debugger. Today this is powerful but opaque in pretix and absent elsewhere.
3. **Festival and resort primitives built in** (Paylogic-level): camping, parking, lockers, cashless top-up, timed entry, accommodation-linked add-ons, ticket transfer and capped resale. NIBIRU's own domain advantage, and also good for any organiser.
4. **No-code, deterministic rules with explanations** (Shopify Functions semantics, but configured in the UI): every rule tested against sample carts in the builder before publish.
5. **Honest payments**: the organiser is the merchant through Stripe Connect direct charges by default (Tito, Ticket Tailor, pretix), with transparent fee modes (pass on / absorb / split, Humanitix-style) shown in the preview, and split-cart transfers for partner vendors.
6. **Versioned everything + an audit trail**: orders pinned to the exact theme, checkout, terms and rule versions they were sold under. Nobody advertises this; it matters for disputes and GDPR.
7. **Accessibility and dark mode by construction**: a contrast gate on tokens (pretix's warning, made blocking), palettes valid in both modes (Weeztix), and an EU Accessibility Act-ready checkout.
8. **Open core like pretix, but with a modern extension model**: sandboxed UI slots + typed webhooks + rule functions instead of in-process Python plugins, so third parties cannot break checkout.
9. **A developer-grade widget** (Tito-level): custom elements, inline or overlay modes, typed events, prefill, UTM and consent pass-through, plus a headless API (vivenu) for big clients.
10. **Full white label including the back office and the scanner app** (Ticket Tailor and others explicitly cannot), so resellers can run EPS under their own name, like vivenu's private label.
