# bot-defence Specification

## Purpose
Keeps bots and bulk buyers from crowding out real fans during an on-sale, by scoring suspicious behaviour silently, enforcing caps per person across sale moments, and asking only risky or bulk buyers for a one-time code before payment.

## Requirements

### Requirement: Silent bot scoring
When bot defence is on, the system SHALL score each queue place from 0 to 100 using behaviour signals (missing proof of work, polling too fast or too regularly, many places per address or address block, hosting addresses, automation markers, shared browser prints, checkout with another session's pass, all codes of an OTP used). Scores SHALL be shown only to the organiser.

#### Scenario: Refresh-spamming bot
- **WHEN** a client without proof of work polls status every 200 milliseconds
- **THEN** its score reaches 70 or more
- Test: services/internal/botdefence/score_test.go

#### Scenario: Ordinary fan
- **WHEN** a fan passes proof of work, polls every 10 seconds with jitter and reloads twice
- **THEN** its score stays below 40
- Test: services/internal/botdefence/score_test.go

### Requirement: Shared addresses are not suspects
Many places from one address or address block SHALL on their own make a place at most risky, never a suspect, and a browser print SHALL count only when more than 20 people share it.

#### Scenario: Shared addresses alone
- **WHEN** many places come from one address and its /24
- **THEN** the score stays below the suspect threshold unless another signal adds to it
- Test: services/internal/botdefence/score_test.go

### Requirement: Suspects go to the back after the shuffle
Places scoring 70 or more SHALL be moved behind every other place, and places scoring 90 or more admitted only when no one else waits. This SHALL happen only after the shuffle, and suspects SHALL see a normal waiting room with no error or block message.

#### Scenario: Moved back silently
- **WHEN** a place scored 75 is at position 300 after the shuffle
- **THEN** it is moved behind all other places and its page still shows a normal, slowly moving position
- Test: services/internal/botdefence/penalty_test.go

#### Scenario: Nothing visible before opening
- **WHEN** a suspect is in the pre-queue before opening
- **THEN** its experience is identical to any other pre-queue member
- Test: services/internal/botdefence/penalty_test.go

### Requirement: Caps per person across sale moments
The system SHALL limit tickets per person per event across all its sale moments, counting held and paid tickets, by account (and its verified email) or by normalised guest email, including tickets paid before the event had sale moments. A hold over the cap SHALL be refused with `EPS-CAP-REACHED` and the number still allowed; the cap SHALL be checked again when payment succeeds.

#### Scenario: Cap across presale and general sale
- **WHEN** a fan with a cap of 6 bought 4 in the presale and tries to hold 3 in the general sale
- **THEN** the hold is refused and the message says 2 more can be bought
- Test: services/internal/botdefence/caps_test.go

#### Scenario: Guest email variants
- **WHEN** guests use `ana.pop+1@gmail.com` and `anapop@gmail.com`
- **THEN** both count toward the same cap
- Test: services/internal/botdefence/caps_test.go

#### Scenario: Bought before the sale moment
- **WHEN** a fan paid 6 tickets (cap 6) before the event had sale moments and then holds 1 in a sale moment
- **THEN** the hold is refused
- Test: services/internal/botdefence/caps_test.go

#### Scenario: Account email and guest share a cap
- **WHEN** an account with a verified email bought 4 (cap 6) and a guest using that email asks for 3
- **THEN** the guest is refused and told 2 more can be bought
- Test: services/internal/botdefence/caps_test.go

#### Scenario: Parallel holds at the cap
- **WHEN** two sessions of one account each try to hold 3 tickets at once with 4 left on the cap
- **THEN** exactly one hold succeeds
- Test: services/internal/botdefence/caps_test.go

### Requirement: Per-order cap
The checkout SHALL refuse an order above the sale's cap per order.

#### Scenario: Too many in one order
- **WHEN** a fan asks for 5 tickets with a cap of 4 per order
- **THEN** the order is refused with the cap shown
- Test: services/internal/botdefence/caps_test.go

### Requirement: Risk-based OTP before payment
When the organiser has switched OTP on, payment SHALL require a one-time code only from buyers scored 40 or more or whose order exceeds the ticket threshold; all other buyers SHALL go straight to payment. The code SHALL be simulated and shown on screen.

#### Scenario: Ordinary fan skips OTP
- **WHEN** a fan scored 10 buys 2 tickets with the threshold at 4
- **THEN** no code is asked
- Test: apps/web/tests/e2e/otp.spec.ts

#### Scenario: Bulk order asked
- **WHEN** a fan buys 6 tickets with the threshold at 4
- **THEN** the fan must enter the code shown in the simulated SMS panel before payment
- Test: apps/web/tests/e2e/otp.spec.ts

#### Scenario: OTP switched off
- **WHEN** the organiser has OTP off
- **THEN** no buyer is asked for a code whatever the score or quantity
- Test: services/internal/botdefence/otp_test.go

### Requirement: OTP limits
A code SHALL be 6 digits, valid 5 minutes, allow 3 wrong tries and up to 3 resends. When all tries are used, the order SHALL be cancelled and its stock released.

#### Scenario: Wrong code three times
- **WHEN** a buyer enters a wrong code three times and has used all resends
- **THEN** the order is cancelled, the stock is free again and the buyer's score rises
- Test: services/internal/botdefence/otp_test.go

#### Scenario: Expired code
- **WHEN** a correct code is entered after 5 minutes
- **THEN** it is refused and a resend is offered
- Test: services/internal/botdefence/otp_test.go

### Requirement: Real fans are not blocked
Bot defence SHALL never show real fans a block or error for behaviour alone; the strongest measures for a fan who is not a suspect SHALL be an OTP or a rate-limit wait.

#### Scenario: Simulated crowd
- **WHEN** a test crowd of 10,000 human-like fans and 1,000 bots goes through the queue
- **AND** the crowd includes carrier-grade NAT, a popular phone model and fans with two tabs
- **THEN** fewer than 1% of the fans are marked suspect and more than 90% of the bots are
- Test: services/internal/botdefence/crowd_test.go
