# Spec Delta

## Purpose

Lets one fan hold tickets for a whole group while each friend pays their own share, or first instalment, from a personal link, with unpaid shares released at the deadline or covered by the leader, and each friend holding and naming their own ticket.

## ADDED Requirements

### Requirement: Organiser controls squad buy per sale
Squad buy SHALL be available on a sale only when the organiser has switched it on for that sale, with a squad window the organiser sets (default 30 minutes when the sale uses the waiting room, at most 48 hours). Both values MUST be part of the published sale snapshot.

#### Scenario: Squad buy switched off
- **WHEN** a fan views a product on a sale with squad buy off
- **THEN** no "Buy as a squad" option is shown and the squad API refuses to create a squad for that sale
Test: services/internal/squad/create_test.go

#### Scenario: Window comes from the snapshot
- **WHEN** the organiser changes the squad window in a draft after publishing
- **THEN** squads on the live sale keep the published window until the next publish
Test: services/internal/squad/create_test.go

### Requirement: Leader takes one group hold
A fan who may check out SHALL be able to start a squad of N tickets of one product, where N is at least 2 and at most the sale's maximum per order and the leader's remaining cap. Creating the squad MUST hold all N units at once or none, and MUST require a valid queue pass when the sale uses the waiting room.

#### Scenario: Squad of four
- **WHEN** an admitted fan starts a squad of 4 GA passes
- **THEN** 4 units are held together, the squad is forming with 4 seats, seat 1 belongs to the leader, and the stock left drops by 4
Test: services/internal/squad/create_test.go

#### Scenario: Not enough stock
- **WHEN** a fan starts a squad of 4 and only 3 units are left
- **THEN** no units are held and the fan is told only 3 are left, with the option to start a squad of 3
Test: services/internal/squad/create_test.go

#### Scenario: Over the leader's cap
- **WHEN** a fan with a cap of 4 who already holds 2 tickets starts a squad of 4
- **THEN** the squad is refused with the number they can still buy
Test: services/internal/squad/create_test.go

#### Scenario: No queue pass
- **WHEN** a fan without a valid queue pass starts a squad on a sale with the waiting room on
- **THEN** the request is refused like any checkout without a pass
Test: services/internal/squad/create_test.go

### Requirement: Squad starts when the leader pays their share
A new squad SHALL stay forming with the normal checkout hold time until the leader's own share is paid; then it MUST become active and its hold MUST last until the squad deadline. A forming squad whose leader does not pay MUST expire with all its units released.

#### Scenario: Leader pays first share
- **WHEN** the leader's payment for seat 1 is confirmed
- **THEN** the squad becomes active, its deadline is now plus the squad window, and the leader's ticket is issued
Test: services/internal/squad/lifecycle_test.go

#### Scenario: Leader never pays
- **WHEN** the leader's checkout hold expires without payment
- **THEN** the squad expires, all N units return to stock and no seat link works
Test: services/internal/squad/lifecycle_test.go

### Requirement: Each seat has its own link
Each seat other than the leader's SHALL have a unique, unguessable link that opens that seat only. The leader MUST be able to copy or share each link and to replace the link of an unpaid seat, which makes the old link stop working.

#### Scenario: Friend opens a seat link
- **WHEN** a friend opens the link for seat 3 without an account
- **THEN** they see the event, the leader's display name, the all-in share price, the deadline countdown and how many seats are paid, with a Pay my share action
Test: apps/web/tests/e2e/squad-buy.spec.ts

#### Scenario: Replaced link
- **WHEN** the leader replaces the link of unpaid seat 3 and someone opens the old link
- **THEN** the page says the link is no longer valid
Test: services/internal/squad/seats_test.go

#### Scenario: Guessed link
- **WHEN** someone requests a seat with a random token
- **THEN** the request fails with not found, and repeated failures from one client are rate limited
Test: services/internal/squad/seats_test.go

### Requirement: Each friend pays their own share
A friend SHALL pay their seat's share, or the first instalment of the product's instalment plan, through the normal payment flow (fake providers with failover, Apple/Google Pay, saved card or EPS Wallet), as a separate order for that seat. Their own cap MUST be checked before payment.

#### Scenario: Friend pays in full
- **WHEN** a friend pays the share for seat 2 and the payment is confirmed
- **THEN** seat 2 is paid, a ticket is issued to that friend (account or guest), and everyone's squad page shows 2 of 4 paid
Test: apps/web/tests/e2e/squad-buy.spec.ts

#### Scenario: Friend chooses instalments
- **WHEN** a friend picks the instalment plan and the first instalment is confirmed
- **THEN** seat 2 is paid, the ticket is issued, and the friend's account shows the remaining instalments
Test: services/internal/squad/pay_test.go

#### Scenario: Payment declined
- **WHEN** a friend's payment is declined
- **THEN** the seat goes back to open and the friend can try again with another method before the deadline
Test: services/internal/squad/pay_test.go

#### Scenario: Friend over their own cap
- **WHEN** a friend who already holds their cap of tickets for the event tries to pay a seat
- **THEN** payment is refused before any charge, and the seat stays open
Test: services/internal/squad/pay_test.go

### Requirement: One payer per seat and no double charge
A seat SHALL be paid at most once. While someone is paying for a seat, a second payer MUST be refused before any charge, and retries of the same attempt MUST NOT charge twice.

