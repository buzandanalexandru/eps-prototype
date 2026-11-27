# Spec Delta

## Purpose

Lets an organiser set up each sale moment of an event (opening time, queue, admission rate, holds, caps, bot defence and OTP) and change the live controls while the sale runs.

## ADDED Requirements

### Requirement: Sale settings per sale moment
An organiser SHALL be able to create sale moments for a published event and set for each: opening time, pre-queue opening (5 to 120 minutes before), queue on or off, admission rate per minute, the limit of fans shopping at once, hold length (5 to 15 minutes), cap per order, cap per person, bot defence on or off, OTP on or off and its ticket threshold. Values outside these ranges SHALL be refused.

#### Scenario: Save settings
- **WHEN** an organiser sets opening at 12:00, pre-queue 30 minutes before, rate 2,000 per minute and caps of 4 per order and 6 per person
- **THEN** the settings are saved as a new version and the sale shows them
- Test: apps/web/tests/e2e/sale-settings.spec.ts

#### Scenario: Value out of range
- **WHEN** an organiser sets a hold length of 30 minutes
- **THEN** the save is refused with a message giving the allowed range
- Test: services/internal/sales/settings_test.go

### Requirement: Live controls change during the sale
The admission rate, the limit of fans shopping at once, pause, bot defence and the OTP switch SHALL be changeable while the sale runs and SHALL take effect within 2 seconds. Opening time, pre-queue opening and queue on or off SHALL NOT be changeable once the pre-queue has opened.

#### Scenario: Rate changed live
- **WHEN** the organiser changes the rate from 2,000 to 500 per minute during the sale
- **THEN** within 2 seconds admissions follow the new rate
- Test: services/internal/sales/settings_test.go

#### Scenario: Locked field
- **WHEN** the organiser tries to move the opening time after the pre-queue opened
- **THEN** the change is refused and the sale keeps its opening time
- Test: services/internal/sales/settings_test.go

### Requirement: Settings belong to one organiser
Only the sale's own organiser SHALL be able to read or change its settings.

#### Scenario: Other organiser
- **WHEN** organiser B tries to change a sale of organiser A
- **THEN** the request is refused and nothing changes
- Test: services/internal/sales/settings_test.go

### Requirement: Every change is recorded
Every settings change SHALL be kept as a version with who made it and when, so the sale's history can be read afterwards.

#### Scenario: History
- **WHEN** the rate is changed three times during a sale
- **THEN** the sale's history shows the three versions with their times and authors
- Test: services/internal/sales/settings_test.go
