# identity Specification

## Purpose
Lets organisers and fans sign in once to the control plane, lets guests buy without an account, and gives the Go data plane short-lived tokens it can verify on its own, without calling back to the control plane.

## Requirements

### Requirement: Email magic-link sign-in
A person SHALL be able to sign in by entering an email address and opening a one-time link. The link MUST expire after 15 minutes and MUST work only once.

#### Scenario: First sign-in creates the user
- **WHEN** a new address requests a link and opens it within 15 minutes
- **THEN** a user is created, a session starts and the person lands on the page they came from
Test: apps/web/tests/unit/auth-magic-link.test.ts

#### Scenario: Link reused or expired
- **WHEN** a link is opened a second time, or after 15 minutes
- **THEN** no session starts and the page offers to send a new link
Test: apps/web/tests/unit/auth-magic-link.test.ts

#### Scenario: Demo identity cannot be claimed from outside
- **WHEN** someone signs in with Google using an address under the demo domain
- **THEN** no user is created and no account is linked to an existing demo user
Test: apps/web/tests/unit/auth-config.test.ts

#### Scenario: Demo mailbox
- **WHEN** no mail provider is configured and a demo-domain address requests a link
- **THEN** the link is shown in the on-screen demo mailbox and not sent anywhere
Test: apps/web/tests/e2e/sign-in.spec.ts

### Requirement: Google sign-in
When Google credentials are configured, the sign-in page SHALL offer Google; signing in with a Google address that already has a user MUST attach to that user rather than create a second one.

#### Scenario: Not configured
- **WHEN** Google credentials are absent
- **THEN** the sign-in page shows no Google button
Test: apps/web/tests/unit/auth-config.test.ts

#### Scenario: Existing email
- **WHEN** a person who signed in by magic link later uses Google with the same verified address
- **THEN** both methods lead to the same user
Test: apps/web/tests/unit/auth-config.test.ts

### Requirement: Demo sign-in path
When demo sign-in is switched on, the sign-in page SHALL offer "Continue as demo organiser" and "Continue as demo fan". Each demo fan click MUST create a fresh fan user. Demo sign-in MUST be off unless configured and rate-limited per address.

#### Scenario: Demo organiser
- **WHEN** a visitor chooses "Continue as demo organiser" on a preview
- **THEN** a session starts for the shared demo organiser user and Studio opens
Test: apps/web/tests/e2e/sign-in.spec.ts

#### Scenario: Two demo fans
- **WHEN** two browsers each choose "Continue as demo fan"
- **THEN** they are signed in as two different users
Test: apps/web/tests/e2e/sign-in.spec.ts

#### Scenario: Switched off
- **WHEN** demo sign-in is not configured
- **THEN** the buttons are absent and the demo sign-in endpoint returns 404
Test: apps/web/tests/unit/auth-config.test.ts

### Requirement: Studio requires sign-in, storefronts do not
Organiser screens SHALL require a signed-in user and redirect others to sign-in, returning them afterwards. Public storefront and status routes MUST work without sign-in.

#### Scenario: Anonymous visitor opens Studio
- **WHEN** a visitor without a session opens `/studio`
- **THEN** they are redirected to `/sign-in` and, after signing in, back to `/studio`
Test: apps/web/tests/e2e/sign-in.spec.ts

### Requirement: Sign-out
Signing out SHALL end the session at once, and tokens already issued MUST stop being renewable.

#### Scenario: Sign out
- **WHEN** a signed-in user signs out
- **THEN** the next Studio request redirects to sign-in and a request for a new core token is refused
Test: apps/web/tests/unit/auth-session.test.ts

### Requirement: Short-lived tokens for the Go core
The control plane SHALL issue signed tokens for the Go core that last at most 5 minutes and carry an issuer naming the environment, the subject, its kind (organiser, fan or guest), the active organiser and role for organisers, the session id and an audience reserved for the core.

#### Scenario: Organiser token
- **WHEN** a signed-in organiser's request needs the core
- **THEN** the token carries `kind=organiser`, the active organiser id, the role, audience `eps-core` and an expiry no more than 5 minutes ahead
Test: apps/web/tests/unit/core-token.test.ts

#### Scenario: Fan token has no organiser
- **WHEN** a fan's token is issued
- **THEN** it has `kind=fan` and no organiser claim
Test: apps/web/tests/unit/core-token.test.ts

### Requirement: Public verification keys with rotation
The control plane SHALL publish its token verification keys as a JWKS, sign each token with a key identified in its header, and keep a retired key published for 7 days after rotation.

#### Scenario: Rotation
- **WHEN** the signing key rotates
- **THEN** new tokens use the new key id and tokens signed with the previous key still verify until they expire
Test: apps/web/tests/unit/core-token.test.ts

### Requirement: The Go core verifies tokens on its own
The Go core SHALL accept a request needing identity only with a token whose signature, issuer (the one configured for its environment), audience, expiry (30 s leeway), kind and role are valid, MUST answer 401 otherwise, and MUST reload keys for unknown key ids at most once every 15 seconds, separately from its periodic key refresh. A failed key load MUST NOT stop verification once the database answers again.

#### Scenario: Valid token
- **WHEN** `/v1/whoami` is called with a fresh organiser token
- **THEN** it returns 200 with the subject, kind and organiser
Test: services/internal/auth/verify_test.go

#### Scenario: Expired, tampered, wrong audience or issuer
- **WHEN** the token expired more than 30 s ago, its payload was altered, its audience is not `eps-core`, its issuer belongs to another environment, or its role is not an organiser role
- **THEN** the core returns 401 with a problem+json body and no identity details
Test: services/internal/auth/verify_test.go

#### Scenario: Flood of unknown key ids
- **WHEN** many requests arrive with tokens naming an unknown key id
- **THEN** the core reloads keys at most once per 15 seconds and rejects them all
Test: services/internal/auth/verify_test.go

#### Scenario: Key database briefly down
- **WHEN** the first key load fails and the database then recovers
- **THEN** the next request after a short back-off loads the keys and verifies
Test: services/internal/auth/verify_test.go

### Requirement: Guest buyers
A visitor without an account SHALL be able to get a guest identity, kept in a signed http-only cookie, and a guest token for the core. A guest token MUST NOT grant organiser access.

#### Scenario: Guest identity
- **WHEN** a visitor with no session asks for a core token from a storefront
- **THEN** a guest cookie is set and the token has `kind=guest` and subject `guest:<id>`
Test: apps/web/tests/unit/core-token.test.ts

#### Scenario: Guest calls an organiser endpoint
- **WHEN** a guest token is used on an organiser-only core endpoint
- **THEN** the core returns 403
Test: services/internal/auth/verify_test.go
