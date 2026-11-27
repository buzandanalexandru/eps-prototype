# Spec Delta

## Purpose

Serves each brand's published shop to fans at `/s/<brand>` and on any configured hostname, fast and cached until the next publish, with the price the fan will pay shown everywhere and a ticket picker that hands a valid selection to checkout.

## ADDED Requirements

### Requirement: Published shop at its brand address
Each brand with a published storefront SHALL be served at `/s/<brand>`, which MUST redirect to the shop in the visitor's language (cookie, then browser language, then the shop default, among the shop's languages). Unknown and unpublished brands MUST give the same not-found page.

#### Scenario: Visitor in Romanian
- **WHEN** a visitor whose browser prefers Romanian opens `/s/nibiru-festivals`
- **THEN** they are redirected to `/s/nibiru-festivals/ro` and see the published home page in Romanian with diacritics
Test: apps/web/tests/e2e/storefront.spec.ts

#### Scenario: Brand with no published version
- **WHEN** a visitor opens the address of a brand that has only a draft, or of a brand that does not exist
- **THEN** both get the same 404 page and no draft content
Test: apps/web/tests/e2e/storefront.spec.ts

#### Scenario: Language not offered
- **WHEN** a visitor opens `/s/<brand>/en` for a shop that sells only in Romanian
- **THEN** they are redirected to the Romanian page
Test: apps/web/tests/unit/storefront-routing.test.ts

### Requirement: The same shop on a configured hostname
A request whose host is configured for a brand SHALL be served that brand's shop at the root of the host, with links that stay on that host; a request for an unknown host MUST get the platform's not-found page.

#### Scenario: Brand hostname
- **WHEN** a request arrives for a hostname configured for Sunwave with path `/en`
- **THEN** it shows the same page as `/s/sunwave/en` and its links point to the hostname
Test: apps/web/tests/unit/storefront-routing.test.ts

#### Scenario: Unknown host
- **WHEN** a request arrives for a hostname that no brand has
- **THEN** it gets the platform 404 and no brand content
Test: apps/web/tests/unit/storefront-routing.test.ts

### Requirement: Shop pages render the published version
The shop SHALL render its home, event and information pages from the live storefront version and the live sale snapshots, in the brand's tokens, and MUST NOT show anything from drafts.

#### Scenario: Draft edits after publishing
- **WHEN** the organiser changes the hero title in the draft without publishing
- **THEN** the public shop still shows the published title
Test: apps/web/tests/e2e/storefront.spec.ts

#### Scenario: Event page
- **WHEN** a fan opens `/s/nibiru-festivals/en/e/galaxia-2027`
- **THEN** it shows the event's dates, venue, line-up and ticket grid from the live snapshot, in the brand's colours and fonts
Test: apps/web/tests/e2e/storefront.spec.ts

### Requirement: Shop pages are cached until the next publish
Shop pages SHALL be served from cache after their first render and MUST show the new version on the first request after a publish, without a deploy.

#### Scenario: Publish refreshes the shop
- **WHEN** the organiser publishes a new hero title
- **THEN** the next request to the shop shows the new title
Test: apps/web/tests/e2e/publish.spec.ts

#### Scenario: Cache hit
- **WHEN** a shop page is requested a second time with no publish in between
- **THEN** it is served without querying the database
Test: apps/web/tests/unit/storefront-cache.test.ts

### Requirement: All-in prices everywhere
Every price a fan sees in the shop SHALL be the all-in amount they will pay, taken from the live snapshot, including "from" prices, ticket grid prices, product details and picker totals. No fan-facing price MUST exclude a fee.

#### Scenario: Pass on policy
- **WHEN** GA has face price 539.00 RON with fee policy "pass on"
- **THEN** the ticket grid, the event's "from" price and the picker all show 552.48 RON, and the product detail adds "includes platform fee of 13.48 RON"
Test: apps/web/tests/e2e/storefront-prices.spec.ts

#### Scenario: Absorb policy
- **WHEN** the event's policy is absorb
- **THEN** the shop shows the face price, which is the all-in price, and no fee line
Test: apps/web/tests/unit/storefront-prices.test.ts

#### Scenario: "From" price
- **WHEN** an event has visible products on sale at 552.48, 675.48 and 859.98 RON all-in and a hidden one at 100 RON
- **THEN** its "from" price is 552.48 RON
Test: apps/web/tests/unit/storefront-prices.test.ts

