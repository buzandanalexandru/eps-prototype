# Spec Delta

## Purpose

Turns what an organiser publishes into immutable, versioned sale snapshots of products, pools, prices and rules, so the on-sale engine sells only published terms, drafts can change during a live sale, and every sale can be traced to the terms it was sold under.

## ADDED Requirements

### Requirement: Publishing compiles a sale snapshot per event
Publishing a brand's storefront, including restoring an older version, SHALL compile a sale snapshot for each event the storefront sells from the current catalogue, and MUST make the new snapshots live in the same action as the storefront version.

#### Scenario: First publish of a festival
- **WHEN** the organiser publishes a storefront whose event page sells Galaxia 2027
- **THEN** snapshot version 1 of Galaxia 2027 is live, holding its products, pools, price ladders, fee policy, buying rules and instalment settings
Test: apps/web/tests/unit/snapshot-compile.test.ts

#### Scenario: Restore uses current prices
- **WHEN** the organiser restores an older storefront version after changing a price in the catalogue
- **THEN** the restored look is live and the new snapshot carries the current price, not the price at the time of the older version
Test: apps/web/tests/unit/snapshot-compile.test.ts

#### Scenario: Design-only publish
- **WHEN** the organiser publishes a change of colours only
- **THEN** a new storefront version is live and no new snapshot version is created for unchanged events
Test: apps/web/tests/unit/snapshot-compile.test.ts

### Requirement: Publishing is refused when an event is not ready
Publishing SHALL be refused, with nothing made live, when any event the storefront sells fails the catalogue readiness check, and the refusal MUST list each problem.

#### Scenario: Product without a pool
- **WHEN** one sold event has a product that draws on no pool
- **THEN** publish is refused, the problem is listed with a link, and neither a storefront version nor a snapshot is created
Test: apps/web/tests/unit/snapshot-publish-gate.test.ts

### Requirement: Publishing cannot shrink capacity below what is sold or held
Publishing SHALL be refused when a pool instance's new capacity is below the units the on-sale engine reports as sold or held for it, or when a product or pool with such units would be removed, and the refusal MUST show the numbers.

#### Scenario: Capacity below usage
- **WHEN** the core reports 1,200 units used in pool instance "Arena" and the organiser sets its capacity to 1,000
- **THEN** publish is refused with "Arena: 1,200 sold or held, new capacity 1,000"
Test: apps/web/tests/unit/snapshot-publish-gate.test.ts

#### Scenario: Core unreachable
- **WHEN** the usage check cannot reach the core
- **THEN** publish is refused with a retry message rather than published unchecked
Test: apps/web/tests/unit/snapshot-publish-gate.test.ts

### Requirement: Snapshots are immutable and versioned
Each snapshot SHALL have a unique id, a version that increases by one per event, a content hash and its publication time and author, and MUST never be changed or deleted after it is written.

#### Scenario: Attempted change
- **WHEN** any database role tries to update or delete a stored snapshot
- **THEN** the database refuses it
Test: services/internal/snapshot/immutable_test.go

#### Scenario: Concurrent publishes
- **WHEN** two publishes for the same brand run at the same moment
- **THEN** versions are not duplicated or skipped: one publish wins and the other is refused or applied after it
Test: apps/web/tests/unit/snapshot-compile.test.ts

### Requirement: Snapshot contents are complete and self-contained
A snapshot SHALL contain everything the on-sale engine and the published shop need without reading the catalogue: currency, time zone, fee policy, the event's slug, venue (name, address, coordinates), line-up, event days with opening hours, buying rules, pools with their day or slot instances and capacities, and products with type settings, pools, requirements, sales windows, per-order maxima, price steps and instalment settings.

#### Scenario: Day-scoped pool instances
- **WHEN** a pool scoped to each day of a 74-day season is compiled
- **THEN** the snapshot lists 74 instances, each with a stable id made of the pool and the date, and the pool's capacity
Test: apps/web/tests/unit/snapshot-compile.test.ts