#### Scenario: Two friends open the same link and pay at once
- **WHEN** two people press Pay my share on seat 3 at the same moment
- **THEN** one payment proceeds, the other is refused with "Someone is paying for this spot" and is never charged
Test: services/internal/squad/pay_test.go

#### Scenario: Double click and retry
- **WHEN** a friend double-clicks Pay or their phone retries the request
- **THEN** exactly one charge is made for the seat
Test: services/internal/squad/pay_test.go

### Requirement: Share price is locked at hold time
Every seat SHALL pay the all-in unit price of the snapshot the group hold was taken under, even if the price ladder steps up before the friend pays.

#### Scenario: Price steps up during the squad
- **WHEN** the price ladder moves GA from 300 to 350 RON after the squad was created at 300 RON
- **THEN** friends still pay 300 RON for their seats
Test: services/internal/squad/pay_test.go

### Requirement: Unpaid seats are released at the deadline
At the squad deadline the system SHALL release every unpaid seat back to stock and close the squad. A payment already in progress MUST get up to 2 minutes to resolve. A payment confirmed for a seat after it was released MUST be refunded automatically and MUST NOT issue a ticket.

#### Scenario: Two of four paid at the deadline
- **WHEN** the deadline passes with seats 1 and 2 paid and seats 3 and 4 open
- **THEN** 2 units return to stock, the squad closes, the paid tickets stay valid, and links 3 and 4 say the spot has expired
Test: services/internal/squad/expiry_test.go

#### Scenario: Payment in flight at the deadline
- **WHEN** a friend's payment for seat 3 is still pending at the deadline and is confirmed 40 seconds later
- **THEN** seat 3 is paid and its ticket issued
Test: services/internal/squad/expiry_test.go

#### Scenario: Late confirmation after release
- **WHEN** a confirmation arrives for a seat that was already released
- **THEN** the payment is refunded (simulated), no ticket is issued, and the friend's page says the spot expired and the money was returned
Test: services/internal/squad/expiry_test.go

#### Scenario: Worker down at the deadline
- **WHEN** the worker misses a squad's deadline
- **THEN** the backstop sweep closes the squad and releases its unpaid seats within a minute of the worker returning, and no unit is double-released
Test: services/internal/squad/expiry_test.go

### Requirement: Leader can cover unpaid seats
Before the deadline the leader SHALL be able to pay for any unpaid seats in one order. Covered seats MUST issue tickets held by the leader, and each covered seat's link MUST become a claim link that lets the friend take that ticket for free.

#### Scenario: Leader covers the last seat
- **WHEN** the leader covers seat 4 and the payment is confirmed
- **THEN** seat 4 is covered, the leader holds its ticket, and the squad is complete
Test: services/internal/squad/cover_test.go

#### Scenario: Friend claims a covered seat
- **WHEN** the friend opens seat 4's link after it was covered and claims it
- **THEN** the ticket is reissued to the friend at a new generation, leaves the leader's wallet, and the friend can name it
Test: services/internal/squad/cover_test.go

#### Scenario: Friend pays while the leader covers
- **WHEN** the friend's payment for seat 4 and the leader's cover order run at the same time
- **THEN** only one is charged for seat 4 and the other is refused before any charge
Test: services/internal/squad/cover_test.go

### Requirement: Leader can release unpaid seats early
The leader SHALL be able to release unpaid seats before the deadline; released units MUST return to stock at once and their links MUST stop working.

#### Scenario: One friend drops out
- **WHEN** the leader releases seat 3
- **THEN** one unit returns to stock and seat 3's link says the spot was released
Test: services/internal/squad/seats_test.go

### Requirement: Each friend holds and names their own ticket
Each paid seat's ticket SHALL be held by the person who paid it and appear in their trip wallet, and personalisation MUST be done by that holder. The leader MUST NOT see a friend's email, payment method or personal details.

#### Scenario: Friend names their ticket
- **WHEN** a friend who paid seat 2 opens their trip wallet
- **THEN** their ticket is there with "Name this ticket", and the leader's wallet does not contain it
Test: apps/web/tests/e2e/squad-buy.spec.ts

#### Scenario: Leader's view of the squad
- **WHEN** the leader opens the squad page
- **THEN** each seat shows only its state and the display name the friend chose
Test: services/internal/squad/status_test.go

### Requirement: Live squad page
The squad page SHALL show, for the leader and for friends, the seats paid out of N, the time left and the squad's state (forming, active, complete, closed), updated within 5 seconds of a change, in Romanian and English.

#### Scenario: Leader watches friends pay
- **WHEN** a friend pays on another phone while the leader has the squad page open
- **THEN** the leader's page shows the new paid count within 5 seconds without a reload
Test: apps/web/tests/e2e/squad-buy.spec.ts

#### Scenario: Squad page in Romanian
- **WHEN** a friend opens a seat link on a Romanian storefront
- **THEN** all labels, the countdown and the price are in Romanian with diacritics
Test: apps/web/tests/e2e/squad-buy.spec.ts

### Requirement: Squad events on the stream
The system SHALL write `squad.created`, `squad.seat_paid`, `squad.completed` and `squad.expired` to the outbox with the organiser, event, squad and seat counts, so dashboards and webhooks can follow squads.

#### Scenario: Squad completes
- **WHEN** the last unpaid seat of a squad is paid
- **THEN** one `squad.seat_paid` and one `squad.completed` event are in the outbox
Test: services/internal/squad/lifecycle_test.go
