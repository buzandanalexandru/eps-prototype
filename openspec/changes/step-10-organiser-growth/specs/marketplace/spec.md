# Spec Delta

## Purpose

Gives fans one public page where the published events of every organiser on EPS can be discovered, each linking to that organiser's own branded shop.

## ADDED Requirements

### Requirement: Public marketplace page
The system SHALL serve a public page at `/marketplace`, in Romanian and English, listing the published events of every organiser that has not switched marketplace listing off, with date, venue, organiser brand, the lowest all-in price and a link to the brand's shop.

#### Scenario: Events of two organisers
- **WHEN** two organisers have published events and both are listed
- **THEN** the page shows both organisers' events and each card links to `/s/<brand>`
- Test: apps/web/tests/e2e/marketplace.spec.ts

#### Scenario: Unpublished or unlisted events
- **WHEN** an event is only a draft, or its organiser switched marketplace listing off
- **THEN** the event does not appear on the page
- Test: apps/web/tests/e2e/marketplace.spec.ts

### Requirement: Sample data labelled
The system SHALL label events of the sample organiser as sample data on the marketplace page.

#### Scenario: Sample NIBIRU events
- **WHEN** a visitor opens the marketplace
- **THEN** the sample NIBIRU events carry a visible sample data label
- Test: manual check on the deployed preview (sample label on the NIBIRU cards, RO and EN)
