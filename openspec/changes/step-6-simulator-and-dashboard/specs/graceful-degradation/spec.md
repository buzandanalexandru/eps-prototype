# Spec Delta

## Purpose

Gives an organiser a switch that keeps a shop honest under extreme load: static event pages with a clear busy message instead of errors, and a pause on new checkouts while everyone keeps their place in the queue.

## ADDED Requirements

### Requirement: Degrade switch
An organiser SHALL be able to switch a sale into degraded mode from the live dashboard and back, taking effect within 10 seconds each way. The system SHALL only suggest the switch when load is high, never flip it on its own.

#### Scenario: Switch on
- **WHEN** the organiser turns on degraded mode
- **THEN** within 10 seconds the event page shows the busy message in place of its buy controls, in Romanian or English
- Test: apps/web/tests/e2e/degrade.spec.ts

#### Scenario: Suggestion only
- **WHEN** core latency is above 2 seconds in each of the last 30 frames
- **THEN** the frame suggests degraded mode and the mode stays off until the organiser chooses
- Test: services/internal/live/aggregator_test.go

### Requirement: Static pages and honest message
In degraded mode the event page SHALL stay a static page with prices and line-up and show an honest busy message in Romanian and English; new holds SHALL be refused with `EPS-BUSY` and a retry time; the waiting room SHALL let no one in and keep everyone's place; and orders already held SHALL still be payable.

#### Scenario: Held order still pays
- **WHEN** degraded mode is on and a fan with a held order pays
- **THEN** the payment goes through normally
- Test: services/internal/onsale/degrade_test.go

#### Scenario: New hold refused politely
- **WHEN** degraded mode is on and an admitted fan tries to hold tickets
- **THEN** the answer is `EPS-BUSY` with a retry time of 30 seconds
- Test: services/internal/onsale/degrade_test.go

#### Scenario: Queue keeps its place
- **WHEN** degraded mode is on while fans wait in the line
- **THEN** no one is admitted, their positions do not change and their status says the shop is busy; when it is off, admissions resume in the same order
- Test: services/internal/onsale/degrade_test.go
