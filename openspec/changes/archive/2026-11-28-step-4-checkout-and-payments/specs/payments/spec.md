# Spec Delta

## Purpose

Takes payment for orders and wallet top-ups through simulated providers behind one adapter, with explicit attempt states, idempotency, automatic failover between providers and one-tap methods, without EPS ever handling card data or real money.

## ADDED Requirements

### Requirement: Payment attempt states
Every payment attempt SHALL be persisted and SHALL be in one of `created`, `submitted`, `challenge_required`, `succeeded`, `declined`, `failed`, `unknown`, `refund_pending` or `refunded`. An order SHALL have at most one unresolved attempt and at most one `succeeded` attempt at a time.

#### Scenario: Normal success
- **WHEN** a fan pays and the provider accepts and later confirms the charge
- **THEN** the attempt moves `created`, `submitted`, `succeeded` and the order becomes `paid`
- Test: services/internal/payments/attempt_state_test.go

#### Scenario: Second attempt while one is open
- **WHEN** a new attempt is requested while the order already has a `submitted` attempt
- **THEN** no new attempt is created and the response returns the open attempt
- Test: services/internal/payments/attempt_state_test.go

### Requirement: No double charges
Every payment request SHALL carry an idempotency key. A repeat with the same key and body SHALL return the original result without a new charge; the same key with a different body SHALL be refused with `EPS-IDEMPOTENCY-MISMATCH`. Every call to a provider SHALL carry the attempt's own idempotency key.

#### Scenario: Double click on Pay
- **WHEN** the Pay request is sent twice with the same idempotency key
- **THEN** the provider records one charge and both responses show the same attempt
- Test: services/internal/payments/idempotency_test.go

#### Scenario: Retry after a lost response
- **WHEN** the core's call to the provider times out and the core retries with the same attempt key
- **THEN** the provider returns the charge it already made, not a second one
- Test: services/internal/payments/idempotency_test.go

#### Scenario: Reused key, different amount
- **WHEN** a request reuses an idempotency key with a different order or amount
- **THEN** it is refused with `EPS-IDEMPOTENCY-MISMATCH` and nothing is charged
- Test: services/internal/payments/idempotency_test.go

### Requirement: Two simulated providers with live knobs
The platform SHALL run two simulated providers, A and B, as a separate service reached over the network. Each SHALL have live settings for failure rate, decline rate, latency, outage (answering 503 by default, or hanging), dropped or delayed webhooks and the share of charges needing a challenge code, per organiser, changeable from Studio without a restart; changed settings SHALL go back to normal 15 minutes after the last change, and Studio SHALL show when.

#### Scenario: Outage switched on
- **WHEN** an operator switches on provider A's outage in Studio
- **THEN** within 2 seconds every new call to A fails or hangs until timeout, and switching it off restores A
- Test: services/internal/fakepsp/knobs_test.go

#### Scenario: Failure rate
- **WHEN** provider A's failure rate is set to 0.3 and 1,000 charges are sent
- **THEN** between 250 and 350 of them fail before acceptance
- Test: services/internal/fakepsp/knobs_test.go

### Requirement: No card data in EPS
Card payments SHALL be entered only on the simulated provider's hosted page, which offers test outcomes and no card number field. EPS SHALL store only the provider, the charge id, a saved-card token and a display label, and SHALL never receive or store a card number.

#### Scenario: Hosted test outcomes
- **WHEN** a fan chooses card payment
- **THEN** the provider's page opens with test outcomes (approve, decline, challenge failed, cancel) and there is no field to type a card number
- Test: apps/web/tests/e2e/payment-hosted.spec.ts

### Requirement: Automatic failover
When a provider's circuit breaker is open, new attempts SHALL go to the other provider. An attempt that fails before the provider accepted it SHALL be retried once on the other provider within the same Pay press, without the fan seeing an error. Declines SHALL NOT trigger failover.

#### Scenario: A fails, B pays
- **WHEN** provider A returns errors for a payment and B is healthy
- **THEN** the payment completes on B, the fan sees only a successful payment, and a failover event is recorded
- Test: apps/web/tests/e2e/checkout-failover.spec.ts
- Test: services/internal/payments/router_test.go

#### Scenario: Decline is not failed over
- **WHEN** provider A declines the card
- **THEN** no attempt is made on B and the fan sees the decline with `EPS-PAY-DECLINED`
- Test: services/internal/payments/router_test.go

#### Scenario: Both providers down
- **WHEN** both breakers are open
- **THEN** no attempt is sent, the fan sees `EPS-PAY-PROVIDERS-BUSY` with "Your tickets are still held", and the hold keeps running
- Test: services/internal/payments/router_test.go

