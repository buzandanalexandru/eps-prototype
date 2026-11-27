# Spec Delta

## Purpose

Lets fans resell a ticket inside the platform at no more than face value, or pass it to a friend for free, with the old code cancelled, a new one issued to the new holder and the seller paid out under the organiser's resale policy.

## ADDED Requirements

### Requirement: Organiser sets a resale policy per event
Each event SHALL have a resale policy set by its organiser: resale on or off, an opening time, a closing time (default 2 hours before doors) and a published resale fee per ticket. Listing and transfer MUST be possible only while resale is on and open.

#### Scenario: Resale switched off
- **WHEN** a holder opens a ticket for an event with resale off
- **THEN** no Sell or Transfer action is shown and the API refuses both
Test: services/internal/resale/policy_test.go

#### Scenario: After the closing time
- **WHEN** a holder tries to list or transfer a ticket after resale has closed
- **THEN** the request is refused with the closing time
Test: services/internal/resale/policy_test.go

### Requirement: Resale price is capped at face value
A listing's price SHALL be at least 1 and at most the ticket's face value: the all-in amount paid for that ticket, including any upgrade difference and excluding fees for late check-in or upgrades. The cap MUST be enforced by the server.

#### Scenario: Price above face value
- **WHEN** a holder whose ticket cost 300 RON tries to list it for 320 RON through the API
- **THEN** the listing is refused with the maximum of 300 RON
Test: services/internal/resale/listing_test.go

#### Scenario: Early-bird ticket after a price rise
- **WHEN** a ticket bought at 200 RON is listed after the product's price rose to 350 RON
- **THEN** the maximum price is 200 RON
Test: services/internal/resale/listing_test.go

#### Scenario: Seller sees the payout before listing
- **WHEN** a holder enters a price of 280 RON with a resale fee of 15 RON
- **THEN** the form shows the buyer pays 280 RON and the seller receives 265 RON
Test: apps/web/tests/e2e/resale.spec.ts

### Requirement: Which tickets can be listed or transferred
A ticket SHALL be listed or transferred only if it is valid, held by the requester, fully paid, not already admitted at the door, not suspended and not already listed or in a pending transfer.

#### Scenario: Ticket on an unfinished instalment plan
- **WHEN** a holder with 3 of 10 instalments paid tries to list the ticket
- **THEN** the listing is refused with "Finish your instalments to resell this ticket"
Test: services/internal/resale/listing_test.go

#### Scenario: Ticket already scanned at the door
- **WHEN** an entry for a listed ticket reaches the server
- **THEN** the listing closes and is no longer offered
Test: services/internal/resale/listing_test.go

### Requirement: Seller's code works until the sale
A listed ticket SHALL keep working for its holder until it is sold, and the seller MUST be able to delist it until a buyer holds it.

#### Scenario: Delist before anyone buys
- **WHEN** the seller delists a listed ticket that no buyer holds
- **THEN** the listing is withdrawn and the ticket stays valid for the seller
Test: services/internal/resale/listing_test.go

#### Scenario: Delist while a buyer holds it
- **WHEN** the seller delists while a buyer's hold on the listing is active
- **THEN** the delist is refused until the hold ends
Test: services/internal/resale/listing_test.go

### Requirement: Buying a resale ticket reissues it
A buyer SHALL buy a listed ticket through the normal checkout at the listing price, all-in, with its face value shown. When the payment is confirmed, the ticket MUST be reissued to the buyer with a new code and cleared personalisation, and the old code MUST be revoked, in one transaction.

#### Scenario: Resale sale completes
- **WHEN** a buyer's payment for a listed ticket is confirmed
- **THEN** the ticket has a new generation held by the buyer, the buyer is asked to name it, the seller's wallet and ticket page show it as sold, and the revocation feed lists the old generation
Test: services/internal/resale/sale_test.go

#### Scenario: Old code at the door
- **WHEN** a door device that has synced since the sale scans the seller's old code
- **THEN** it shows "This ticket was transferred or replaced", and the buyer's new code is accepted
Test: apps/web/tests/e2e/resale.spec.ts

#### Scenario: Payment for a resale ticket fails
- **WHEN** the buyer's payment is declined or the hold expires
- **THEN** the listing returns to listed and the ticket stays with the seller unchanged
Test: services/internal/resale/sale_test.go

#### Scenario: Two buyers pick the same listing
- **WHEN** two buyers try to hold the same listing at once
- **THEN** one gets the hold and the other is told it is no longer available
Test: services/internal/resale/sale_test.go

### Requirement: Seller is paid out to the EPS Wallet
On a resale sale the seller SHALL be credited the listing price minus the published resale fee to their EPS Wallet (simulated payout), recorded in the ledger in the same transaction as the reissue; the ledger MUST balance.

#### Scenario: Payout recorded
- **WHEN** a ticket listed at 280 RON with a 15 RON fee is sold
- **THEN** the ledger shows 280 RON in from the buyer, 265 RON credited to the seller's wallet and 15 RON to the fee account, and the entries sum to zero
Test: services/internal/resale/payout_test.go

#### Scenario: Reissue fails after payment
- **WHEN** the reissue cannot be committed after the buyer's payment is confirmed
- **THEN** no payout is posted, the payment is refunded (simulated), and the listing returns to listed
Test: services/internal/resale/payout_test.go

### Requirement: Transfer a ticket to a friend
The holder SHALL be able to send a ticket for free to a friend by email or link. The transfer MUST stay pending until the friend accepts and the sender MUST be able to cancel it until then. On acceptance the ticket MUST be reissued to the friend with a new code and cleared personalisation.

#### Scenario: Friend accepts
- **WHEN** the friend opens the transfer link and accepts, signed in or as a guest
- **THEN** the ticket is reissued to the friend, leaves the sender's wallet, and the sender's old code is revoked
Test: services/internal/resale/transfer_test.go

#### Scenario: Sender cancels
- **WHEN** the sender cancels a pending transfer
- **THEN** the link stops working and the ticket stays with the sender unchanged
Test: services/internal/resale/transfer_test.go

#### Scenario: Transfer link opened twice
- **WHEN** a transfer link that was already accepted is opened again
- **THEN** the page says the ticket has already been accepted
Test: services/internal/resale/transfer_test.go

### Requirement: Resale tickets on the storefront
The storefront SHALL show, for each sold-out product with listings, the resale tickets with their price and face value, labelled as official face-value resale, in Romanian and English.

#### Scenario: Sold-out product with listings
- **WHEN** a fan opens a sold-out product that has 3 listings
- **THEN** a resale section shows the 3 tickets with "Face value" next to each price
Test: apps/web/tests/e2e/resale.spec.ts
