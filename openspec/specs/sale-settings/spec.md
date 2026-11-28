# sale-settings Specification

## Purpose
Lets an organiser set up each sale moment of an event (opening time, queue, admission rate, holds, caps, bot defence and OTP) and change the live controls while the sale runs.

## Requirements

### Requirement: Sale settings per sale moment
An organiser SHALL be able to create sale moments for a published event and set for each: opening time, pre-queue opening (5 to 120 minutes before), queue on or off, admission rate per minute, the limit of fans shopping at once, hold length (5 to 15 minutes), cap per order, cap per person, bot defence on or off, OTP on or off and its ticket threshold, and whether only signed-in fans may join. Values outside these ranges SHALL be refused.

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

### Requirement: Sale moment limits
The opening SHALL be within a year of now, an event SHALL have at most 10 sale moments, and a sale moment SHALL be deletable until its pre-queue opens.

#### Scenario: Limits and deletion
- **WHEN** an organiser adds an 11th sale moment, an opening 13 months away, or deletes a sale whose pre-queue opened
- **THEN** each is refused, while deleting a sale before its pre-queue opens succeeds
- Test: services/internal/sales/settings_test.go

### Requirement: Saves never undo each other
A save SHALL be refused when it was made on an older version than the current one, and the live controls (rate, shopping limit, pause) SHALL also be changeable on top of whatever version is current.

#### Scenario: Two organisers at once
- **WHEN** two organisers edit version 1 and the second saves after the first
- **THEN** the second save is refused with the newer version shown, and a pause applies on top of the newer version
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
