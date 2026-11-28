# Spec Delta

## Purpose

Gives each fan a simulated prepaid balance per organiser, kept as a double-entry ledger, that can be topped up in advance, used to pay instantly during an on-sale or at a stand, and turned into next-season credit.

## ADDED Requirements

### Requirement: Double-entry ledger
Every wallet money movement SHALL be a journal entry whose lines add up to zero in each currency. Lines SHALL never be edited or deleted; corrections SHALL be reversing entries. A fan's balance SHALL always equal the sum of that fan's journal lines and SHALL never go below zero.

#### Scenario: Balanced entries
- **WHEN** any top-up, payment, stand payment or season close is recorded
- **THEN** its lines add up to zero and every cached balance equals the sum of its lines
- Test: services/internal/wallet/ledger_test.go

#### Scenario: Unbalanced entry refused
- **WHEN** code tries to write an entry whose lines do not add up to zero
- **THEN** the transaction is rolled back and no balance changes
- Test: services/internal/wallet/ledger_test.go

### Requirement: Wallet per fan per organiser
A wallet balance SHALL belong to one fan and one organiser, and SHALL be spendable only on that organiser's shops and stands. A fan SHALL see each organiser's balance separately.

#### Scenario: Other organiser's shop
- **WHEN** a fan with 300 RON on NIBIRU's wallet checks out at another organiser's shop
- **THEN** the wallet is not offered there and the NIBIRU balance is unchanged
- Test: services/internal/wallet/ledger_test.go

### Requirement: Top up through the simulated providers
A fan SHALL be able to top up the wallet by an amount within the organiser's limits through the simulated providers. The balance SHALL increase only when the provider confirms the charge, and a repeated top-up request with the same idempotency key SHALL credit once.

#### Scenario: Top-up confirmed
- **WHEN** a fan tops up 200 RON and the provider confirms
- **THEN** the balance rises by 200 RON and the history shows the top-up
- Test: services/internal/wallet/topup_test.go
- Test: apps/web/tests/e2e/wallet.spec.ts

#### Scenario: Top-up declined
- **WHEN** the provider declines the top-up
- **THEN** the balance does not change and the fan sees the decline with its code
- Test: services/internal/wallet/topup_test.go

### Requirement: Pay from balance in one tap
Checkout SHALL offer the wallet when the balance covers the order total. Paying SHALL debit the balance, commit the hold and mark the order paid in one atomic step, with no provider involved. A balance that does not cover the total SHALL be refused with `EPS-WALLET-INSUFFICIENT` and the hold SHALL stay.

#### Scenario: One-tap during a drop
- **WHEN** a fan with 600 RON taps "Pay from wallet" for a 539 RON order
- **THEN** the order is `paid` in one step, the balance is 61 RON, and no provider was called
- Test: services/internal/wallet/pay_test.go
- Test: apps/web/tests/e2e/wallet.spec.ts

#### Scenario: Two payments race for one balance
- **WHEN** two orders of 400 RON try to pay from a 600 RON balance at the same moment
- **THEN** exactly one succeeds and the other is refused with `EPS-WALLET-INSUFFICIENT`
- Test: services/internal/wallet/pay_test.go

### Requirement: Simulated stand payment
A simple stand screen SHALL let an organiser's stand take a payment from a fan's balance for that organiser, recorded in the ledger as a stand sale.

#### Scenario: Pay at a stand
- **WHEN** a stand charges 25 RON to a fan's wallet
- **THEN** the balance drops by 25 RON and the ledger shows a stand sale
- Test: services/internal/wallet/ledger_test.go
- Test: manual check on the deployed preview (open the stand screen, charge a fan's wallet, see the balance and history change)

### Requirement: Season credit
An organiser SHALL be able to close a season, which SHALL move each fan's unspent balance to next-season credit with an expiry date. Credit SHALL be spendable like a balance on that organiser's shops.

#### Scenario: Season closes
- **WHEN** the organiser closes the season while a fan holds 80 RON
- **THEN** the fan's balance is 0, the fan's season credit is 80 RON with its expiry, and the ledger shows the move
- Test: services/internal/wallet/season_credit_test.go

### Requirement: Simulated money is labelled
Every wallet screen SHALL say that the balance is simulated and that no real money is held.

#### Scenario: Label shown
- **WHEN** a fan opens the wallet in Romanian or English
- **THEN** the screen shows the simulated-balance notice in that language
- Test: apps/web/tests/e2e/wallet.spec.ts
