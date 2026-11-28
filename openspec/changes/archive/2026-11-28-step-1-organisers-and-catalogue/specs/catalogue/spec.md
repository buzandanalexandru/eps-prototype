# Spec Delta

## Purpose

Lets an organiser describe what they sell without code: venues, events with days and line-ups, product types from festival passes to padel slots, shared capacity pools, honest price ladders, buying rules and instalment plans, plus a readiness check before anything can be published.

## ADDED Requirements

### Requirement: Venues are reusable
An organiser SHALL be able to create venues (name, address, map position, stages) and reuse them across events and brands of the same organiser.

#### Scenario: Reuse a venue
- **WHEN** the organiser creates venue "NIBIRU Arena" and then two events
- **THEN** both events can pick "NIBIRU Arena" without re-entering it
Test: apps/web/tests/unit/catalogue-events.test.ts

### Requirement: Events with days and line-up
An organiser SHALL be able to create an event for one of its brands with a name and description in each brand language, a venue, a time zone, a start and end, its days with opening hours, and a line-up of artists by day, stage and time.

#### Scenario: Multi-day festival
- **WHEN** the organiser creates "Galaxia 2027" from 15 to 18 Jul 2027, open 17:00 to 03:00, with artists on two stages
- **THEN** the event has four days, each closing at 03:00 the next morning but belonging to its start date, and the line-up is listed per day and stage in time order
Test: apps/web/tests/unit/catalogue-events.test.ts

#### Scenario: End before start
- **WHEN** the end is before the start, or a day lies outside the event dates
- **THEN** the save is refused with the field in error
Test: apps/web/tests/unit/catalogue-events.test.ts

#### Scenario: Missing translation
- **WHEN** the brand sells in Romanian and English and the English name is empty
- **THEN** the event saves as a draft and the readiness check lists the missing translation
Test: apps/web/tests/unit/catalogue-readiness.test.ts

### Requirement: Copy an event as a new edition
An organiser SHALL be able to copy an event as a new edition, with its days, line-up, products, pools, price ladders and instalment plans, every date shifted by a chosen offset, as a new draft that keeps a link to the original.

#### Scenario: Next year's edition
- **WHEN** the organiser copies "Galaxia 2027" as a new edition one year later
- **THEN** a draft "Galaxia 2028" exists with all products and pools, dates on the same weekdays in 2028, ladder dates shifted by the same offset, and no sales
Test: apps/web/tests/unit/catalogue-editions.test.ts

#### Scenario: Edition address already taken
- **WHEN** the organiser copies "Galaxia 2027" a second time
- **THEN** the second copy's address keeps the year and adds a number (`galaxia-2028-2`)
Test: apps/web/tests/unit/review-fixes.test.ts

### Requirement: Core product types
An event SHALL support multi-day passes by zone, day tickets for one day (a set day, or, when the day is left empty, the day the buyer picks when buying), and add-ons such as camping or parking that MAY require a ticket in the same order. Each product has a name per language, perks, a named-ticket flag, a sales window, visibility, a maximum per order, and optional personalisation and check-in deadlines and a late check-in fee used after purchase.

#### Scenario: Pass by zone
- **WHEN** the organiser creates "Golden Circle" as a pass for all four days in zone "Golden Circle" with perks "fast lane" and "VIP toilets"
- **THEN** the product shows its zone, days and perks in Studio
Test: apps/web/tests/unit/catalogue-products.test.ts

#### Scenario: Add-on requiring a ticket
- **WHEN** an add-on "Camping" is set to require any ticket of the event
- **THEN** the product records the requirement and the readiness check flags it if the event has no ticket product
Test: apps/web/tests/unit/catalogue-readiness.test.ts

#### Scenario: Sales window ends before it starts
- **WHEN** a product's sales end is before its sales start
- **THEN** the save is refused
Test: apps/web/tests/unit/catalogue-products.test.ts

### Requirement: Special product types
The model SHALL also support a spend voucher (a ticket worth a stated spend), a table by category with seats, a time slot of fixed length between a first and last start time, and a flexible day ticket whose day is picked later from a set of days.

