# Spec Delta

## Purpose

Lets a fan, signed in or as a guest, hold tickets for a limited time, answer the organiser's questions, choose how to pay (in full, in instalments or from the wallet) and follow one honest order status from hold to issued, expired or declined.

## ADDED Requirements

### Requirement: Timed hold on stock
Creating an order SHALL hold the requested quantities from every capacity pool the order's products draw from, all or nothing, for the hold length set in the sale's buying rules (default 8 minutes, allowed 5 to 15). The hold SHALL never take more stock than is free, and the order SHALL record the snapshot version it was priced under.

#### Scenario: Hold succeeds across shared pools
- **WHEN** a fan orders 2 GA Plus tickets that draw from the Arena pool and the GA Plus pool and both have stock
- **THEN** the order is created in state `held` with a deadline 8 minutes ahead, both pools drop by 2, and the order records the snapshot version
- Test: services/internal/checkout/hold_test.go

#### Scenario: One pool short means nothing is held
- **WHEN** a fan orders a product whose second pool has only 1 place left for a quantity of 2
- **THEN** no stock is taken from any pool and the response is `EPS-HOLD-SOLD-OUT` naming the product
- Test: services/internal/checkout/hold_test.go

#### Scenario: Concurrent holds never oversell
- **WHEN** 500 concurrent hold requests of 1 ticket arrive for a pool with 100 places
- **THEN** exactly 100 holds succeed and the pool never shows less than zero
- Test: services/internal/checkout/hold_race_test.go

### Requirement: Holds follow the live snapshot's capacity and terms
Holds and order confirmation SHALL never take a pool instance above its capacity in the live sale snapshot, and an instance whose sold and held units reach or exceed that capacity MUST be treated as sold out. The core SHALL re-price every order from snapshot `v`, refuse hidden products and off-sale events, and apply the per-order and per-account maxima again, whatever the checkout URL says.

#### Scenario: Capacity lowered below what is taken
- **WHEN** a pool instance has 1,000 units sold or held and a publish sets its capacity to 900 (the step 3 gate was bypassed or raced)
- **THEN** a new hold for it fails with `EPS-HOLD-SOLD-OUT` and confirming an older hold never takes the instance above 900 sold
- Test: services/internal/checkout/hold_test.go

#### Scenario: Hidden product or off-sale event
- **WHEN** a checkout URL names a hidden product, a product of another snapshot, or an event that is off sale
- **THEN** the hold is refused and nothing is taken
- Test: services/internal/checkout/hold_test.go

### Requirement: Visible countdown and automatic release
The checkout SHALL show the time left on the hold, counting down from the server's deadline. When the deadline passes without a payment in progress, the system SHALL release the stock within 5 seconds and move the order to `expired`.

#### Scenario: Countdown follows the server
- **WHEN** a fan opens the checkout 2 minutes after creating the hold
- **THEN** the timer shows about 6 minutes left, computed from the server deadline, not from page load
- Test: apps/web/tests/e2e/checkout-hold.spec.ts

#### Scenario: Hold lapses
- **WHEN** a held order reaches its deadline without a payment attempt
- **THEN** within 5 seconds the stock is back in its pools, the order is `expired` and the page says the tickets were released
- Test: services/internal/checkout/sweeper_test.go

#### Scenario: Payment in flight keeps the hold
- **WHEN** a payment attempt is still unresolved at the hold deadline
- **THEN** the hold is kept until the attempt resolves or its 120-second limit passes, and the order is not expired meanwhile
- Test: services/internal/checkout/sweeper_test.go

### Requirement: One live hold per buyer per sale
The system SHALL allow at most one order in `held`, `paying` or `payment_failed` per buyer (account or guest session) per sale. A new hold request while one is live SHALL return the existing order instead of taking more stock.

#### Scenario: Second hold returns the first order
- **WHEN** a fan with a live held order sends another hold request for the same sale
- **THEN** no extra stock is taken and the response points to the existing order
- Test: services/internal/checkout/hold_test.go

