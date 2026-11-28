# Spec Delta

## Purpose

Makes the on-sale services measurable: metrics for queue, checkout, payments, stock and the simulator, collected in the cluster, plus health checks and structured logs, feeding the live dashboard and the scaling story.

## ADDED Requirements

### Requirement: Service metrics
Every Go service (gateway, core, worker, simulated providers, simulator) SHALL expose metrics in the Prometheus text format for its part of the on-sale: queue, passes, bots and streams in the gateway; holds, orders, tickets, payment attempts, failovers, OTP, caps, degrade refusals and stock in the core; outbox, holds, stock corrections and frames in the worker; charges and knobs in the providers; fans, bots and latency in the simulator.

#### Scenario: Metrics present
- **WHEN** the core's metrics are scraped after one paid order
- **THEN** the listed core series are present and the paid order and its tickets are counted; a degraded refusal is counted in `eps_degrade_refused_total`; after a run the simulator's series (`sim_requests_total`, `sim_request_seconds`, `sim_fans`, `sim_bots`), `eps_sim_reports_total`, `eps_sim_runs_total`, the providers' `fakepsp_charges_total` and `fakepsp_knob_changes_total`, and rehearsal-labelled paid orders are present
- Test: services/internal/metrics/series_test.go, services/internal/onsale/degrade_test.go, services/internal/simulation/fan_test.go

### Requirement: No personal data in metrics or logs
Metric labels SHALL hold only service, route, organiser, event, sale, pool, provider, method and result values, never emails, names, IP addresses or tokens; logs SHALL NOT contain card-like data, tokens or full emails.

#### Scenario: Label check
- **WHEN** a test runs a checkout as a guest with an email and scrapes the metrics and the logs
- **THEN** no metric label or log line contains the email, the pass or the session secret
- Test: services/internal/metrics/series_test.go

### Requirement: Metrics are collected in the cluster
The cluster SHALL run a metrics collector in each environment that scrapes the on-sale services (gateway, core, worker) at least every 2 seconds and the others at least every 15 seconds, and keeps 6 hours of history, with permission only to list the pods of its own namespace.

#### Scenario: Chart
- **WHEN** the chart is rendered
- **THEN** it holds the collector with a 2-second job for the on-sale pods, 6-hour retention and a namespace-scoped role that can only read pods
- Test: services/internal/metrics/chart_test.go

#### Scenario: Scrape
- **WHEN** the chart is installed on the preview namespace
- **THEN** the collector lists every on-sale service pod as a healthy target
- Test: manual check on the deployed preview (port-forward the collector and see every pod up on its targets page)

### Requirement: Health and structured logs
Every Go service SHALL answer a liveness and a readiness check, readiness failing when its database or Redis cannot be reached, and SHALL log JSON lines with a request id.

#### Scenario: Redis unreachable
- **WHEN** a core instance cannot reach Redis
- **THEN** its readiness check fails so it receives no traffic, and its log shows the error with a request id
- Test: services/internal/health/health_test.go