#### Scenario: Each special type
- **WHEN** the organiser creates a 25 RON drink voucher, a category A table for 4, a 30-minute padel slot from 17:00 to 02:30 allowing up to 4 consecutive slots, and an any-day Promenada ticket
- **THEN** each is saved with its type settings and listed in Studio with a short explanation of how it sells
Test: apps/web/tests/unit/catalogue-products.test.ts

#### Scenario: Invalid type settings
- **WHEN** a time slot's length does not divide the span from first to last start, or a voucher has no spend value
- **THEN** the save is refused with the setting in error
Test: apps/web/tests/unit/catalogue-products.test.ts

### Requirement: Shared capacity pools
An organiser SHALL be able to create capacity pools and link each product to one or more of them, and a pool to several products. A pool MAY be scoped to the event, to each day, or to each time slot. A product MUST count as available only while every pool it draws on has room.

#### Scenario: Two products share one pool
- **WHEN** GA and GA Plus both draw on pool "Arena" of 40,000
- **THEN** Studio shows the Arena pool bar with both products drawing on it and a combined capacity of 40,000
Test: apps/web/tests/unit/catalogue-pools.test.ts

#### Scenario: Product in two pools
- **WHEN** "VIP" draws on pool "VIP platform" of 2,000 and pool "Arena" of 40,000
- **THEN** its available quantity is the smaller remaining amount of the two
Test: apps/web/tests/unit/catalogue-pools.test.ts

#### Scenario: Day-scoped pool
- **WHEN** pool "Promenada" of 8,000 is scoped to each day of a 74-day season
- **THEN** each day has its own 8,000 and selling one day never reduces another
Test: apps/web/tests/unit/catalogue-pools.test.ts

#### Scenario: Product without a pool or a zero pool
- **WHEN** a product draws on no pool, or a pool has capacity 0
- **THEN** the readiness check lists it
Test: apps/web/tests/unit/catalogue-readiness.test.ts

### Requirement: Pool scopes match products
Passes, flexible days and add-ons MUST draw on event pools only, a per-slot pool SHALL serve only time slots, and every time slot on one slot pool MUST use the same time grid; the readiness check lists any other combination. A product and a pool it draws on MUST belong to the same event.

#### Scenario: Pool scope and slot grid
- **WHEN** a pass draws on a per-day pool, or two time slots with different start times share one slot pool
- **THEN** the readiness check lists each
Test: apps/web/tests/unit/review-fixes.test.ts

### Requirement: Price ladder by date or by sales
A product's price SHALL be a ladder of steps, either by date (each step from a time) or by sales (each step after a number sold), never both. The price in force MUST depend only on the time and the quantity sold, and storefronts show a fixed public note saying so.

#### Scenario: Date ladder
- **WHEN** the Promenada day ticket has steps 10 RON, 11 RON from 1 Dec 2026, 13 RON from 1 Jan 2027 and 35 RON from 24 Jul 2027
- **THEN** on 15 Dec 2026 the price in force is 11 RON and the next step is 13 RON from 1 Jan 2027
Test: apps/web/tests/unit/pricing.test.ts

#### Scenario: Sales ladder
- **WHEN** GA has steps 539 RON, then 599 RON after 10,000 sold
- **THEN** with 9,999 sold the price is 539 RON and with 10,000 sold it is 599 RON
Test: apps/web/tests/unit/pricing.test.ts

#### Scenario: Unordered or mixed ladder
- **WHEN** step dates or sales thresholds are not strictly increasing, or one ladder mixes dates and sales
- **THEN** the save is refused
Test: apps/web/tests/unit/catalogue-products.test.ts

#### Scenario: Empty first price
- **WHEN** the organiser clears the first step's price but keeps later steps
- **THEN** the save is refused instead of promoting a later step
Test: apps/web/tests/unit/review-fixes.test.ts

