# Proposal

Step 4: Checkout and payments

**Demo act:** Act 2 (the live on-sale): the checkout every simulated and real fan goes through, the failing provider A, the silent failover to B, and one-tap payment from the EPS Wallet.

**Features covered:** D2 cart hold with timer, D3 clear order status, D4 two simulated providers, D5 automatic failover, D6 no double charges, D7 resume payment link (Real), D8 guest checkout and account claim, D9 checkout question builder (Thin), D11 instalment checkout (Real), D12 EPS Wallet (Real, ledger), D13 Apple Pay / Google Pay / saved cards (Real, simulated providers only).
**Decisions covered:** O3 payments state machine and failover, O4 fake payment providers, O6 wallet as a ledger, P3 outbox and Redis Streams. Builds on P1 (orders record the snapshot version), U1 (JWTs and guests), U2 (organiser isolation) and O2 (holds; the sharded version arrives in step 5).

## Why

EPS today holds a cart for 5 minutes, sends the fan to a bank page and then shows a success screen without checking the payment; it has no declined state, no resume link, no idempotency and no automatic use of its second acquirer (Netopia) when BT iPay fails (teardown sections 4, 6 and 10). The on-sale in Act 2 is only credible if checkout survives a failing provider, retries and double clicks without charging twice or losing the fan's tickets, so this step comes before the queue (step 5) and the simulator (step 6) that load it.

## What Changes

- Timed holds on stock (8 minutes by default) with a visible countdown, released automatically when they lapse, all-or-nothing across every capacity pool an order draws from.
- Orders and payment attempts as explicit, persisted state machines, with one honest status page that only shows "paid" after the provider's signed webhook (or a status check) confirms it, and quotable error codes for every failure.
- Two fake payment providers (A and B) running as their own Go service behind a real network boundary, with live knobs for failure rate, decline rate, latency, outage, dropped or late webhooks and the 3-D Secure style challenge; a hosted payment page with test outcomes only, so no card data ever reaches EPS.
- A provider adapter with a shared circuit breaker: when A fails too often, new attempts go to B without the fan noticing; an attempt whose outcome is unknown is never retried on another provider until it is resolved.
- Idempotency keys on every payment request and on every provider call, so retries, refreshes and double clicks never charge twice.
- A signed resume link for a held, unpaid order; guest checkout with a later claim into an account; organiser-defined checkout questions and consents in the locked checkout's fixed slots (Thin).
- Instalment checkout: choose a plan, see the exact schedule, pay the deposit now, later instalments charged to a provider-saved card through the fake providers.
- Simulated Apple Pay and Google Pay sheets and provider-saved cards for one-tap payment.
- The EPS Wallet as a double-entry ledger per fan and organiser: top up through the fake providers, pay from balance in one atomic move, a simulated stand payment, and season credit. In production this needs an e-money licence or a regulated partner; the prototype holds no money.
- A transactional outbox in the sales schema relayed to Redis Streams, the event backbone that the worker, the step 6 dashboard and later webhooks read.

## Capabilities

### New Capabilities
- `checkout`: holds and their timer, order states and the status page, guest checkout and claim, checkout questions, the resume link, instalment checkout.
- `payments`: provider adapters, the fake providers and their knobs, failover, idempotency, webhooks and reconciliation of unknown outcomes, Apple Pay / Google Pay and saved cards (all simulated).
- `fan-wallet`: the ledger, top-ups, paying from balance, stand payments and season credit.
- `event-stream`: the transactional outbox, the relay to Redis Streams, the event envelope and consumer guarantees.

### Modified Capabilities
None. Behaviour that touches earlier steps (snapshot versions, all-in prices, guest identities) is consumed as given; the queue pass that checkout will require is added by step 5 in its own `waiting-room` capability.

## Impact

- **Go (`services/`):** new packages for orders, holds (behind an inventory interface the step 5 sharded engine replaces), payments (adapter, breaker, router), wallet ledger, outbox relay; `cmd/core` gains the checkout, order, payment, wallet and webhook endpoints; `cmd/worker` gains the hold sweeper, payment reconciler, instalment runner and outbox relay; `cmd/fakepsp` is implemented and deployed twice (psp-a, psp-b).
- **Sales schema (Neon):** orders, order lines, answers, holds, payment attempts, idempotency records, instalment plans and schedule rows, saved payment method references, wallet accounts, journal entries and lines, outbox. All rows carry `organiser_id` under row-level security.
- **Redis:** hold keys and expiry index, breaker windows, streams.
- **API contract (`api/openapi.yaml`):** checkout, orders, payments, wallet, questions and provider-webhook paths.
- **Next.js (`apps/web`):** checkout screens inside the locked checkout (timer, contact, questions, payment choice, instalments, wallet), the order status page, the resume page, wallet screens, a Studio questions editor and a Studio provider-knobs panel.
- **Helm:** psp-a and psp-b releases of the fakepsp sub-chart; secrets for webhook signing and resume-link signing, generated by CI.
- **No new dependencies** beyond the decided stack.
