# Spec Delta

## Purpose

Provides the demo's data: NIBIRU's 2027 catalogue rebuilt as organiser #1 and a small second organiser, created through the same rules as Studio, repeatable on every environment and always labelled as sample data.

## ADDED Requirements

### Requirement: NIBIRU sample organiser
The seed SHALL create a sample organiser "NIBIRU" with two brands, "NIBIRU Festivals" (festival template) and "NIBIRU Resort" (resort template), both in Romanian and English, priced in RON, with fee policy "pass on", and the demo organiser user as owner.

#### Scenario: After seeding
- **WHEN** the seed has run and the demo organiser signs in
- **THEN** Studio shows organiser NIBIRU with both brands, the demo organiser is owner, and the fee policy is "pass on"
Test: apps/web/tests/e2e/sample-data.spec.ts

### Requirement: Galaxia 2027 festival passes by zone
The seed SHALL create Galaxia 2027 (15 to 18 Jul 2027, NIBIRU Arena) with passes GA 539, GA Plus 659, Golden Circle 839, VIP 939 and Ultra VIP 1,239 RON face price, GA and GA Plus sharing one Arena pool, a camping add-on with its own pool, named tickets, and instalments of 10 RON now plus up to 7 monthly payments (capped per buyer, so a buyer in early December 2026 gets all 7).

#### Scenario: Galaxia prices all-in
- **WHEN** the seeded GA pass is shown in Studio
- **THEN** its face price is 539.00 RON, the fan pays 552.48 RON, it shares the Arena pool with GA Plus, and its instalment schedule starts with 10.00 RON
Test: apps/web/tests/unit/seed-nibiru.test.ts

#### Scenario: Camping needs a ticket
- **WHEN** the seeded camping add-on is inspected
- **THEN** it requires any Galaxia ticket and draws on the camping pool only
Test: apps/web/tests/unit/seed-nibiru.test.ts

### Requirement: Resort products
The seed SHALL create NIBIRU Resort's season products: Beer Garden concert tables in categories C 79, B 99 and A 149 RON with up to 4 equal monthly payments; Promenada access with a pick-a-day ticket on a date ladder from 10 to 35 RON and an Any-Day ticket from 15 RON drawing on an event pool; a Center Stage evening ticket that is a 25 RON drink voucher; padel at 50 RON per 30-minute slot; a stand-up night for ages 16 and up.

#### Scenario: Promenada ladder
- **WHEN** the seeded Promenada pick-a-day ticket is read on 15 Dec 2026
- **THEN** a date ladder from 10 RON to 35 RON on 24 Jul 2027 is in force, with a day-scoped pool per season day
Test: apps/web/tests/unit/seed-nibiru.test.ts

#### Scenario: Special types in use
- **WHEN** the seeded resort products are listed
- **THEN** they include a table product per category, an Any-Day flexible ticket, a drink voucher worth 25 RON and a 30-minute padel time slot with a slot-scoped pool
Test: apps/web/tests/unit/seed-nibiru.test.ts

### Requirement: A second sample organiser
The seed SHALL create a second, unrelated sample organiser "Sunwave" with one brand and one small event, so the demo shows two organisers and their separation.

#### Scenario: Organisers stay apart
- **WHEN** the demo organiser of NIBIRU opens Studio
- **THEN** nothing of Sunwave is visible, and Sunwave's brand has its own address
Test: apps/web/tests/e2e/sample-data.spec.ts

### Requirement: Seed uses the same rules as Studio
Every seeded record SHALL pass the same validation as a record entered in Studio, and every seeded event MUST pass the readiness check.

#### Scenario: Seeded events are ready
- **WHEN** the seed has run
- **THEN** the readiness check reports no problems for every seeded event
Test: apps/web/tests/unit/seed-nibiru.test.ts

### Requirement: Seed is repeatable and touches only sample organisers
Running the seed again SHALL restore the sample organisers to their defined state without duplicates, removing only unpublished rows a visitor added, and MUST NOT change any organiser that is not flagged as sample, nor any published event of a sample organiser or anything that belongs to it. Previews are seeded on every push; production is seeded only on request.

#### Scenario: Run twice
- **WHEN** the seed runs twice on the same database
- **THEN** the second run creates no new rows and the catalogue equals the first run's
Test: apps/web/tests/unit/seed-nibiru.test.ts

#### Scenario: Published event untouched
- **WHEN** a seeded event has been published and the seed runs again
- **THEN** none of that event's rows change
Test: apps/web/tests/unit/seed-nibiru.test.ts

#### Scenario: Visitor changes are undone
- **WHEN** a visitor moved a seeded event to a new venue, added a day outside its dates and changed prices, and the seed runs again
- **THEN** the seeded catalogue is restored and the visitor's unused venue is removed
Test: apps/web/tests/unit/seed-nibiru.test.ts

#### Scenario: Real organiser untouched
- **WHEN** a real organiser exists and the seed runs
- **THEN** none of its rows change
Test: apps/web/tests/unit/seed-nibiru.test.ts

### Requirement: Sample data is labelled
Every Studio screen of a sample organiser SHALL show a visible "Sample data" label in the active language.

#### Scenario: Label in Romanian
- **WHEN** the demo organiser views NIBIRU in Romanian
- **THEN** the header shows "Date de exemplu"
Test: apps/web/tests/e2e/sample-data.spec.ts