### Requirement: Idempotent order creation
Order creation SHALL accept an idempotency key; a repeated request with the same key and body SHALL return the same order without taking stock again.

#### Scenario: Double click on Continue
- **WHEN** the same hold request with the same idempotency key is sent twice within a second
- **THEN** one order exists, stock is taken once, and both responses carry the same order id
- Test: services/internal/checkout/hold_test.go

### Requirement: Explicit order states
Every order SHALL be in exactly one of `held`, `paying`, `payment_failed`, `paid`, `issued`, `expired`, `cancelled` or `refunded`, and SHALL move only along the allowed transitions. A transition that is not allowed SHALL be refused and leave the order unchanged.

#### Scenario: Allowed path to issued
- **WHEN** a held order is paid and its tickets are issued
- **THEN** its history reads `held`, `paying`, `paid`, `issued` with a timestamp for each
- Test: services/internal/checkout/order_state_test.go

#### Scenario: Racing transitions
- **WHEN** the sweeper tries to expire an order at the same moment its payment succeeds
- **THEN** exactly one transition wins, and if payment won the order is `paid` with its stock committed
- Test: services/internal/checkout/order_state_test.go

#### Scenario: Forbidden transition
- **WHEN** something tries to move an `expired` order to `paying`
- **THEN** the change is refused and the order stays `expired`
- Test: services/internal/checkout/order_state_test.go

### Requirement: Honest order status page
The order status page SHALL show the order's current state from the server, refreshing at least every 2 seconds while the state can still change. It SHALL show "paid" only after the payment is confirmed by the provider, and SHALL show the reason and an error code for every failure.

#### Scenario: Waiting for the bank
- **WHEN** the fan returns from the provider page before the webhook arrives
- **THEN** the page shows "Waiting for confirmation from the payment provider" with the hold timer, not a success message
- Test: apps/web/tests/e2e/order-status.spec.ts

#### Scenario: Declined card
- **WHEN** the provider declines the payment
- **THEN** the page shows the decline in plain words, the code `EPS-PAY-DECLINED`, the time left on the hold and a button to try again
- Test: apps/web/tests/e2e/order-status.spec.ts

#### Scenario: Unknown order
- **WHEN** someone opens a status page for an order id that does not exist or without a valid order token
- **THEN** the page shows "Order not found" and no success screen
- Test: apps/web/tests/e2e/order-status.spec.ts

### Requirement: Fan can cancel a held order
A fan SHALL be able to cancel an order in `held` or `payment_failed`; the stock SHALL be released at once and the order SHALL become `cancelled`.

#### Scenario: Cancel releases stock
- **WHEN** a fan presses "Cancel order" on a held order
- **THEN** the order is `cancelled` and its stock is free for other fans immediately
- Test: services/internal/checkout/order_state_test.go

### Requirement: Late payment on an expired order
When a payment succeeds after its order expired or was cancelled, the system SHALL complete the order if the same stock can still be held, and otherwise SHALL refund the payment through the same provider and mark the order `refunded`.

#### Scenario: Stock still free
- **WHEN** a success webhook arrives for an expired order and the stock is still available
- **THEN** the stock is taken again and the order becomes `paid`
- Test: services/internal/checkout/late_payment_test.go

#### Scenario: Stock gone
- **WHEN** a success webhook arrives for an expired order whose stock was sold to someone else
- **THEN** the payment is refunded, the order is `refunded`, and the page explains it with code `EPS-HOLD-EXPIRED`
- Test: services/internal/checkout/late_payment_test.go

### Requirement: Resume payment link
A held or payment-failed order SHALL have a signed resume link that reopens the payment step on any device until the hold deadline. After the deadline, or once the order is paid, the link SHALL show the order's true state and SHALL NOT offer payment.

#### Scenario: Resume on another device
- **WHEN** a fan opens the resume link on a phone while the hold is live
- **THEN** the payment step opens with the same order, the same timer and the same total
- Test: apps/web/tests/e2e/resume-link.spec.ts

#### Scenario: Expired link
- **WHEN** the resume link is opened after the hold deadline
- **THEN** the page says the hold ended and offers to start a new order, with no payment button
- Test: services/internal/checkout/resume_test.go

