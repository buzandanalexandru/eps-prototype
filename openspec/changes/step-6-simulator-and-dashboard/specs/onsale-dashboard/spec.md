# Spec Delta

## Purpose

Shows an organiser what is happening in a live on-sale, second by second, from the system's own numbers: queue, admissions, sales, payment failures and failovers, bots caught and stock left, with the live controls at hand.

## ADDED Requirements

### Requirement: Live on-sale numbers
The dashboard SHALL show for a sale: fans in the queue and pre-queue, fans admitted per minute, fans shopping, tickets sold of capacity, revenue, payment success rate, payment failures per minute, failovers, provider A and B status, bots flagged and sent back, OTP challenges, refused passes, and stock left and held per pool.

#### Scenario: During a run
- **WHEN** the organiser opens the dashboard during a simulator run
- **THEN** every listed number is shown and changes as the run progresses
- Test: apps/web/tests/e2e/onsale-dashboard.spec.ts

### Requirement: Numbers come from the system
Queue, stock and provider state SHALL be read from the live system's own state, and rates from the services' metrics, never computed or animated in the browser.

#### Scenario: Matches the store
- **WHEN** the dashboard shows 1,234 left in a pool
- **THEN** the sum of that pool's stock counters at that second is 1,234 within one second's change
- Test: services/internal/live/aggregator_test.go

### Requirement: Updated every second
The dashboard SHALL receive a new frame at least every 2 seconds while the sale is live, SHALL reconnect by itself after a dropped connection, and SHALL show a "stale" badge if no frame arrived for 5 seconds.

#### Scenario: Connection drops
- **WHEN** the stream connection drops for 10 seconds
- **THEN** the dashboard shows "stale", reconnects, and resumes with the latest numbers
- Test: apps/web/tests/e2e/onsale-dashboard.spec.ts

#### Scenario: Frame rate
- **WHEN** a sale is live for 60 seconds with the dashboard open
- **THEN** at least 30 frames are delivered
- Test: services/internal/live/stream_test.go

### Requirement: Failovers are visible
Each failover and each provider breaker change SHALL appear on the dashboard within 3 seconds, as a status change for the provider and a marker on the sales chart.

#### Scenario: Provider A goes down
- **WHEN** provider A's breaker opens during a run
- **THEN** within 3 seconds A shows as down, B as taking payments, and a failover marker appears on the chart
- Test: apps/web/tests/e2e/onsale-dashboard.spec.ts

### Requirement: Bots caught against the truth
For a simulator run, the dashboard SHALL show bots caught next to the simulator's ground truth: bots flagged of bots sent, bots admitted, tickets bought by bots and real fans flagged.

#### Scenario: Bot panel
- **WHEN** a run with 10,000 bots is in progress
- **THEN** the bots panel shows flagged of 10,000, admitted bots and real fans flagged
- Test: services/internal/live/aggregator_test.go

### Requirement: Live controls on the dashboard
The dashboard SHALL hold the admission rate dial, the pause button and the degrade switch, acting through the same rules as the sale settings and the degrade switch.

#### Scenario: Turn the dial
- **WHEN** the organiser moves the dial from 2,000 to 4,000 per minute on the dashboard
- **THEN** admitted per minute rises toward 4,000 within the next minute's frames
- Test: apps/web/tests/e2e/onsale-dashboard.spec.ts

### Requirement: Organiser only
The live stream and dashboard for a sale SHALL be available only to that sale's organiser.

#### Scenario: Other organiser
- **WHEN** organiser B opens the live stream of organiser A's sale
- **THEN** the connection is refused
- Test: services/internal/live/stream_test.go
