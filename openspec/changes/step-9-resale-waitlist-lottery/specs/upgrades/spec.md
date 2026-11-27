# Spec Delta

## Purpose

Lets a fan move a ticket to a better product the organiser offers as an upgrade, such as GA to VIP, paying the difference plus a published fee, with the ticket reissued as the new product.

## ADDED Requirements

### Requirement: Organiser defines upgrade paths
An organiser SHALL be able to define upgrade paths between products of the same event, each with an upgrade fee, published with the sale. A ticket MUST show an Upgrade action only for paths whose target product has stock.

#### Scenario: VIP has stock
- **WHEN** a GA ticket holder opens their ticket and a GA-to-VIP path exists with VIP stock left
- **THEN** an Upgrade to VIP action shows the all-in price to pay
Test: manual check on the deployed preview (GA sample ticket shows Upgrade to VIP with its price)

### Requirement: Pay the difference and reissue the ticket
The upgrade price SHALL be the target's current all-in price minus the ticket's face value (never below zero) plus the upgrade fee, paid through the simulated payment flow. When payment is confirmed, the ticket MUST be reissued as the target product with a new code and the same name, and one unit of the original product MUST return to its stock.

#### Scenario: GA to VIP
- **WHEN** a holder of a 300 RON GA ticket upgrades to VIP at 550 RON with a 20 RON fee and the payment is confirmed
- **THEN** they pay 270 RON, the ticket becomes VIP at a new generation with its name kept, its face value becomes 550 RON, VIP stock drops by one and GA stock rises by one
Test: services/internal/upgrade/upgrade_test.go

#### Scenario: VIP sells out during the upgrade
- **WHEN** the last VIP unit is taken by someone else before the holder's upgrade hold is placed
- **THEN** the upgrade is refused before any charge and the ticket stays GA
Test: services/internal/upgrade/upgrade_test.go

#### Scenario: Ticket cannot be upgraded
- **WHEN** a ticket is on an unfinished instalment plan, listed for resale, suspended or already admitted
- **THEN** no Upgrade action is shown and the API refuses the upgrade
Test: services/internal/upgrade/upgrade_test.go
