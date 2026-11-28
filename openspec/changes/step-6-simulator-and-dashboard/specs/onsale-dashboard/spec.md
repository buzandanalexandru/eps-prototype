# Spec Delta

## Purpose

Shows an organiser what is happening in a live on-sale, second by second, from the system's own numbers: queue, admissions, sales, payment failures and failovers, bots caught and stock left, with the live controls at hand.

## ADDED Requirements

### Requirement: Live on-sale numbers
The dashboard SHALL show for a sale: fans in the queue and pre-queue, fans admitted per minute, fans shopping, tickets sold of capacity, revenue, payment success rate, payment failures per minute, failovers, provider A and B status, bots flagged and sent to the back, OTP challenges, refused passes, and stock left, held and sold per pool.

#### Scenario: During a run
- **WHEN** the organiser opens the dashboard during a simulator run
- **THEN** every listed number is shown and the queue and sales numbers change as the run progresses
- Test: apps/web/tests/e2e/simulator-run.spec.ts

#### Scenario: Before any traffic
- **WHEN** the organiser opens the dashboard of a sale with no traffic yet
- **THEN** every tile shows its value (zero where nothing happened) and the frame time advances
- Test: apps/web/tests/e2e/onsale-dashboard.spec.ts

### Requirement: Numbers come from the system
Queue, stock, sales and provider state SHALL be read from the live system's own state each second, and rates SHALL be the change of those totals over the last minute, never computed or animated in the browser.

#### Scenario: Matches the store
- **WHEN** a frame is built while a pool has units held and sold
- **THEN** its left, held and sold equal the sum of that pool's stock shards and its sold total equals the paid orders' tickets
- Test: services/internal/live/aggregator_test.go

### Requirement: Updated every second
The dashboard SHALL receive a new frame at least every 2 seconds while it is open, SHALL reconnect by itself after a dropped connection, and SHALL show a "stale" badge if no frame arrived for 5 seconds.

#### Scenario: Connection drops
- **WHEN** the stream connection drops for 10 seconds
- **THEN** the dashboard shows "stale", reconnects, and resumes with the latest numbers
- Test: apps/web/tests/e2e/onsale-dashboard.spec.ts

#### Scenario: Frame rate
- **WHEN** a sale is watched for 30 seconds
- **THEN** at least 15 frames are delivered to the stream
- Test: services/internal/live/stream_test.go

### Requirement: Failovers are visible
Each failover and each provider breaker change SHALL appear on the dashboard within 3 seconds, as a status change for the provider, a timeline entry and a marker on the sales chart.

#### Scenario: Provider A goes down
- **WHEN** provider A's breaker opens for the sale's merchant
- **THEN** the next frame shows A as open, the timeline records it, and failovers counted since then appear as markers
- Test: services/internal/live/aggregator_test.go

### Requirement: Bots caught against the truth
For a simulator run, the dashboard SHALL show bots flagged next to the simulator's ground truth: bots flagged of bots sent, bots admitted, tickets bought by bots and real fans asked for a code.

#### Scenario: Bot panel
- **WHEN** a frame is built while a run with bots is active
- **THEN** it carries the system's flagged count and the run's truth: bots sent, bots admitted, bot tickets and real fans asked for a code
- Test: services/internal/live/aggregator_test.go

### Requirement: Live controls on the dashboard
The dashboard SHALL hold the admission rate dial, the pause button and the degrade switch, acting through the same rules as the sale settings and the degrade switch.

#### Scenario: Turn the dial
- **WHEN** the organiser moves the dial from 2,000 to 4,000 per minute on the dashboard
- **THEN** the sale's rate becomes 4,000 and the next frames show the new rate setting
- Test: apps/web/tests/e2e/onsale-dashboard.spec.ts

### Requirement: Organiser only
The live stream and dashboard for a sale SHALL be available only to that sale's organiser.

#### Scenario: Other organiser
- **WHEN** organiser B opens the live stream of organiser A's sale
- **THEN** the connection is refused
- Test: services/internal/live/stream_test.go

#### Scenario: No token
- **WHEN** the live stream is opened without an organiser token or with a fan's token
- **THEN** the connection is refused
- Test: services/internal/live/stream_test.go
