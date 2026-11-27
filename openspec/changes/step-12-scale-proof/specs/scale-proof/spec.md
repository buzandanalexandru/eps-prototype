# Spec Delta

## Purpose

Proves, honestly, how EPS handles a 100,000-fan on-sale in 10 minutes: a deployable chart with autoscaling, measured per-instance load results with their method, the scaling maths built on them, a roadmap of what comes next, and a roll-up showing every specified behaviour is tested.

## ADDED Requirements

### Requirement: One chart deploys the data plane
The system SHALL install every Go service (gateway, core, worker, fake providers, simulator) and Redis on a Kubernetes cluster from the one Helm chart in the repository, with resource requests and limits, readiness and liveness checks, and a disruption budget for the gateway and core.

#### Scenario: Fresh install
- **WHEN** the chart is installed with its default values into an empty namespace on DOKS fra1
- **THEN** every deployment becomes ready and the gateway and core health endpoints answer
- Test: manual check on the deployed preview (CI helm upgrade log and all pods ready in the PR namespace)

#### Scenario: Node drained during a sale
- **WHEN** a node running a gateway pod is drained
- **THEN** at least one gateway pod and one core pod stay ready throughout
- Test: manual check on the deployed preview (kubectl drain on a preview node while smoke.js runs, zero failed requests above 1%)

### Requirement: Gateway and core scale out under load
The system SHALL scale the gateway (2 to 12 pods) and core (2 to 8 pods) horizontally on CPU use and requests per second per pod, scaling up without delay and scaling down only after 5 minutes of lower load.

#### Scenario: Load beyond one pod
- **WHEN** load rises to three times one pod's measured maximum
- **THEN** the number of pods grows within 2 minutes and the error rate stays under 1%
- Test: load/autoscale.js

#### Scenario: Short dip in load
- **WHEN** load falls for 2 minutes and then rises again
- **THEN** no pods are removed during the dip
- Test: load/autoscale.js

### Requirement: Per-instance throughput is measured
The system SHALL have k6 load tests that find the highest sustained rate one gateway pod and one core pod handle within the thresholds (p95 under 150 ms for the gateway and 300 ms for checkout, errors under 1%), run inside the cluster.

#### Scenario: Gateway queue capacity
- **WHEN** the queue-join test steps up joins and position polls against one gateway pod
- **THEN** the result records the highest step that met all thresholds as the gateway's per-pod maximum
- Test: load/queue-join.js

#### Scenario: Checkout capacity
- **WHEN** the checkout test steps up admitted fans taking holds and paying against one core pod
- **THEN** the result records the highest orders per second that met all thresholds, the requests per order, and zero oversold units
- Test: load/checkout.js

### Requirement: Correct under payment failure
The system SHALL keep charging each order at most once and keep selling while one fake provider fails completely during a load test.

#### Scenario: Provider A fails mid-run
- **WHEN** provider A switches to 100% failure two minutes into a steady 50 orders per second
- **THEN** new payments move to provider B, the number of charges equals the number of paid orders, and the p95 during the switch is recorded
- Test: load/failover.js

### Requirement: Results recorded with their method
The system SHALL store every load-test result in the repository with its date, commit, chart version, replicas, node size and provider settings, and SHALL show only numbers taken from those stored results.

#### Scenario: Numbers on the page come from results
- **WHEN** the scale page is built
- **THEN** every measured number on it equals the value in the stored result it cites, and the page shows that result's date and setup
- Test: apps/web/tests/unit/scale-results.test.ts

### Requirement: Scaling maths to 100k in 10 minutes
The system SHALL calculate, from the measured per-pod numbers, the gateway and core pods needed for 100,000 fans in 10 minutes (167 orders per second on average, planned at twice that), the queue clearing time at the safe admission rate, and which shared limit binds first.

#### Scenario: Pods needed
- **WHEN** the measured core maximum is 120 orders per second per pod at 6 requests per order
- **THEN** the maths shows ceil(333 × 6 ÷ (720 × 0.6)) = 5 core pods, with each input visible
- Test: apps/web/tests/unit/scale-maths.test.ts

#### Scenario: Queue clearing time
- **WHEN** the admission rate is set to 300 fans per second
- **THEN** the page shows that 100,000 fans clear the queue in about 6 minutes
- Test: apps/web/tests/unit/scale-maths.test.ts

### Requirement: Honest labels
The system SHALL label every figure on the scale page as Measured, Simulated or Calculated, and SHALL state that no real 100,000-client test was run.

#### Scenario: Labels and statement
- **WHEN** a visitor opens `/scale` in Romanian or English
- **THEN** each figure carries one of the three labels and the statement about the method is visible above the figures
- Test: apps/web/tests/e2e/scale-page.spec.ts

### Requirement: Roadmap page
The system SHALL serve a public `/roadmap` page in Romanian and English listing the Vision items (refunds and cancellation first), the items cut in review and the out-of-scope items, each with a short reason and none linking to a screen.

#### Scenario: Refunds named as next
- **WHEN** a visitor opens `/roadmap`
- **THEN** refunds and cancellation appear under Next with the reason that they follow the payment flow, and the cut and out-of-scope items appear in their own sections
- Test: apps/web/tests/e2e/roadmap.spec.ts

### Requirement: Every scenario names an existing test
The system SHALL check on every pull request that each scenario in the specs ends with a test line naming a test file that exists or a described manual check, and SHALL show the counts of scenarios, automated tests, manual checks and passing suites on the scale page.

#### Scenario: Missing test file
- **WHEN** a scenario's test line names a file that does not exist
- **THEN** the check fails and lists the scenario and the missing path
- Test: apps/web/tests/unit/spec-coverage.test.ts

#### Scenario: Roll-up shown
- **WHEN** CI has run on `main`
- **THEN** the scale page shows the scenario and test counts from that run
- Test: apps/web/tests/e2e/scale-page.spec.ts
