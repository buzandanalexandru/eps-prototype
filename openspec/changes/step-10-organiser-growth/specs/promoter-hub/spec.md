# Spec Delta

## Purpose

Lets an organiser grow sales through influencers, promoters and partners: discount, promo and access codes, tracked links, sales attributed to each, a live leaderboard and a list of commissions owed.

## ADDED Requirements

### Requirement: Promoters with codes and tracked links
The system SHALL let an organiser create promoters (influencer, promoter or partner) and give each one or more discount codes and tracked links for an event or a shop, with a commission rate per promoter.

#### Scenario: Create an influencer with a code and a link
- **WHEN** the organiser creates influencer "Ana" with code ANA10 (10% off) and a tracked link for the Galaxia shop
- **THEN** the hub lists Ana with the code, a copyable link of the form `/s/<brand>/<shop>?ref=<slug>` and zero sales
- Test: apps/web/tests/e2e/promoter-hub.spec.ts

### Requirement: Promo and access codes
The system SHALL support plain promo codes (percentage or fixed discount) and access codes that unlock hidden products or an access-only shop, each with an optional usage limit and validity window; codes SHALL be case-insensitive and unique per event.

#### Scenario: Access code unlocks a presale product
- **WHEN** a fan enters a valid partner access code on a shop
- **THEN** the hidden presale product appears with its all-in price and can be added to the cart
- Test: services/core/internal/codes/codes_test.go

#### Scenario: Code used up or expired
- **WHEN** a fan enters a code whose usage limit is reached or whose window has ended
- **THEN** the code is refused with a clear message and the cart keeps its normal prices
- Test: services/core/internal/codes/codes_test.go

#### Scenario: Usage limit under concurrent use
- **WHEN** 50 fans apply a code with a limit of 10 at the same moment
- **THEN** exactly 10 holds get the discount and the rest are refused
- Test: services/core/internal/codes/codes_test.go

### Requirement: Discount shown all-in before checkout
The system SHALL show the discounted all-in price in the cart as soon as a code is applied, so no new amount appears at payment.

#### Scenario: Discount in the cart
- **WHEN** a fan applies ANA10 to a 539 RON all-in ticket
- **THEN** the cart shows 485.10 RON all-in and the payment step charges 485.10 RON
- Test: apps/web/tests/e2e/promoter-hub.spec.ts

### Requirement: Sales attributed to codes and links
The system SHALL attribute each paid order to at most one promoter: a typed code wins over a tracked link, and among links the last one clicked within 7 days wins. The attribution SHALL be stored on the order and carried on the order-paid event.

#### Scenario: Link then code
- **WHEN** a fan arrives through Ana's link and then types promoter Bogdan's code before paying
- **THEN** the paid order is attributed to Bogdan's code
- Test: services/core/internal/codes/attribution_test.go

#### Scenario: Expired or unpaid orders
- **WHEN** an order attributed to Ana expires without payment
- **THEN** Ana's sales count does not change
- Test: services/worker/internal/rollup/promoter_test.go

### Requirement: Live leaderboard
The system SHALL show a leaderboard per event ranking promoters by tickets sold and net revenue, updating within 3 seconds of each attributed paid order, including orders from the simulator.

#### Scenario: Leaderboard moves during the simulated on-sale
- **WHEN** the simulator sends fans through three promoters' links during an on-sale
- **THEN** the leaderboard counts rise live and the order of promoters matches their attributed ticket counts
- Test: apps/web/tests/e2e/promoter-hub.spec.ts

### Requirement: Promoter payouts list
The system SHALL compute, per promoter, the commission owed (commission rate × attributed net revenue) and list it with a status of owed or marked paid; marking paid is a record only and moves no money.

#### Scenario: Commission owed
- **WHEN** Ana has 10% commission and 10,000 RON attributed net revenue
- **THEN** the payouts list shows 1,000 RON owed to Ana, and the organiser can mark it paid (simulated)
- Test: manual check on the deployed preview (payouts list on the Promoters screen shows the commission and the mark-paid state)

### Requirement: Read-only promoter stats link
The system SHALL let the organiser share a revocable read-only link that shows one promoter only their own codes, links and sales.

#### Scenario: Revoked link
- **WHEN** the organiser revokes Ana's stats link and someone opens it
- **THEN** the page refuses access and shows no figures
- Test: apps/web/tests/unit/promoter-stats-link.test.ts
