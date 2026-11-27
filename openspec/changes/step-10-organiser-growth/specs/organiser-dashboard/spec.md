# Spec Delta

## Purpose

Gives each organiser a live, reconciled view of its sales (orders, revenue, fees and net per event, product and day), so it can run an on-sale and its business from EPS Studio.

## ADDED Requirements

### Requirement: Sales summary per event
The system SHALL show, for each event of the signed-in organiser, the number of paid orders, tickets sold, gross revenue (all-in amounts paid), platform fees and net revenue, per product and per day, in the event's currency.

#### Scenario: Totals by product and day
- **WHEN** an organiser opens the Sales screen of an event with paid orders on two days for two products
- **THEN** the screen shows orders, tickets, gross, fees and net for each product and for each day, and the totals equal the sum of the rows
- Test: apps/web/tests/e2e/sales-dashboard.spec.ts

#### Scenario: Only paid orders count
- **WHEN** an event has orders that are held, expired, declined and paid
- **THEN** only paid orders count toward orders, tickets and revenue, and the other states are shown as separate counts
- Test: services/core/internal/sales/summary_test.go

### Requirement: Figures reconcile with orders
The system SHALL compute fees from the fee policy recorded on each order at sale time, keep amounts in minor units without mixing currencies, and produce totals that equal the sum of the underlying paid orders exactly.

#### Scenario: Fee policy changed after sales
- **WHEN** the organiser changes its fee policy from absorb to pass-on after 100 orders were paid
- **THEN** the fees of those 100 orders keep the values recorded at sale time and only later orders use the new policy
- Test: services/core/internal/sales/summary_test.go

#### Scenario: Reconciliation after redelivered events
- **WHEN** the same `order.paid` event is delivered to the roll-up twice
- **THEN** the dashboard counts the order once and its totals equal a direct sum over the paid orders
- Test: services/worker/internal/rollup/rollup_test.go

### Requirement: Live updates during an on-sale
The system SHALL update the dashboard of an event while it is open, within 3 seconds of an order being paid, without a page reload.

#### Scenario: Simulator sales appear live
- **WHEN** the simulator runs an on-sale and the organiser has the Sales screen open
- **THEN** the orders and revenue figures rise within 3 seconds of each paid order, at most once per second
- Test: apps/web/tests/e2e/sales-dashboard.spec.ts

### Requirement: Day buckets in the organiser's time zone
The system SHALL group sales by calendar day in the Europe/Bucharest time zone.

#### Scenario: Order just after midnight
- **WHEN** an order is paid at 23:30 UTC on 1 July (02:30 on 2 July in Bucharest)
- **THEN** it is counted on 2 July
- Test: services/core/internal/sales/summary_test.go

### Requirement: Simulated and sample sales labelled
The system SHALL mark orders created by the on-sale simulator as simulated and SHALL show a Simulated label and filter on the dashboard, and the sample organiser's figures SHALL carry the sample data label.

#### Scenario: Filter simulated orders
- **WHEN** the organiser switches the Simulated filter off on an event that has both simulated and real test orders
- **THEN** the figures include only the non-simulated orders
- Test: apps/web/tests/e2e/sales-dashboard.spec.ts

### Requirement: Dashboard kept per organiser
The system SHALL show only the signed-in organiser's events and orders, and a brand-scoped member SHALL see only that brand's events.

#### Scenario: Another organiser's event
- **WHEN** an organiser requests the sales summary of an event that belongs to another organiser
- **THEN** the system answers not found and returns no figures
- Test: services/core/internal/sales/summary_test.go
