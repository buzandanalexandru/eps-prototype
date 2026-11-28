# Spec Delta

## Purpose

Puts fans in a fair line for a high-demand sale: a pre-queue before opening, a random shuffle at opening, first come after that, and admission to checkout at a controlled rate with a signed pass checkout requires.

## ADDED Requirements

### Requirement: Pre-queue before opening
When the queue is on, fans who arrive between the pre-queue opening and the sale opening SHALL join a pre-queue and see that the order will be drawn at opening. Arriving earlier within the pre-queue window SHALL give no advantage.

#### Scenario: Join early
- **WHEN** a fan opens the waiting room 20 minutes before opening
- **THEN** the fan is in the pre-queue and sees a countdown to the draw instead of a position
- Test: apps/web/tests/e2e/waiting-room.spec.ts

#### Scenario: Before the window
- **WHEN** a fan arrives before the pre-queue opens
- **THEN** the page shows when the pre-queue opens and does not take a place
- Test: services/internal/queue/join_test.go

### Requirement: Random shuffle at opening
At opening the system SHALL give every member who arrived before opening a random position, using a uniform random permutation with one audited seed. Fans who arrive at or after opening, including during the draw, SHALL be placed behind, in order of arrival.

#### Scenario: Uniform shuffle
- **WHEN** 10,000 pre-queue members are shuffled 200 times in a test
- **THEN** arrival order and position are uncorrelated and every position is equally likely within statistical tolerance
- Test: services/internal/queue/shuffle_test.go

#### Scenario: Latecomer
- **WHEN** a fan joins 3 minutes after opening while 80,000 are in line
- **THEN** the fan's position is 80,001
- Test: services/internal/queue/shuffle_test.go

### Requirement: The draw is resumable and decided by the line
A draw that stops part way SHALL be continued with the same seed, so no position is handed out twice. Whether a fan joins the line or waits for the draw SHALL be decided by the line's own state and clock, not by the clock of the server that took the join.

#### Scenario: Draw cut short
- **WHEN** the draw stops after its first batch and runs again
- **THEN** every position is handed out once, with the same seed, and a fan who joined meanwhile is behind it
- Test: services/internal/queue/review_test.go

#### Scenario: Join from a lagging clock after the draw
- **WHEN** a server whose clock is a little behind takes a join after the draw finished
- **THEN** the fan enters the line and is let in like any latecomer
- Test: services/internal/queue/review_test.go

### Requirement: One place per fan
A fan SHALL hold one place per sale, by account when signed in and by guest identity otherwise. Reloading the page, opening another tab or reconnecting SHALL keep the same place, and each of the fan's browsers SHALL be able to collect its own pass for that place once it is admitted.

#### Scenario: Second device buys with the place
- **WHEN** a fan's phone and laptop share one place and the place is admitted
- **THEN** each gets a pass for its own session and either can buy
- Test: services/internal/queue/review_test.go

#### Scenario: Reload keeps place
- **WHEN** a fan at position 12,340 reloads the page
- **THEN** the fan is still at the same place in line
- Test: apps/web/tests/e2e/waiting-room.spec.ts

#### Scenario: Second join with the same account
- **WHEN** the same account joins again from another device
- **THEN** it gets the same place, not a second one
- Test: services/internal/queue/join_test.go

### Requirement: Position and estimate
The waiting room SHALL show the number of fans ahead and an estimated wait based on the admissions actually made in the last minute, updated at least every 15 seconds, in Romanian and English. While the sale is paused no estimate SHALL be shown.

#### Scenario: Estimate from measured admissions
- **WHEN** the shopping limit holds admissions far below the dial
- **THEN** the estimate follows the admissions actually made, and is hidden while paused
- Test: services/internal/queue/review_test.go

#### Scenario: Position moves
- **WHEN** 2,000 fans are admitted ahead of a waiting fan
- **THEN** at the next update the fan's number ahead has dropped by 2,000 and the estimate is recalculated
- Test: services/internal/queue/status_test.go

