# Spec Delta

## Purpose

Lets any event organiser sign up on their own, hold several brands under one account, invite a team with roles, and choose how the published platform fee is shared, while fans always see one all-in price.

## ADDED Requirements

### Requirement: Self-serve organiser sign-up
A signed-in user without an organisation SHALL be able to create one by giving its name and address slug, and MUST become its owner. Onboarding SHALL then ask for the first brand and a starting template and land the user in Studio.

#### Scenario: Complete onboarding
- **WHEN** a new user enters organisation "Sunwave Events", slug `sunwave-events`, brand "Sunwave Festival" with slug `sunwave` and picks the festival template
- **THEN** the organisation and brand exist, the user is owner, the template choice is recorded on the brand, and Studio opens on the brand
Test: apps/web/tests/e2e/organiser-onboarding.spec.ts

#### Scenario: Interrupted onboarding resumes
- **WHEN** the user closes the browser after naming the organisation and signs in again
- **THEN** onboarding resumes at the brand step instead of creating a second organisation
Test: apps/web/tests/e2e/organiser-onboarding.spec.ts

### Requirement: Organiser and brand slugs are unique and safe
Organiser and brand slugs SHALL be 3 to 40 lowercase letters, digits or hyphens, unique across the whole platform and not on the reserved list. The availability check MUST reveal nothing about the organiser that holds a slug.

#### Scenario: Slug taken by another organiser
- **WHEN** an organiser tries brand slug `nibiru`, already used by another organiser
- **THEN** the form says the address is taken and shows nothing else about its holder
Test: apps/web/tests/unit/slugs.test.ts

#### Scenario: Reserved or malformed slug
- **WHEN** the slug is `studio`, `Ab`, or contains a space or diacritic
- **THEN** the form refuses it with the rule it breaks
Test: apps/web/tests/unit/slugs.test.ts

### Requirement: Several brands per organiser
An organiser SHALL be able to hold several brands, each with its own name, slug, languages, currency and logo slot, and MAY have hostnames added by platform configuration for host-based addresses.

#### Scenario: Second brand
- **WHEN** the owner of an organisation with brand "NIBIRU Festivals" adds brand "NIBIRU Resort"
- **THEN** both brands are listed under the organisation and each has its own address `/s/<slug>`
Test: apps/web/tests/e2e/brands.spec.ts

#### Scenario: Brand from another organiser
- **WHEN** a member of organiser A opens the settings URL of organiser B's brand
- **THEN** Studio shows not found
Test: apps/web/tests/e2e/brands.spec.ts

### Requirement: Active organiser
A user who belongs to several organisations SHALL choose one active organiser at a time; every Studio screen and core token MUST be scoped to it.

#### Scenario: Switching organiser
- **WHEN** a user who belongs to A and B switches from A to B
- **THEN** Studio shows only B's brands and events, and the next core token carries B
Test: apps/web/tests/unit/active-organiser.test.ts

### Requirement: Team roles screen
Studio SHALL show the organisation's members with role (owner, admin, box office, finance) and scope (organisation, a brand or an event), and let the owner invite by email (a pending invitation with a copyable link), change roles and remove members. Only owner versus non-owner MUST be enforced.

#### Scenario: Owner invites a box office member
- **WHEN** the owner invites `door@example.com` as box office for one event
- **THEN** a pending invitation with a link appears, and opening the link while signed in with that address adds the member
Test: apps/web/tests/e2e/team-roles.spec.ts

#### Scenario: Non-owner tries an owner action
- **WHEN** an admin tries to change the fee policy or the team
- **THEN** the action is refused with "Only the owner can do this"
Test: apps/web/tests/unit/roles.test.ts

#### Scenario: Last owner
- **WHEN** the only owner tries to demote or remove themselves
- **THEN** the change is refused
Test: apps/web/tests/unit/roles.test.ts

### Requirement: The platform fee is published
The platform fee (a percentage and a fixed amount per ticket) SHALL be shown on a public fees page, readable without sign-in, and Studio MUST use the same published value.

#### Scenario: Fees page
- **WHEN** anyone opens `/fees`
- **THEN** it shows the current fee (2.5% and no fixed part at launch), the three fee policies and a worked example, in Romanian and English
Test: apps/web/tests/e2e/fees-page.spec.ts

### Requirement: Fee policy switch
The owner SHALL choose per organiser whether it absorbs the platform fee, passes it to the fan, or splits it, and MAY override the choice per event. Changing the policy MUST affect only prices not yet published.

#### Scenario: Event override
- **WHEN** the organiser policy is "pass on" and one event is set to "absorb"
- **THEN** that event uses absorb and all other events use pass on
Test: apps/web/tests/unit/fee-policy.test.ts

#### Scenario: Studio shows both sides
- **WHEN** the organiser views a product priced at 539.00 RON under "pass on"
- **THEN** Studio shows "Fan pays 552.48 RON" and "You receive 539.00 RON"
Test: apps/web/tests/unit/fee-policy.test.ts

### Requirement: One all-in price calculation
The platform SHALL compute every fan-facing price per unit as the all-in total for the policy in force, in integer minor units with half-up rounding of the fee, and the fan's and organiser's amounts MUST always add up to the face price plus nothing more than the fee.

#### Scenario: Pass on
- **WHEN** the face price is 539.00 RON and the fee is 2.5%
- **THEN** the fee is 13.48 RON and the fan pays 552.48 RON
Test: apps/web/tests/unit/pricing.test.ts

#### Scenario: Split with an odd fee
- **WHEN** the policy is split and the fee is 0.25 RON
- **THEN** the fan pays the face price plus 0.12 RON and the organiser receives the face price minus 0.13 RON
Test: apps/web/tests/unit/pricing.test.ts

#### Scenario: Absorb
- **WHEN** the policy is absorb and the face price is 25.00 RON
- **THEN** the fan pays 25.00 RON and the organiser receives 24.37 RON
Test: apps/web/tests/unit/pricing.test.ts

#### Scenario: Free product
- **WHEN** the face price is 0
- **THEN** the fee is 0 under every policy
Test: apps/web/tests/unit/pricing.test.ts