#### Scenario: Before the first threshold
- **WHEN** a date ladder is read before any step date
- **THEN** the first step, which has no threshold, is in force
Test: apps/web/tests/unit/pricing.test.ts

### Requirement: Buying rules
An event SHALL record a minimum age, an age under which parental consent is required, and a maximum per account; each product SHALL record a maximum per order. These are settings here; enforcement belongs to checkout and the on-sale engine, where the maxima are the defaults of a sale and the stricter of these and the sale settings applies.

#### Scenario: Stand-up age limit
- **WHEN** the organiser sets minimum age 16 on a stand-up night
- **THEN** the setting is saved and shown in Studio's buying rules panel and its summary
Test: apps/web/tests/e2e/catalogue.spec.ts

#### Scenario: Inconsistent rule
- **WHEN** the consent age is below the minimum age, or a maximum is 0 or negative
- **THEN** the save is refused
Test: apps/web/tests/unit/catalogue-rules.test.ts

### Requirement: Instalment settings per product
An organiser SHALL be able to offer instalments on a product as a deposit (a fixed amount or a share of the all-in price) plus up to a number of equal monthly payments. The number each buyer gets SHALL be computed from their purchase date: payments fall on a day of the month in the event's time zone, and the last MUST fall at least 7 local days before the event starts. Studio SHALL show the next buyer's schedule and from which date buyers get how many payments.

#### Scenario: NIBIRU-style plan
- **WHEN** a 539.00 RON pass (all-in 552.48 RON) offers 10 RON now and up to 7 monthly payments, and a buyer purchases on 5 Dec 2026 for an event on 15 Jul 2027
- **THEN** the schedule shows 10.00 RON now and 7 payments from 1 Jan to 1 Jul 2027 that add up to 542.48 RON, any rounding remainder on the last one
Test: apps/web/tests/unit/instalments.test.ts

#### Scenario: Plan capped per buyer
- **WHEN** the same plan is bought on 10 Mar 2027, or on 1 Jul 2027
- **THEN** the first buyer gets 4 monthly payments and the second no instalments, and Studio lists the dates from which buyers get each number
Test: apps/web/tests/unit/instalments.test.ts

#### Scenario: Plan runs past the event
- **WHEN** the organiser saves more monthly payments than fit for the next buyer before the 7-day limit
- **THEN** the save is refused with the latest possible number of payments
Test: apps/web/tests/unit/instalments.test.ts

#### Scenario: Deposit too large
- **WHEN** the deposit is not below the all-in price
- **THEN** the save is refused
Test: apps/web/tests/unit/instalments.test.ts

### Requirement: Readiness check before publishing
Studio SHALL list, per event, every problem that would stop it from being published, each with a code and a link to the field to fix. An event with no problems MUST show as ready.

#### Scenario: Problems listed
- **WHEN** an event has a product without a pool and a missing English description
- **THEN** the checklist shows both problems with links, and the event is not ready
Test: apps/web/tests/unit/catalogue-readiness.test.ts

#### Scenario: Ready
- **WHEN** every problem is fixed
- **THEN** the checklist shows the event as ready
Test: apps/web/tests/unit/catalogue-readiness.test.ts

### Requirement: Catalogue data stays with its organiser
Every catalogue record SHALL belong to exactly one organiser, and a product, pool or event MUST NOT be linked to a record of another organiser.

#### Scenario: Linking across organisers
- **WHEN** a request tries to link organiser A's product to organiser B's pool
- **THEN** the database refuses it and Studio reports not found
Test: apps/web/tests/unit/catalogue-isolation.test.ts

#### Scenario: Linking across events
- **WHEN** a product is linked to a pool of another event of the same organiser, or a line-up slot to another event's day
- **THEN** the database refuses it
Test: apps/web/tests/unit/catalogue-isolation.test.ts

#### Scenario: Saving with another organiser's id
- **WHEN** a request saves a product or pool using the id of another organiser's row
- **THEN** nothing changes and Studio reports not found; new rows always get server-generated ids
Test: apps/web/tests/unit/catalogue-isolation.test.ts
