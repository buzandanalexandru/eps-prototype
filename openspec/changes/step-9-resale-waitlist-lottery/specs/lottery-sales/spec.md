# Spec Delta

## Purpose

Lets an organiser sell high-demand tickets by pre-registration and a verifiable random draw instead of a race, with single-use access codes for the winners.

## ADDED Requirements

### Requirement: Organiser creates a lottery sale moment
An organiser SHALL be able to create a lottery sale moment for a product with a registration window, the number of tickets to allocate and a maximum per registrant. When registration opens, the commitment to the draw's random seed MUST be shown on the lottery page.

#### Scenario: Create a lottery
- **WHEN** the organiser creates a lottery for 1,000 GA passes, maximum 2 per person, with registration open for 7 days
- **THEN** the lottery page shows the window, says registration does not guarantee a ticket, and shows the seed commitment
Test: manual check on the deployed preview (create a lottery in EPS Studio and open its page)

### Requirement: One registration per person
A signed-in fan SHALL be able to register once per lottery, for up to the maximum per registrant and within their cap per person, only during the registration window.

#### Scenario: Register twice
- **WHEN** a fan who has registered tries to register again
- **THEN** the second registration is refused and the first one is shown
Test: services/internal/lottery/register_test.go

#### Scenario: Window closed
- **WHEN** a fan tries to register after the window has closed
- **THEN** the registration is refused
Test: services/internal/lottery/register_test.go

### Requirement: Verifiable draw run once
After registration closes, the organiser SHALL run the draw once. The order MUST come from the committed seed applied to each registration, with registrations flagged by bot scoring ranked after all others, and the seed and ordered list MUST be published so anyone can recompute the result. Running the draw again MUST return the same result.

#### Scenario: Draw allocates winners
- **WHEN** 5,000 fans registered for 1,000 passes and the organiser presses Draw
- **THEN** winners are taken in rank order until 1,000 passes are allocated, and the seed, which matches the commitment, is published with the order
Test: services/internal/lottery/draw_test.go

#### Scenario: Recompute the draw
- **WHEN** the published seed and registration IDs are run through the published ranking
- **THEN** the same order of winners results
Test: services/internal/lottery/draw_test.go

#### Scenario: Draw pressed twice
- **WHEN** the organiser presses Draw a second time
- **THEN** the stored result is shown and nothing is redrawn
Test: services/internal/lottery/draw_test.go

### Requirement: Winners buy with a single-use access code
Each winner SHALL get a single-use access code bound to their account, for their quantity, valid for a purchase window, which unlocks the product in the normal checkout. Fans who did not win MUST be told so.

#### Scenario: Winner buys
- **WHEN** a winner enters their code at checkout within the window
- **THEN** the product is unlocked for their quantity and the code cannot be used again
Test: services/internal/lottery/codes_test.go

#### Scenario: Code used by another account
- **WHEN** someone else signs in and enters a winner's code
- **THEN** the code is refused
Test: services/internal/lottery/codes_test.go
