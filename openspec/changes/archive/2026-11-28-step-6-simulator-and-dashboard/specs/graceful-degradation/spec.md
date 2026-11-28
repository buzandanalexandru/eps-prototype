# Spec Delta

## Purpose

Gives an organiser a switch that keeps a shop honest under extreme load: static event pages with a clear busy message instead of errors, and a pause on new checkouts while everyone keeps their place in the queue.

## ADDED Requirements

### Requirement: Degrade switch
An organiser SHALL be able to switch a sale into degraded mode from the live dashboard and back, taking effect within 10 seconds each way. The system SHALL only suggest the switch when load is high (core errors counted without its own `EPS-BUSY` refusals), never flip it on its own, and SHALL drop the suggestion while degraded mode is on. In production the shared sample organiser's real sale SHALL NOT be switched (its rehearsals can be). Every switch SHALL be logged with who made it.

#### Scenario: Switch on
- **WHEN** the organiser turns on degraded mode
- **THEN** within 10 seconds of the switch the event page shows the busy message in place of its buy controls, in Romanian or English, and within 10 seconds of switching off buying returns
- Test: apps/web/tests/e2e/degrade.spec.ts

#### Scenario: Suggestion only
- **WHEN** core latency is above 2 seconds in each of the last 30 frames
- **THEN** the frame suggests degraded mode and the mode stays off until the organiser chooses
- Test: services/internal/live/aggregator_test.go

### Requirement: Static pages and honest message
In degraded mode the event page SHALL stay a static page with prices and line-up and show an honest busy message in Romanian and English ("Your place in the queue is kept; buying resumes when the load eases." / "Locul tău în coadă este păstrat; cumpărarea se reia când scade aglomerația."); new holds SHALL be refused with `EPS-BUSY` and a retry time; the waiting room SHALL let no one in and keep everyone's place; and orders already held SHALL still be payable.

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

### Requirement: Admitted fans keep their turn
A fan admitted before degraded mode was switched on SHALL keep the right to buy once it is off, even if their pass expired while degraded.

#### Scenario: Admitted fans kept
- **WHEN** a fan's pass expires while degraded mode is on
- **THEN** once it is off (within the length of the degraded window) the fan can still hold tickets with that pass
- Test: services/internal/onsale/degrade_test.go
