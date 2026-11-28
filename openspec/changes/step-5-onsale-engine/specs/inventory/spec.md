# Spec Delta

## Purpose

Keeps each capacity pool's stock correct under heavy concurrent demand: stock split across several counters, holds that expire on their own, commits on payment, and a reconciler that keeps the fast store in line with the database of record.

## ADDED Requirements

### Requirement: Stock split across shards
Each capacity pool's stock SHALL be split across several independent counters at sale start, and the sum of their allocations SHALL equal the pool's capacity in the sale's snapshot.

#### Scenario: Pool split
- **WHEN** a sale starts with an Arena pool of 8,000 places
- **THEN** its stock is spread over 32 counters whose allocations add up to 8,000
- Test: services/internal/inventory/shards_test.go

### Requirement: Atomic holds that never oversell
A hold SHALL take stock from one or more shards so that no shard goes below zero, and a hold over several pools SHALL be all or nothing. If a hold fails part way, the stock already taken SHALL be returned.

#### Scenario: Heavy contention
- **WHEN** 20,000 concurrent hold requests of 1 to 4 tickets hit a pool of 5,000
- **THEN** the tickets held and sold never exceed 5,000 and every shard stays at zero or above
- Test: services/internal/inventory/reserve_race_test.go

#### Scenario: Order larger than one shard
- **WHEN** a hold of 4 arrives and the first shard has 1 left
- **THEN** the hold takes from more than one shard and succeeds if 4 are free in total
- Test: services/internal/inventory/reserve_test.go

#### Scenario: Second pool short
- **WHEN** an order draws from two pools and the second has too little stock
- **THEN** the stock taken from the first pool is returned and nothing is held
- Test: services/internal/inventory/reserve_test.go

### Requirement: Holds expire on their own
Every hold SHALL carry a deadline on each shard it touches, and stock from a lapsed hold SHALL return to its shard within 2 seconds of the deadline, even if the service that made it has stopped.

#### Scenario: Crash after a partial hold
- **WHEN** the core stops after taking stock from one pool of a two-pool order
- **THEN** the taken stock returns when its deadline passes
- Test: services/internal/inventory/expiry_test.go

### Requirement: Commit and release are safe to repeat
Committing a hold on payment SHALL move its stock to sold exactly once, and releasing SHALL return it exactly once, however many times either is called. Committing a hold that has already expired SHALL fail and leave stock unchanged.

#### Scenario: Repeated commit
- **WHEN** a paid order's commit is called three times
- **THEN** the sold count rises once
- Test: services/internal/inventory/commit_test.go

#### Scenario: Commit after expiry
- **WHEN** commit is called for a hold that has already expired
- **THEN** it fails and the shard's stock is unchanged
- Test: services/internal/inventory/commit_test.go

### Requirement: Honest sold-out
A pool SHALL be shown sold out only when no stock is free and no holds remain; while holds remain, the shop SHALL say that all tickets are held and some may come back.

#### Scenario: Shop answer for held stock
- **WHEN** every ticket of a product's pool is held by live holds
- **THEN** the availability answer marks it all held, and once the holds are paid it is plainly sold out
- Test: services/internal/inventory/soldout_test.go

#### Scenario: Held, not sold
- **WHEN** a pool has 0 free and 40 held
- **THEN** the shop shows "All tickets are held right now; some may come back"
- Test: services/internal/inventory/soldout_test.go

### Requirement: Reconciler repairs drift
The system SHALL compare the shard counters with the paid sales in the database every 15 seconds and correct any difference seen in two rounds in a row, recording each correction. If the database shows more sold than the counters can account for, the pool SHALL stop taking new holds and an alert SHALL be raised.

#### Scenario: Counter lost an update
- **WHEN** the counters show 3 fewer free tickets than capacity minus paid sales minus live holds, in two rounds in a row
- **THEN** the reconciler adds 3 back and records the adjustment
- Test: services/internal/inventory/reconciler_test.go

#### Scenario: Commit without a paid order
- **WHEN** stock was committed for an order that is still unpaid 2 minutes later
- **THEN** the reconciler returns it to free stock
- Test: services/internal/inventory/reconciler_test.go

#### Scenario: Oversell risk
- **WHEN** the database has 10 more paid tickets than the counters allow after a store restart
- **THEN** the pool stops taking new holds and an oversell-risk alert is raised
- Test: services/internal/inventory/reconciler_test.go

#### Scenario: Throughput
- **WHEN** k6 drives holds and commits against one core instance
- **THEN** it sustains at least 1,000 holds per second with p95 under 50 ms and no oversell
- Test: load/inventory.js