#### Scenario: Slot-scoped pool instances
- **WHEN** a padel pool of 2 courts with 30-minute slots from 17:00 to 02:30 is compiled for one day
- **THEN** the snapshot lists 20 slot instances for that day, each of capacity 2, with ids made of the pool and the local start time
Test: apps/web/tests/unit/snapshot-compile.test.ts

#### Scenario: Matches the shared schema
- **WHEN** any seeded event is compiled
- **THEN** the document validates against the shared snapshot schema used by both the web app and the core
Test: services/internal/snapshot/schema_test.go, apps/web/tests/unit/snapshot-compile.test.ts

### Requirement: Snapshots carry precomputed all-in amounts
Each price step in a snapshot SHALL carry the face amount, the all-in amount the fan pays, the fan's fee share and the organiser's net, in integer minor units, so that no consumer recomputes fees.

#### Scenario: GA price step
- **WHEN** GA at 539.00 RON with fee policy "pass on" and a 2.5% fee is compiled
- **THEN** its step holds face 53900, all-in 55248, fan fee 1348 and organiser net 53900
Test: apps/web/tests/unit/snapshot-compile.test.ts

#### Scenario: Amounts add up
- **WHEN** any snapshot is compiled
- **THEN** for every step the all-in amount equals face plus fan fee
Test: services/internal/snapshot/schema_test.go, apps/web/tests/unit/snapshot-compile.test.ts

### Requirement: The core reads only published snapshots
The Go core SHALL obtain sale terms only from live or earlier published snapshots, through a read that returns published data only, and MUST verify each snapshot's content hash when loading it.

#### Scenario: Draft change during a sale
- **WHEN** the organiser changes a price in the catalogue without publishing
- **THEN** the core keeps selling at the published price
Test: services/internal/snapshot/store_test.go

#### Scenario: Hash mismatch
- **WHEN** a cached snapshot's content does not match its hash
- **THEN** the core discards the cached copy, reloads from the database and logs the mismatch
Test: services/internal/snapshot/store_test.go

#### Scenario: Earlier version for an order in flight
- **WHEN** a request names an event and an earlier snapshot version
- **THEN** the core can load that version unchanged
Test: services/internal/snapshot/store_test.go

### Requirement: New versions reach the core promptly
After a publish the control plane SHALL tell the core which snapshot version is live, and the core MUST serve the new version within 10 seconds even if that message is lost.

#### Scenario: Activation message
- **WHEN** a publish commits
- **THEN** the core is told the event's new version and serves it on its next request
Test: services/internal/snapshot/store_test.go

#### Scenario: Activation lost
- **WHEN** the activation call fails after the publish committed
- **THEN** the publish stays done and the core serves the new version within 10 seconds
Test: services/internal/snapshot/store_test.go

#### Scenario: Activation for another organiser's event
- **WHEN** an organiser token asks the core to activate an event of another organiser
- **THEN** the core returns 404
Test: services/internal/snapshot/store_test.go, services/internal/coreapi/snapshots_test.go

### Requirement: Public availability per event
The core SHALL serve a public, short-cached availability answer per event from its live snapshot: for each visible product its sale status, current price step and all-in amount, and the remaining quantity only when few are left.

#### Scenario: Before sales start
- **WHEN** a product's sales window opens tomorrow
- **THEN** its status is "not yet" with the opening time
Test: services/internal/snapshot/availability_test.go

#### Scenario: Plenty left
- **WHEN** more than 50 units and more than 5% of capacity remain
- **THEN** the answer says "on sale" and omits the remaining quantity
Test: services/internal/snapshot/availability_test.go

#### Scenario: Hidden products
- **WHEN** a product is not visible
- **THEN** it does not appear in the public answer
Test: services/internal/snapshot/availability_test.go

### Requirement: Studio shows publication state
Studio SHALL show for each event its live snapshot version and time, and whether the catalogue has changes that are not yet published.

#### Scenario: Unpublished changes
- **WHEN** the organiser edits a product after publishing
- **THEN** the event shows "Changes not published" until the next publish
Test: apps/web/tests/e2e/publish.spec.ts