### Requirement: Admission at the organiser's rate
The system SHALL admit fans from the front of the line at the sale's admission rate and SHALL NOT exceed the limit of fans shopping at once. A paused sale SHALL admit nobody until it is resumed.

#### Scenario: Rate respected
- **WHEN** the rate is 1,200 per minute and the line is long
- **THEN** between 1,140 and 1,260 fans are admitted in each minute
- Test: services/internal/queue/admitter_test.go

#### Scenario: Shopping limit
- **WHEN** 1,500 fans are shopping and the limit is 1,500
- **THEN** no one else is admitted until a pass ends
- Test: services/internal/queue/admitter_test.go

#### Scenario: Pause
- **WHEN** the organiser pauses admissions
- **THEN** no fan is admitted and waiting fans see "Paused for a moment, your place is safe"
- Test: services/internal/queue/admitter_test.go

### Requirement: Signed pass on admission
Each admitted fan SHALL receive a signed pass valid for 10 minutes, bound to the fan's session, sale and event. It SHALL end when the fan's order is paid or cancelled, freeing a shopping place. An admission whose pass is not collected within a minute SHALL free its shopping place for the next fan.

#### Scenario: Pass not collected
- **WHEN** an admitted fan's page does not collect its pass within a minute
- **THEN** the next fan is let in and the first fan's page offers to join again
- Test: services/internal/queue/review_test.go

#### Scenario: Pass issued
- **WHEN** a fan reaches the front of the line
- **THEN** the page moves to the shop with a pass that expires 10 minutes later
- Test: services/internal/queue/pass_test.go

#### Scenario: Pass ends after purchase
- **WHEN** the fan's order is paid
- **THEN** the pass can no longer be used and the shopping count drops by one
- Test: services/internal/queue/pass_test.go

### Requirement: Checkout requires a valid pass
On a sale with the queue on, the checkout SHALL refuse to hold stock, take payment or check an OTP without a pass that has a valid signature, has not expired, matches the sale and event and belongs to the requesting session and account. A payment or OTP for an order already held under a pass SHALL accept that pass after its 10 minutes (the hold bounds it), and a payment through the order's own signed link SHALL need no pass.

#### Scenario: No pass
- **WHEN** a request tries to hold tickets without a pass
- **THEN** it is refused and the fan is sent to the waiting room
- Test: services/internal/queue/gate_test.go

#### Scenario: Pass from another session
- **WHEN** a valid pass is replayed from a different session
- **THEN** it is refused
- Test: services/internal/queue/gate_test.go

#### Scenario: Forged pass
- **WHEN** a pass's payload is edited or it is signed with another key
- **THEN** it is refused
- Test: services/internal/queue/gate_test.go

#### Scenario: Payment needs the order's pass
- **WHEN** a held order is paid without its pass from the same session
- **THEN** the payment is refused, while a payment through the order's signed link goes ahead
- Test: services/internal/queue/gate_test.go

### Requirement: Queue off still uses passes
When the queue is off, a fan opening the shop SHALL receive a pass at once, so checkout rules stay the same.

#### Scenario: No queue
- **WHEN** a fan opens a sale whose queue is off
- **THEN** a pass is issued immediately and checkout opens without waiting
- Test: services/internal/queue/join_test.go

### Requirement: Waiting room holds up at opening
The waiting room's join and status endpoints SHALL keep answering under the opening spike without errors to real fans. The rates below are targets measured on the cluster in steps 6 and 12 (on the 4-core sandbox one gateway answered with no errors at every rate and saturated near 4,000 requests a second).

#### Scenario: Opening spike
- **WHEN** a load test sends 5,000 joins per second and 10,000 status checks per second to one gateway instance's share
- **THEN** p95 latency stays under 200 ms and the error rate under 0.1%
- Test: load/queue.js
