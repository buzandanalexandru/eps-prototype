# Spec Delta

## Purpose

Sends the organiser's own systems a signed notice of what happens on EPS (orders paid, tickets issued and more), with retries and a delivery log, so organisers can connect EPS to their CRM, accounting or access tools.

## ADDED Requirements

### Requirement: Webhook endpoints
The system SHALL let an owner or admin add HTTPS webhook endpoints with a chosen list of event types (at least `order.paid`, `ticket.issued`, `order.expired`, `snapshot.published`), and SHALL show the endpoint's signing secret once at creation.

#### Scenario: Add an endpoint
- **WHEN** an admin adds `https://crm.example.com/eps` subscribed to `order.paid`
- **THEN** the endpoint is listed as active and its secret is shown once and never again
- Test: apps/web/tests/e2e/webhooks.spec.ts

#### Scenario: Private address refused
- **WHEN** an admin enters an `http://` URL or a URL that resolves to a private, loopback or link-local address
- **THEN** the endpoint is refused with a message, and the same check runs again before every delivery
- Test: services/worker/internal/webhooks/target_test.go

### Requirement: Signed deliveries
The system SHALL deliver each subscribed event as a JSON POST carrying an event ID, a timestamp and an HMAC-SHA256 signature of the timestamp and body made with the endpoint's secret, and only events of the endpoint's own organiser.

#### Scenario: Receiver verifies the signature
- **WHEN** an order of the organiser is paid
- **THEN** the endpoint receives one POST whose signature verifies with its secret, and another organiser's endpoints receive nothing
- Test: services/worker/internal/webhooks/deliver_test.go

### Requirement: Retries and failure
The system SHALL treat any 2xx answer within 5 seconds as delivered and SHALL otherwise retry with growing delays up to 6 attempts in total, then mark the delivery failed; redeliveries SHALL keep the same event ID.

#### Scenario: Receiver down, then back
- **WHEN** the endpoint answers 500 twice and then 200
- **THEN** the delivery log shows three attempts with their status codes and the delivery ends as delivered with one event ID
- Test: services/worker/internal/webhooks/deliver_test.go

### Requirement: Delivery log and test event
The system SHALL show per endpoint a log of deliveries (event type, time, attempts, last status) with a Redeliver action and a Send test event action.

#### Scenario: Send a test event
- **WHEN** an admin presses Send test event on the sample organiser's endpoint pointed at the built-in demo receiver
- **THEN** a `test.ping` delivery appears in the log as delivered and the demo receiver shows a verified signature
- Test: manual check on the deployed preview (Webhooks screen, test event delivered to the demo receiver)