### Requirement: Circuit breaker
Each provider SHALL have one breaker per organiser (merchant account) shared by all cores. It SHALL open when, in the last 20 seconds, at least 20 calls were made and half failed (refused connections or the provider's 503; not declines), or after 5 failures in a row; after 15 seconds it SHALL let up to 5 probe payments through, close when 4 succeed, and reopen for twice as long (at most 120 seconds) when a second probe fails. A probe slot not used within 10 seconds SHALL be given out again.

#### Scenario: Opens under failure
- **WHEN** 12 of 20 calls to A in 20 seconds fail
- **THEN** A's breaker opens and every core instance routes new attempts to B within 1 second
- Test: services/internal/payments/breaker_test.go

#### Scenario: Recovers
- **WHEN** A's breaker has been open for 15 seconds and 4 of 5 probe payments succeed
- **THEN** the breaker closes and A takes new attempts again
- Test: services/internal/payments/breaker_test.go

### Requirement: Unknown outcomes are resolved before failover
When a call to a provider times out or ends without a usable answer (a reset connection, a 502, 504 or another 5xx than 503), the attempt SHALL be `unknown` and SHALL be resolved by asking that provider for the charge by its idempotency key. Another provider SHALL be tried only once the first reports no such charge.

#### Scenario: Timeout, charge did happen
- **WHEN** A accepts a charge but its response is lost, and the status check finds the charge succeeded
- **THEN** the attempt becomes `succeeded`, the order `paid`, and B is never called
- Test: services/internal/payments/unknown_outcome_test.go

#### Scenario: Timeout, no charge
- **WHEN** the status check on A reports no such charge
- **THEN** the attempt becomes `failed` and a new attempt goes to B
- Test: services/internal/payments/unknown_outcome_test.go

### Requirement: Signed webhooks confirm payments
The core SHALL accept a provider's result only from a webhook with a valid signature no older than 5 minutes, or from its own status query. Duplicate webhooks SHALL have no further effect, and an attempt still unresolved after 20 seconds SHALL be checked with the provider.

#### Scenario: Forged webhook
- **WHEN** a webhook arrives with a wrong signature
- **THEN** it is rejected with 401 and no attempt or order changes
- Test: services/internal/payments/webhook_test.go

#### Scenario: Duplicate webhook
- **WHEN** the same success webhook arrives three times
- **THEN** the order is paid once and one outbox event is written
- Test: services/internal/payments/webhook_test.go

#### Scenario: Dropped webhook
- **WHEN** the provider never sends the webhook for a successful charge
- **THEN** within 30 seconds the reconciler finds the success and the order becomes `paid`
- Test: services/internal/payments/reconciler_test.go

### Requirement: Simulated bank challenge
A provider SHALL be able to ask for a one-time code before confirming a charge, shown on its own page as a simulated bank code. A wrong code three times SHALL decline the attempt with `EPS-PAY-3DS-FAILED`.

#### Scenario: Challenge passed
- **WHEN** the provider asks for a code and the fan enters the code shown
- **THEN** the charge proceeds and the order becomes `paid` on the webhook
- Test: services/internal/fakepsp/challenge_test.go

#### Scenario: Challenge failed
- **WHEN** the fan enters a wrong code three times
- **THEN** the attempt is `declined` with `EPS-PAY-3DS-FAILED` and the order is `payment_failed` with its hold still live
- Test: services/internal/fakepsp/challenge_test.go
- Test: services/internal/payments/attempt_state_test.go

### Requirement: Simulated Apple Pay and Google Pay
Checkout SHALL offer Apple Pay and Google Pay as one-tap methods that open a simulated wallet sheet from the provider, clearly labelled as simulated, and complete through the same attempt states, idempotency and failover as cards.

#### Scenario: One-tap with Apple Pay
- **WHEN** a fan taps "Apple Pay (simulated)" and confirms in the sheet
- **THEN** the order is paid through the provider with method `apple_pay`, with no card page shown
- Test: apps/web/tests/e2e/one-tap-payments.spec.ts

#### Scenario: Google Pay during failover
- **WHEN** a fan pays with Google Pay while A's breaker is open
- **THEN** the payment completes on B
- Test: services/internal/payments/router_test.go

### Requirement: Saved cards kept by the provider
A signed-in fan SHALL be able to save a card during a payment; the provider SHALL keep it and EPS SHALL store only its token and label. The fan SHALL be able to pay with a saved card in one tap and to delete it. A saved card SHALL be usable only by the account and organiser it was saved for.

#### Scenario: Pay with a saved card
- **WHEN** a fan with a saved "Test Visa •••• 4242" taps it at checkout
- **THEN** the order is paid without opening the card page
- Test: apps/web/tests/e2e/one-tap-payments.spec.ts
- Test: services/internal/payments/saved_cards_test.go

#### Scenario: Token from another account
- **WHEN** a request uses a saved-card token that belongs to another account
- **THEN** it is refused and nothing is charged
- Test: services/internal/payments/saved_cards_test.go

#### Scenario: Card of an active plan
- **WHEN** the fan tries to delete the card an active instalment plan charges
- **THEN** it is refused with `EPS-SAVED-CARD-IN-USE` and stays until the plan ends
- Test: services/internal/payments/regression_test.go

#### Scenario: Delete a saved card
- **WHEN** the fan deletes a saved card
- **THEN** it no longer appears at checkout and the provider token is revoked
- Test: services/internal/payments/saved_cards_test.go

### Requirement: Refunds for late or duplicate charges
The system SHALL refund, through the provider that charged and with its own idempotency key, any charge that succeeded for an order that could not be completed, and any second success on the same order.

#### Scenario: Duplicate success refunded
- **WHEN** two attempts on one order both report success
- **THEN** the second is refunded automatically, the order is paid once, and an alert event is recorded
- Test: services/internal/payments/refund_test.go
