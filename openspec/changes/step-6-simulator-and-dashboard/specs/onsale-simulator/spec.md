# Spec Delta

## Purpose

Lets an organiser rehearse a high-demand on-sale by sending a configurable crowd of simulated fans and bots, plus a provider outage, through the real waiting room and checkout, without touching real stock or real reports.

## ADDED Requirements

### Requirement: Rehearsal sales
An organiser SHALL be able to copy a published sale into a rehearsal sale with the same products, prices, settings and its own stock. The shop SHALL label it as a rehearsal with simulated fans, its orders SHALL be marked as rehearsal and kept out of sales reports and real caps, and it SHALL be resettable.

#### Scenario: Create a rehearsal
- **WHEN** an organiser presses "Create rehearsal" on a published sale
- **THEN** a rehearsal sale exists with the same snapshot version and full stock, and its shop page shows the rehearsal banner
- Test: services/internal/simulation/rehearsal_test.go

#### Scenario: Kept out of reports
- **WHEN** 5,000 rehearsal orders are paid
- **THEN** the organiser's sales figures for the real sale are unchanged
- Test: services/internal/simulation/rehearsal_test.go

### Requirement: Control panel with knobs
Studio SHALL offer a simulator panel to start a run on a rehearsal sale with: number of fans (up to 200,000, default 100,000), bot share, share arriving before opening, arrival pattern, quantity mix, payment-method mix, abandonment rate, decline rate, polling interval, an outage or failure rate on provider A at a set time, and duration. Invalid values SHALL be refused.

#### Scenario: Start the default run
- **WHEN** the organiser presses Start with the defaults
- **THEN** a run of 100,000 fans with 10% bots starts against the rehearsal sale and the panel shows its state
- Test: apps/web/tests/e2e/simulator-panel.spec.ts

#### Scenario: Real sale refused
- **WHEN** a run is requested against a sale that is not a rehearsal
- **THEN** it is refused
- Test: services/internal/simulation/run_test.go

### Requirement: Run lifecycle
A run SHALL move through `pending`, `starting`, `running`, `finishing` and `done`, or end as `failed` or `stopped`. Only one run SHALL be active per sale and per organiser at a time, and "Stop" SHALL end all its traffic within 15 seconds.

#### Scenario: Stop a run
- **WHEN** the organiser presses Stop during a run
- **THEN** within 15 seconds no simulated traffic reaches the gateway and the run is `stopped`
- Test: services/internal/simulation/run_test.go

#### Scenario: Second run
- **WHEN** a second run is requested while one is active for the same organiser
- **THEN** it is refused with the active run named
- Test: services/internal/simulation/run_test.go

### Requirement: Real traffic through the real system
Simulated fans SHALL use the same public HTTP API as browsers, through the gateway and core, obeying the queue, passes, caps, OTP and payment rules, and paying through the fake providers. The simulator SHALL NOT write counters, stock or orders directly.

#### Scenario: Fan journey
- **WHEN** a simulated fan is admitted
- **THEN** its hold, payment attempt and order appear in the system exactly as a human fan's would, marked with the run id
- Test: services/internal/simulation/fan_test.go

#### Scenario: No shortcuts
- **WHEN** a run of 1,000 fans completes
- **THEN** every sold ticket has a paid order with a payment attempt at a fake provider, and the queue admitted at least as many fans as orders were paid
- Test: services/internal/simulation/fan_test.go

### Requirement: Bots behave like bots
The bot share SHALL be spread across refresh spammers, account farms, headless clients, queue skippers and cap breakers, each trying its behaviour against the real defences. The simulator SHALL report which clients were bots, so results can be measured, without telling the system under test.

#### Scenario: Queue skipper
- **WHEN** a skipper bot calls checkout without a pass or with a copied pass
- **THEN** the system refuses it and the simulator counts the refusal
- Test: services/internal/simulation/bots_test.go

#### Scenario: Ground truth reported
- **WHEN** a run ends
- **THEN** the run summary shows bots sent, bots flagged by the system, bots admitted and tickets bots bought
- Test: services/internal/simulation/bots_test.go

### Requirement: Provider outage during the run
A run SHALL be able to switch provider A to an outage or a failure rate at a set time and back after a set length, through the providers' own controls, so failover happens for real.

#### Scenario: Outage at minute three
- **WHEN** a run is set with A's outage at 3 minutes for 90 seconds
- **THEN** A fails from minute 3, new payments complete on B, and A takes payments again after its breaker recovers
- Test: apps/web/tests/e2e/simulator-run.spec.ts

### Requirement: Run summary
When a run ends, Studio SHALL show its summary: fans and bots sent, fans admitted, tickets sold, time to sell out if it did, payment success rate, failovers, bots caught and real fans flagged.

#### Scenario: Summary after the run
- **WHEN** a run ends as `done`
- **THEN** the panel shows the summary figures for that run
- Test: apps/web/tests/e2e/simulator-run.spec.ts
