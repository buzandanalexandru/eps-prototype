# Spec Delta

## Purpose

Makes the on-sale services measurable: metrics for queue, checkout, payments, stock and the simulator, collected in the cluster, plus health checks and structured logs, feeding the live dashboard and the scaling story.

## ADDED Requirements

### Requirement: Service metrics
Every Go service (gateway, core, worker, fake providers, simulator) SHALL expose metrics in the Prometheus format covering its part of the on-sale: queue, passes and bots in the gateway; holds, orders, payments, failovers, breaker state, OTP, caps and stock in the core; outbox lag, expired holds and stock corrections in the worker; charges and knob values in the fake providers; fan and bot states in the simulator.

#### Scenario: Metrics present
- **WHEN** each service's metrics endpoint is scraped after one paid order
- **THEN** the listed series for that service are present and the paid order is counted
- Test: services/internal/metrics/metrics_test.go

### Requirement: No personal data in metrics or logs
Metric labels SHALL hold only organiser, event, sale, pool, provider, method and result values, never emails, names, IP addresses or tokens; logs SHALL NOT contain card-like data, tokens or full emails.

#### Scenario: Label check
- **WHEN** a test runs a checkout as a guest with an email and scrapes every service
- **THEN** no metric label or log line contains the email, the pass or the session secret
- Test: services/internal/metrics/metrics_test.go

### Requirement: Metrics are collected in the cluster
The cluster SHALL run a metrics collector that scrapes the on-sale services at least every 2 seconds and keeps at least 6 hours of history.

#### Scenario: Scrape
- **WHEN** the chart is installed on the preview namespace
- **THEN** the collector lists every on-sale service pod as a healthy target
- Test: manual check on the deployed preview (open the collector's targets page through a port-forward and see every pod up)

### Requirement: Health and structured logs
Every Go service SHALL answer a liveness and a readiness check, readiness failing when its database or Redis cannot be reached, and SHALL log JSON lines with a request id.

#### Scenario: Redis unreachable
- **WHEN** a core instance cannot reach Redis
- **THEN** its readiness check fails so it receives no traffic, and its log shows the error with a request id
- Test: services/internal/metrics/health_test.go
