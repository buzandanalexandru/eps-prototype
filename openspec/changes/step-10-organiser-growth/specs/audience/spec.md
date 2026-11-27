# Spec Delta

## Purpose

Makes each organiser the owner of its buyer list: a table of its buyers with their consents and an export the organiser can take to its own tools.

## ADDED Requirements

### Requirement: Buyers table with consent flags
The system SHALL show the organiser a table of the people who paid for its orders (account and guest buyers), with email, name, number of orders, total spent and the consent flags given at checkout, and nobody else's buyers.

#### Scenario: Buyers of one organiser
- **WHEN** a fan buys from two different organisers and the first organiser opens its Audience screen
- **THEN** the fan appears with only the orders and consents from the first organiser
- Test: apps/web/tests/e2e/audience.spec.ts

### Requirement: CSV export respects consent
The system SHALL export the buyers table as CSV containing only buyers with marketing consent by default; an export of all buyers SHALL be available to owners only and recorded with who exported it and when.

#### Scenario: Default export
- **WHEN** an admin exports the audience of an organiser with 3 consenting and 2 non-consenting buyers
- **THEN** the CSV has 3 rows with a header row and consent columns
- Test: apps/web/tests/unit/audience-export.test.ts

#### Scenario: Spreadsheet formula in a name
- **WHEN** a buyer's name starts with `=`
- **THEN** the exported cell is prefixed so a spreadsheet does not run it as a formula
- Test: apps/web/tests/unit/audience-export.test.ts

### Requirement: Deleted people are not exported
The system SHALL exclude buyers who deleted their personal data from the table and from every export.

#### Scenario: Buyer deleted their account
- **WHEN** a buyer deletes their personal data and the organiser exports again
- **THEN** that buyer is not in the table or the CSV
- Test: apps/web/tests/unit/audience-export.test.ts
