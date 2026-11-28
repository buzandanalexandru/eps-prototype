# event-stream Specification

## Purpose
Delivers every business event that other parts of the platform react to (order paid, payment failover, wallet moves) reliably and in a common format, so the worker, the live dashboard and webhooks never miss a change.

## Requirements

### Requirement: Events written with the change
Every state change that others react to SHALL write its event in the same database transaction as the change itself, so an event exists if and only if the change was committed.

#### Scenario: Rolled-back change has no event
- **WHEN** an order's payment transaction fails and rolls back
- **THEN** no `order.paid` event exists
- Test: services/internal/outbox/outbox_test.go

#### Scenario: Committed change has an event
- **WHEN** an order becomes paid
- **THEN** exactly one `order.paid` event is recorded with the order id
- Test: services/internal/outbox/outbox_test.go

### Requirement: Relay to streams, at least once
Recorded events SHALL be published to the event streams in the order they were recorded per topic, within 1 second under normal load. If the relay stops in the middle, the events SHALL be published when it restarts, possibly twice, never lost.

#### Scenario: Relay crash
- **WHEN** the relay stops after publishing an event but before marking it sent
- **THEN** after restart the event is published again and no event is missing
- Test: services/internal/outbox/relay_test.go

#### Scenario: Latency
- **WHEN** 1,000 orders are paid in one second
- **THEN** all 1,000 `order.paid` events reach the stream within 2 seconds
- Test: services/internal/outbox/relay_test.go

### Requirement: Common event envelope
Every event SHALL carry a unique id, a type, a version, the organiser id, the event and sale ids where they apply, the time it happened and a payload. Consumers SHALL be able to ignore an event id they have already processed.

#### Scenario: Consumer deduplicates
- **WHEN** a consumer receives the same event id twice
- **THEN** it applies the event once
- Test: services/internal/outbox/consumer_test.go

### Requirement: Events stay within their organiser
An event SHALL be readable through EPS screens and APIs only by its own organiser and the platform's internal services.

#### Scenario: Other organiser
- **WHEN** organiser B asks for organiser A's events
- **THEN** none are returned
- Test: services/internal/outbox/consumer_test.go
