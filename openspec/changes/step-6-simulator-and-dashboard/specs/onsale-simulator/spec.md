# Spec Delta

## Purpose

Lets an organiser rehearse a high-demand on-sale by sending a bounded, configurable crowd of simulated fans and bots, plus a provider outage, through the real waiting room and checkout of a rehearsal copy of a sale, without touching real stock, real caps, the real payment merchant or real reports, and without claiming more than was measured.

## ADDED Requirements

### Requirement: Rehearsal sales
An organiser SHALL be able to copy a published sale moment into a rehearsal with the same products, prices and settings, under an event identity and stock of its own. Its orders SHALL be marked rehearsal and kept out of the real sale's figures, stock and caps; its payments SHALL use their own merchant account and breaker at the simulated providers; its waiting room and checkout SHALL say "Rehearsal: simulated fans"; a reset SHALL give a fresh rehearsal.

#### Scenario: Create a rehearsal
- **WHEN** an organiser presses "Create rehearsal" on a sale moment of a published event
- **THEN** a rehearsal exists with the source's snapshot version, prices and settings, full stock of its own, and its waiting room reports it as a rehearsal
- Test: services/internal/simulation/rehearsal_test.go

#### Scenario: Kept out of the real sale
- **WHEN** rehearsal orders are paid
- **THEN** the real event's paid orders, stock counters and cap ledger are unchanged, and each rehearsal order is marked rehearsal with its run
- Test: services/internal/simulation/rehearsal_test.go

#### Scenario: Rehearsed outage stays in the rehearsal
- **WHEN** provider A is set to an outage for a rehearsal
- **THEN** a payment on the real sale still goes to provider A and its breaker for the organiser stays closed
- Test: services/internal/simulation/rehearsal_test.go

#### Scenario: Reset
- **WHEN** the organiser resets a rehearsal with no active run
- **THEN** a fresh rehearsal with full stock and an empty queue replaces it, and the old one's Redis data is deleted automatically
- Test: services/internal/simulation/rehearsal_test.go

### Requirement: No real money on a rehearsal
A rehearsal SHALL offer and accept only one-off payments through the simulated providers (hosted card, Apple Pay, Google Pay). The wallet, saved cards, saving a card and instalments SHALL be refused with 422 `EPS-PAY-METHOD`. Its orders SHALL make no ledger postings and no tickets, and its payment attempts SHALL carry the rehearsal's event.

#### Scenario: No real money on a rehearsal
- **WHEN** a simulated fan on a rehearsal tries to pay with the wallet, a saved card, saving a card or instalments
- **THEN** it is refused with `EPS-PAY-METHOD`, none of them is offered, and a paid rehearsal order makes no ledger posting and no ticket
- Test: services/internal/simulation/rehearsal_test.go

### Requirement: Rehearsals close and are cleaned up
A rehearsal SHALL close (paused, its opening moved past the horizon) once its run ends. When a rehearsal is reset or deleted, the clean-up SHALL wait out its holds, delete its Redis data and anonymise its orders.

#### Scenario: Closed after the run
- **WHEN** a run ends as `done`
- **THEN** its rehearsal is paused with its opening at least a day away
- Test: services/internal/simulation/fan_test.go

#### Scenario: Cleaned after a reset
- **WHEN** a reset rehearsal's hold time has passed and the clean-up runs
- **THEN** its Redis keys are gone and its orders hold no contact details, and the real event's keys are untouched
- Test: services/internal/simulation/rehearsal_test.go

### Requirement: Simulated fans only reach rehearsals
Simulated fans SHALL authenticate with a rehearsal-only token issuer that the gateway and core accept only for the rehearsal named in the token and only for joining its waiting room and its checkout operations. The simulator SHALL have no database or Redis access.

#### Scenario: Token on a real sale
- **WHEN** a simulated fan's token is used to join or hold on a sale that is not its rehearsal
- **THEN** it is refused and its address is not trusted
- Test: services/internal/simulation/run_test.go

