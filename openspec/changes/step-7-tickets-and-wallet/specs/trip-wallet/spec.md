# Spec Delta

## Purpose

Gives each fan one timeline of everything they hold across events and organisers (tickets, add-ons, tables, time slots, transport), each with the next thing to do and its deadline.

## ADDED Requirements

### Requirement: One timeline per fan
The trip wallet SHALL show every item the signed-in fan currently holds, across all organisers, grouped by event and ordered by start time. Items without their own time MUST sit at their event's start. Each event group MUST use that organiser's brand tokens.

#### Scenario: Mixed items for one event
- **WHEN** a fan holds a 3-day pass, a camping add-on, a table on Friday, a padel slot on Saturday at 11:00 and a bus seat (a time slot) on Thursday evening for one event
- **THEN** the wallet shows one event group with the bus seat first, then the pass and camping at the event start, then the table and the padel slot in time order
Test: apps/web/tests/unit/trip-timeline.test.ts

#### Scenario: Two organisers
- **WHEN** a fan holds tickets from two organisers
- **THEN** the wallet shows two event groups, each in its own brand's colours and logo, ordered by event date
Test: apps/web/tests/e2e/trip-wallet.spec.ts

#### Scenario: Nothing held yet
- **WHEN** a fan with no tickets opens the wallet
- **THEN** an empty state explains where tickets will appear, with no error
Test: apps/web/tests/e2e/trip-wallet.spec.ts

### Requirement: Only items the fan holds now
The wallet SHALL list only items whose current holder is the fan. An item reissued to someone else MUST leave the old holder's wallet and appear in the new holder's wallet.

#### Scenario: Ticket moves to another fan
- **WHEN** a ticket in fan A's wallet is reissued to fan B
- **THEN** it no longer appears in A's wallet and appears in B's wallet on the next load
Test: services/internal/tickets/trip_test.go

#### Scenario: Another fan's ticket ID in the URL
- **WHEN** a fan opens a ticket page with a ticket ID they do not hold
- **THEN** the page shows not found
Test: apps/web/tests/e2e/trip-wallet.spec.ts

### Requirement: Next action with its deadline
Each item SHALL show at most one next action, chosen by a fixed order with the earliest deadline first: pay an overdue instalment, personalise, pick the day, check in, then show the code on the event day. The wallet header MUST show the single most urgent action across all items.

#### Scenario: Personalise before check-in
- **WHEN** a ticket needs a name by 1 June and online check-in by 10 July
- **THEN** its next action is "Name this ticket by 1 June"
Test: apps/web/tests/unit/trip-next-action.test.ts

#### Scenario: Overdue instalment comes first
- **WHEN** a ticket's instalment plan is overdue and it also needs a name
- **THEN** its next action is "Pay overdue instalment" and the header shows it
Test: apps/web/tests/unit/trip-next-action.test.ts

#### Scenario: Event day
- **WHEN** it is the event day and every other action is done
- **THEN** the next action is "Show your code", which opens the live ticket
Test: apps/web/tests/unit/trip-next-action.test.ts

### Requirement: Pending tickets are shown honestly
Between payment and issuing, the wallet SHALL show the order's items as "Issuing" and refresh until they are issued, never as ready tickets.

#### Scenario: Order paid a moment ago
- **WHEN** the fan opens the wallet right after paying and the tickets are not issued yet
- **THEN** the items show "Issuing" and turn into tickets without a manual reload
Test: apps/web/tests/e2e/trip-wallet.spec.ts

### Requirement: Romanian and English
The trip wallet and the live ticket SHALL be available in Romanian and English with correct diacritics, and dates and prices MUST be formatted for the chosen language and the shop's currency.

#### Scenario: Romanian wallet
- **WHEN** the fan opens the wallet with Romanian chosen
- **THEN** all labels, actions and deadlines are in Romanian with diacritics and dates use Romanian format
Test: apps/web/tests/e2e/trip-wallet.spec.ts