#### Scenario: Tampered link
- **WHEN** the resume link's order id or deadline is changed by hand
- **THEN** the signature check fails and the page shows "Link not valid"
- Test: services/internal/checkout/resume_test.go

### Requirement: Guest checkout
A fan SHALL be able to buy without an account by giving an email address and the required contact details. The guest SHALL receive an order link that opens the order status page without signing in.

#### Scenario: Guest buys
- **WHEN** a guest completes checkout with an email address
- **THEN** the order belongs to that guest identity and the order link opens its status page
- Test: apps/web/tests/e2e/guest-checkout.spec.ts

### Requirement: Claim guest orders into an account
A signed-in fan whose email is verified SHALL be able to claim guest orders placed with the same email, after which those orders belong to the account. Orders placed with another email SHALL NOT be claimable.

#### Scenario: Claim succeeds
- **WHEN** a fan signs in with the verified email used for two guest orders and presses "Add to my account"
- **THEN** both orders now belong to the account and show in its order list
- Test: services/internal/checkout/claim_test.go

#### Scenario: Different email
- **WHEN** a signed-in fan tries to claim an order placed with a different email
- **THEN** the claim is refused and the order is unchanged
- Test: services/internal/checkout/claim_test.go

### Requirement: Checkout questions and consents
An organiser SHALL be able to add up to 5 questions or consents (short text, single choice, checkbox) to the locked checkout's fixed slots, each required or optional. Checkout SHALL show them, SHALL refuse to start payment until required ones are answered, and SHALL store the answers with the order.

#### Scenario: Required consent missing
- **WHEN** a fan presses Pay without ticking a required consent
- **THEN** payment does not start and the consent is marked as needed
- Test: manual check on the deployed preview (add a required consent in Studio, then try to pay without it on the shop)

### Requirement: Instalment checkout
When a product allows instalments, checkout SHALL offer the plan, show the full schedule (dates and amounts in the shop's currency) before payment, and charge the deposit now. The parts SHALL add up exactly to the all-in total, and the order SHALL become `paid` when the deposit succeeds.

#### Scenario: Schedule adds up
- **WHEN** a fan picks a 10 RON deposit plus the monthly payments for a 539 RON order
- **THEN** the schedule shows the deposit and the monthly parts, whose sum is exactly 539 RON, with any rounding remainder on the last part
- Test: services/internal/checkout/instalments_test.go

#### Scenario: Plan capped per buyer
- **WHEN** a product offers 10 RON now and up to 9 monthly payments for an event on 15 Jul 2027 and a fan buys on 5 Dec 2026
- **THEN** checkout offers that fan 10 RON now and 7 monthly payments (1 Jan to 1 Jul 2027, in the event's time zone), because the last payment must fall at least 7 days before the event; a fan buying later is offered fewer
- Test: services/internal/checkout/instalments_test.go
- Test: services/internal/checkout/instalments_test.go

#### Scenario: Deposit pays the order
- **WHEN** the deposit payment succeeds
- **THEN** the order is `paid`, the plan is `active`, and the remaining parts are `scheduled` against a provider-saved card
- Test: apps/web/tests/e2e/instalment-checkout.spec.ts

#### Scenario: Later instalment fails
- **WHEN** a scheduled part is charged and the simulated provider declines it
- **THEN** the part becomes `overdue` with a 7-day grace period and the fan sees "Pay now" and "Change card"
- Test: services/internal/checkout/instalments_test.go

### Requirement: All-in total at checkout
The checkout total SHALL equal the sum of the all-in prices shown on the storefront for the same snapshot version; no fee SHALL appear for the first time at checkout. The fee breakdown SHALL be stored with the order for reporting.

#### Scenario: Same price as the shop
- **WHEN** a fan adds a ticket shown at 539 RON on the shop and opens checkout
- **THEN** the checkout total is 539 RON and the stored order lines carry the fee split from the fee policy
- Test: services/internal/checkout/pricing_test.go