### Requirement: Control panel with bounded knobs
Studio SHALL offer a simulator panel to start a run on a rehearsal with: number of fans (default 1,000, at most the environment's cap), bot share, share arriving before opening, arrival pattern, quantity mix, payment-method mix, abandonment rate, decline rate, a polling floor, an outage or failure rate on provider A at a set time and for a set length, and duration (at most 10 minutes). Invalid values SHALL be refused, naming the field and its range.

#### Scenario: Start the default run
- **WHEN** the organiser presses Start with the defaults on a rehearsal
- **THEN** a run of 1,000 fans with 10% bots starts and the panel shows its state
- Test: apps/web/tests/e2e/simulator-run.spec.ts

#### Scenario: Out of range
- **WHEN** a run is requested with more fans than the environment's cap or a duration over 10 minutes
- **THEN** it is refused naming the field and its range
- Test: services/internal/simulation/run_test.go

#### Scenario: Real sale refused
- **WHEN** a run is requested against a sale that is not a rehearsal
- **THEN** it is refused
- Test: services/internal/simulation/run_test.go

### Requirement: Run lifecycle
A run SHALL move through `pending`, `starting`, `running`, `finishing` and `done`, or end as `failed` or `stopped`. Only one run SHALL be active per organiser and on the whole cluster at a time. The simulator SHALL stop all its fans by itself when no report reaches the core for 25 seconds. "Stop" SHALL end all its traffic within 15 seconds; a run whose simulator stops reporting for 30 seconds SHALL end as `failed`; every end SHALL restore the rehearsal's provider settings.

#### Scenario: Stop a run
- **WHEN** the organiser presses Stop during a run
- **THEN** within 15 seconds the simulator sends no more traffic and the run is `stopped`
- Test: services/internal/simulation/run_test.go

#### Scenario: Second run
- **WHEN** a second run is requested while one is active for the same organiser
- **THEN** it is refused with the active run named
- Test: services/internal/simulation/run_test.go

#### Scenario: Core lost
- **WHEN** the simulator's reports stop reaching the core
- **THEN** within 25 seconds it stops every fan and sends no more traffic
- Test: services/internal/simulation/run_test.go

#### Scenario: Simulator lost
- **WHEN** an active run gets no report for 30 seconds
- **THEN** it ends as `failed` and provider A's settings for the rehearsal are restored
- Test: services/internal/simulation/run_test.go

### Requirement: Runs are bounded for a shared cluster
A run SHALL be capped at the environment's fan limit (5,000 in production, 2,000 elsewhere) and 1,500 requests a second. A run SHALL be refused when its organiser started 3 in the last hour, or while a real sale on the platform has fans in its waiting room, and these checks SHALL come before the rehearsal is reset. In production the shared sample organiser SHALL run at the default size only. Every run start and stop SHALL be logged with who made it.

#### Scenario: Run refused
- **WHEN** a run is requested by an organiser who started 3 this hour, or while a real sale has fans in its waiting room, or in production on the shared sample above the default size
- **THEN** it is refused (rate limited, busy with the reason `real_sale_live`, or out of range) and nothing is reset
- Test: services/internal/simulation/run_test.go

### Requirement: Real traffic through the real system
Simulated fans SHALL use the same HTTP API as browsers, through the gateway and core, obeying the queue, passes, caps, OTP and payment rules, and paying through the simulated providers (a card fan presses the provider's hosted page). The simulator SHALL NOT write counters, stock or orders directly.

#### Scenario: Fan journey
- **WHEN** a simulated fan is admitted
- **THEN** its hold, payment attempt and order appear in the system exactly as a human fan's would, marked with the run id
- Test: services/internal/simulation/fan_test.go

#### Scenario: No shortcuts
- **WHEN** a run of 200 fans completes
- **THEN** every paid order has a succeeded payment attempt at a simulated provider, and the queue admitted at least as many fans as orders were paid
- Test: services/internal/simulation/fan_test.go

### Requirement: Bots behave like bots
The bot share SHALL be spread across refresh spammers, account farms, headless clients, queue skippers and cap breakers, each trying its behaviour against the real defences. The simulator SHALL report which clients were bots, so results can be measured, without telling the system under test.

#### Scenario: Queue skipper
- **WHEN** a skipper bot tries to hold without a pass, with a forged pass or with another session's pass
- **THEN** the system refuses it and the simulator counts the refusal
- Test: services/internal/simulation/bots_test.go

#### Scenario: Ground truth reported
- **WHEN** a run ends
- **THEN** the run summary shows bots planned and arrived, places the system flagged split into bots and real fans, bots admitted and tickets bots bought
- Test: services/internal/simulation/bots_test.go

### Requirement: Provider outage during the run
A run SHALL be able to switch provider A to an outage or a failure rate at a set time after opening and back after a set length, through the providers' own controls for the rehearsal's merchant, so failover happens for real.

#### Scenario: Outage on schedule
- **WHEN** a run is set with A's outage at 20 seconds for 30 seconds
- **THEN** A's settings for the rehearsal change at 20 seconds, new payments complete on B, and A's settings return after 50 seconds
- Test: services/internal/simulation/run_test.go (the schedule), services/internal/simulation/fan_test.go (TestOutageRunPaysAtB: payments during the outage complete on B)

#### Scenario: Failover in a real run
- **WHEN** a small run on the deployed preview has A down in its middle
- **THEN** the dashboard shows a failover and the run ends with a summary
- Test: apps/web/tests/e2e/simulator-run.spec.ts

### Requirement: Honest summary
When a run ends, Studio SHALL show its summary: fans and bots planned and arrived (on their first request), fans admitted, tickets sold, time to sell out from the system's own record, payment success per order, failovers and payments at B during the outage, bots flagged against the truth, real fans asked for a code, requests per second averaged over the run with its busiest second, and p95 latency over the run.

#### Scenario: Summary after the run
- **WHEN** a run ends as `done`
- **THEN** the panel shows the summary figures for that run, fans arrived of planned, payments at B during the outage, the average and peak requests per second, and the dashboard then shows the bots against the truth
- Test: apps/web/tests/e2e/simulator-run.spec.ts, services/internal/simulation/fan_test.go

### Requirement: Load panel keeps measurements apart from arithmetic
The panel SHALL say this run's numbers are measured in this environment and keep the sandbox measurements of step 5 under their own heading. It SHALL show any 100,000-fan figure only as arithmetic, sized on the sustainable measured rate (2,000 status polls a second at p95 13 ms per gateway), with its assumptions and the opening burst named, and SHALL state the enforced limits.

#### Scenario: Arithmetic labelled
- **WHEN** the organiser opens the simulator panel
- **THEN** it says the fans are simulated and this run's numbers are measured here, shows the sandbox's k6 numbers under their own heading, shows the 100,000-fan figure as arithmetic on 2,000 polls a second with its assumptions and the opening burst, and lists the enforced limits
- Test: apps/web/tests/e2e/simulator-panel.spec.ts
