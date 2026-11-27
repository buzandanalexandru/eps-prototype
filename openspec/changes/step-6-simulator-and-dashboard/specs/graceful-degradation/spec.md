# Spec Delta

## Purpose

Gives an organiser a switch that keeps a shop honest under extreme load: static event pages, a clear busy message instead of errors, and a pause on new checkouts while everyone keeps their place in the queue.

## ADDED Requirements

### Requirement: Degrade switch
An organiser SHALL be able to switch a sale into degraded mode from Studio or the live dashboard and back, taking effect within 10 seconds each way. The system SHALL only suggest the switch when load is high, never flip it on its own.

#### Scenario: Switch on
- **WHEN** the organiser turns on degraded mode
- **THEN** within 10 seconds the shop shows the static event page and the busy message
- Test: apps/web/tests/e2e/degrade.spec.ts

#### Scenario: Suggestion only
- **WHEN** core latency is above 2 seconds for 30 seconds
- **THEN** the dashboard suggests degraded mode and the mode stays off until the organiser chooses
- Test: services/internal/live/aggregator_test.go

### Requirement: Static pages and honest message
In degraded mode the event page SHALL be served as a static page with prices and line-up and an honest busy message in Romanian and English, and new holds SHALL be refused with `EPS-BUSY` and a retry time. Fans in the queue SHALL keep their place, and orders already held SHALL still be payable.

#### Scenario: Held order still pays
- **WHEN** degraded mode is on and a fan with a held order presses Pay
- **THEN** the payment goes through normally
- Test: services/internal/degrade/degrade_test.go

#### Scenario: New hold refused politely
- **WHEN** degraded mode is on and an admitted fan tries to hold tickets
- **THEN** the response is `EPS-BUSY` with a retry time and the page shows the busy message, not an error
- Test: manual check on the deployed preview (switch degraded mode on, open the shop on desktop and phone, see the static page and busy message in RO and EN)
