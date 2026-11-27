# Spec Delta

## Purpose

Gives a signed-in fan one place to see their orders, their tickets and their instalment schedules, including orders bought as a guest and claimed later.

## ADDED Requirements

### Requirement: Orders list
The fan account SHALL list the fan's orders, newest first, each with its organiser, event, order number, all-in total and its current state as recorded by the system (held, paying, paid, tickets issued, declined, expired).

#### Scenario: Orders with different states
- **WHEN** a fan with one paid order and one expired order opens the account
- **THEN** both appear with their true states and totals, and the paid one links to its tickets
Test: manual check on the deployed preview (account page lists a paid and an expired order)

#### Scenario: Claimed guest order
- **WHEN** a fan claims an order bought as a guest
- **THEN** that order and its tickets appear in the account and the trip wallet
Test: apps/web/tests/e2e/fan-account.spec.ts

### Requirement: Tickets list
The fan account SHALL list the tickets the fan currently holds with their product, event and state (valid, needs a name, checked in, suspended, moved), each linking to the live ticket.

#### Scenario: Open a ticket from the account
- **WHEN** the fan presses a ticket in the list
- **THEN** its live ticket page with the rotating code opens
Test: manual check on the deployed preview (open a ticket from the account)

### Requirement: Instalment schedules
The fan account SHALL show each instalment plan with its payments, due dates and statuses (scheduled, paid, overdue, grace period), and a "Pay now" action for the next or overdue payment that runs through the simulated payment flow.

#### Scenario: Pay the next instalment
- **WHEN** the fan presses Pay now on a scheduled instalment and the simulated payment succeeds
- **THEN** that payment shows as paid and the paid count increases by one
Test: apps/web/tests/e2e/fan-account.spec.ts

#### Scenario: Overdue plan
- **WHEN** a plan has an overdue payment
- **THEN** the plan is marked overdue with the amount due, and the account links to the suspended ticket
Test: manual check on the deployed preview (seeded overdue plan shows as overdue)
