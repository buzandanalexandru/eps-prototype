# Spec Delta

## Purpose

Lets fans join a waitlist for a sold-out product with a simulated card pre-authorisation, so a returned or resold ticket goes automatically to the next person in line instead of to the fastest clicker.

## ADDED Requirements

### Requirement: Join the waitlist with a pre-authorisation
A signed-in fan SHALL be able to join the waitlist of a sold-out product for one ticket, within their cap per person, by pre-authorising the product's face value on a simulated provider. No money MUST be taken at joining, and the fan MUST see their place in line.

#### Scenario: Join a sold-out product's waitlist
- **WHEN** a fan joins the waitlist for a sold-out GA pass and the simulated authorisation succeeds
- **THEN** the fan is on the list with their position, and the authorisation shows as held, not charged
Test: manual check on the deployed preview (join the waitlist of a sold-out sample product and see the position)

#### Scenario: Authorisation refused
- **WHEN** the simulated provider refuses the pre-authorisation
- **THEN** the fan is not added and is asked to try another method
Test: services/internal/waitlist/join_test.go

### Requirement: Returned tickets go to the first person automatically
When a ticket returns to a product with a non-empty waitlist (a new resale listing or a ticket the organiser returns to sale), the system SHALL hold it for the first person in line before anyone else can buy it, capture their pre-authorisation for its price, and give them the ticket. If the capture fails, the system MUST remove that entry and offer the ticket to the next person.

#### Scenario: A resale listing fills the waitlist
- **WHEN** a holder lists a ticket for a product whose waitlist has 3 fans
- **THEN** the first fan is charged the listing price, the ticket is reissued to them, the seller is paid out, and the listing never appears on the storefront
Test: services/internal/waitlist/allocate_test.go

#### Scenario: Capture fails for the first person
- **WHEN** the first fan's capture fails
- **THEN** that fan is removed with a notice and the second fan gets the ticket
Test: services/internal/waitlist/allocate_test.go

### Requirement: Leave the waitlist
A fan SHALL be able to leave a waitlist at any time before being allocated; the pre-authorisation MUST be released (simulated void).

#### Scenario: Fan leaves
- **WHEN** a fan leaves the waitlist
- **THEN** they are removed, the authorisation is voided, and everyone behind them moves up one place
Test: services/internal/waitlist/join_test.go
