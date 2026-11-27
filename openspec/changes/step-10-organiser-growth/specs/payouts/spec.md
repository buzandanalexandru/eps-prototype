# Spec Delta

## Purpose

Shows each organiser how it gets paid: a choice between connecting its own merchant account and letting EPS act as merchant, and a simulated list of payouts, without any real money or bank details.

## ADDED Requirements

### Requirement: Payment account choice
The system SHALL let the organiser owner choose between connecting its own merchant account and letting the platform act as merchant, through a simulated connect flow that collects no bank or card details, and SHALL show the account's status.

#### Scenario: Connect a merchant account
- **WHEN** the owner chooses "Connect your own merchant account" and completes the simulated connect flow
- **THEN** the screen shows the account as connected (simulated) and a notice that no real payments are made
- Test: apps/web/tests/e2e/payouts.spec.ts

#### Scenario: Not the owner
- **WHEN** a box office member opens the payment account screen
- **THEN** the screen is read-only and the choice cannot be changed
- Test: apps/web/tests/e2e/payouts.spec.ts

### Requirement: Simulated payouts list
The system SHALL list weekly payouts computed from the organiser's paid net revenue, each with period, amount and a status of scheduled or paid (simulated).

#### Scenario: Payouts after sales
- **WHEN** the organiser has paid orders in two weeks
- **THEN** the payouts list shows two rows whose amounts equal the net revenue of each week on the sales dashboard
- Test: manual check on the deployed preview (payouts list amounts match the Sales screen net per week)