### Requirement: Honest price ladder notes
For a product on a date ladder the shop SHALL show the next step's all-in price and date; for a sales ladder it SHALL say only that the price may rise as tickets sell. Every ladder MUST show the platform note that prices depend only on date or sales, never on the buyer.

#### Scenario: Promenada next step
- **WHEN** on 15 Dec 2026 the Promenada ticket is in its 11 RON step and the next step is 13 RON from 1 Jan 2027
- **THEN** the shop shows the current all-in price, "Price rises to" the next all-in price "on 1 Jan 2027", and the platform note
Test: apps/web/tests/unit/storefront-prices.test.ts

### Requirement: Instalment summaries match the all-in price
Where a product offers instalments, the shop SHALL show the deposit, the number of monthly payments and their amount, computed from the all-in price, and the summary MUST add up to the all-in price.

#### Scenario: Galaxia GA instalments
- **WHEN** GA (552.48 RON all-in) offers 10 RON now and 9 monthly payments
- **THEN** the shop shows "10.00 RON now + 9 × 60.27 RON" with a note that the last payment is 60.32 RON, totalling 552.48 RON
Test: apps/web/tests/unit/storefront-prices.test.ts

### Requirement: Live availability in a cached page
The shop SHALL show each product's live sale status (on sale, not yet on sale with its date, ended, sold out) and the remaining quantity only when few are left, fetched from the core without making the page itself uncacheable. If the core cannot be reached the page MUST still render with buying paused.

#### Scenario: Not yet on sale
- **WHEN** a product's sales window opens on 1 Feb 2027
- **THEN** its card says "On sale from 1 Feb 2027" and it cannot be added
Test: apps/web/tests/e2e/storefront.spec.ts

#### Scenario: Core unreachable
- **WHEN** the availability request fails
- **THEN** the page shows its content with prices and a "Tickets are temporarily unavailable, try again shortly" note, with no console errors
Test: apps/web/tests/e2e/storefront.spec.ts

### Requirement: Ticket picker hands a valid selection to checkout
The ticket picker SHALL let a fan choose quantities within each product's maximum per order, sales window and availability, require a ticket for add-ons that need one, show the running all-in total, and pass the selection with the snapshot version to the shop's checkout route.

#### Scenario: Running total
- **WHEN** a fan picks 2 GA and 1 Camping
- **THEN** the picker shows the sum of the three all-in unit prices and "Continue" opens the checkout route with the event, snapshot version and items
Test: apps/web/tests/e2e/storefront-prices.spec.ts

#### Scenario: Above the maximum per order
- **WHEN** GA allows 6 per order and the fan presses + a seventh time
- **THEN** the quantity stays at 6 and the picker says the limit
Test: apps/web/tests/unit/ticket-picker.test.ts

#### Scenario: Add-on without a ticket
- **WHEN** the fan picks Camping alone
- **THEN** "Continue" is disabled with "Camping needs a Galaxia ticket"
Test: apps/web/tests/unit/ticket-picker.test.ts

#### Scenario: Time slot and day choice
- **WHEN** a fan picks a padel slot or a pick-a-day Promenada ticket
- **THEN** the picker asks for the date and start time or the day, and the selection carries that pool instance
Test: apps/web/tests/unit/ticket-picker.test.ts

### Requirement: Shops work on phones and in both languages
Every shop page SHALL work on desktop and phone without horizontal scroll, switch between the shop's languages, and format money and dates for the active language and the event's time zone.

#### Scenario: Phone in English
- **WHEN** a fan opens the event page on a 390 px wide screen in English
- **THEN** there is no horizontal scroll, the ticket grid stacks, and prices read like "RON 552.48"
Test: apps/web/tests/e2e/storefront.spec.ts

### Requirement: Sample shops are labelled
The shop of a sample organiser SHALL show a visible sample-data banner in the active language and MUST NOT be indexed by search engines.

#### Scenario: NIBIRU sample shop
- **WHEN** a visitor opens the NIBIRU sample shop in Romanian
- **THEN** a banner says "Date de exemplu" and the page carries a noindex directive
Test: apps/web/tests/e2e/storefront.spec.ts
