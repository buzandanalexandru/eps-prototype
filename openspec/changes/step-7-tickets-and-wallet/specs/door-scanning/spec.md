# Spec Delta

## Purpose

Lets an organiser's door staff scan rotating ticket codes on a phone at the gate, verify them offline with the published public key, reject screenshots, old and reissued codes, and record each entry once.

## ADDED Requirements

### Requirement: Door page for the organiser's staff only
The door page SHALL be usable only by the owner, admins and box office staff of the event's organiser. Preparing a device MUST download, while online, the public ticket keys, the event's sync package (revocations, entries, holder names) and the server time.

#### Scenario: Box office prepares a device
- **WHEN** a box office member of the organiser opens the door page for their event and presses Prepare this device
- **THEN** the device stores the keys, the sync package and the clock offset, and shows Ready to scan
Test: apps/web/tests/e2e/door-scanning.spec.ts

#### Scenario: Staff of another organiser
- **WHEN** a signed-in member of another organiser opens the door page for this event
- **THEN** access is refused and no event data is downloaded
Test: services/internal/door/access_test.go

### Requirement: Codes are verified offline
A prepared door device SHALL verify each code with no network: the signature with the published key for its key ID, the event, the generation against known revocations, and the time window against its corrected clock. It MUST accept only the current window and one window either side.

#### Scenario: Valid live code in airplane mode
- **WHEN** a prepared device with no network scans the live code on a holder's phone
- **THEN** it shows a full-screen green Valid with the product and the holder's name if the ticket is named
Test: apps/web/tests/e2e/door-scanning.spec.ts

#### Scenario: Screenshot from a minute ago
- **WHEN** the device scans a code whose window is two or more windows before its current window
- **THEN** it shows red "Code expired: this looks like a screenshot or an old code"
Test: apps/web/tests/unit/door-verify.test.ts

#### Scenario: Phone clock slightly fast
- **WHEN** the device scans a code from the next window
- **THEN** it is accepted
Test: apps/web/tests/unit/door-verify.test.ts

#### Scenario: Code from further in the future
- **WHEN** the device scans a code two or more windows ahead of its current window
- **THEN** it is rejected as not yet valid
Test: apps/web/tests/unit/door-verify.test.ts

#### Scenario: Ticket for another event
- **WHEN** the device scans a valid code for a different event
- **THEN** it shows red "Ticket for another event"
Test: apps/web/tests/unit/door-verify.test.ts

#### Scenario: Forged or altered code
- **WHEN** the device scans a code with a bad signature or an unknown key ID
- **THEN** it shows red "Not a valid ticket"
Test: apps/web/tests/unit/door-verify.test.ts

#### Scenario: Reissued ticket
- **WHEN** the device has synced a revocation for generation 1 of a ticket and scans a generation 1 code
- **THEN** it shows red "This ticket was transferred or replaced"
Test: apps/web/tests/unit/door-verify.test.ts

#### Scenario: Suspended ticket
- **WHEN** the device has synced a suspension for a ticket and scans its code
- **THEN** it shows red "Ticket suspended: send the holder to the help desk"
Test: apps/web/tests/unit/door-verify.test.ts

### Requirement: One entry per ticket
A device SHALL admit each ticket once and reject a later scan of it with the time and gate of the first entry. When entries from several devices meet at sync, the server MUST keep the first by device time and record any duplicate as a conflict shown to the organiser.

#### Scenario: Same ticket twice on one device
- **WHEN** a device scans a valid code for a ticket it has already admitted
- **THEN** it shows red "Already scanned at 21:04 at Gate B"
Test: apps/web/tests/unit/door-verify.test.ts

#### Scenario: Two offline devices admit the same ticket
- **WHEN** two devices admit the same ticket while offline and then both sync
- **THEN** the server records one entry and one conflict, and both devices show the conflict in their log
Test: services/internal/door/sync_test.go

### Requirement: Works offline and syncs when online
The door page SHALL load and scan with no network after the device is prepared. Scans MUST be stored on the device first and uploaded idempotently when online; while online the device MUST fetch new revocations and other devices' entries at least every 30 seconds and show the time since its last sync, with a warning after 10 minutes.

#### Scenario: Reload with no network
- **WHEN** a prepared device reloads the door page in airplane mode
- **THEN** the page opens from cache and scans still verify
Test: apps/web/tests/e2e/door-scanning.spec.ts

#### Scenario: Scans uploaded after reconnect
- **WHEN** a device that scanned 20 tickets offline gets a network again
- **THEN** all 20 entries reach the server exactly once, even if the upload is retried
Test: services/internal/door/sync_test.go

#### Scenario: Stale sync warning
- **WHEN** a device has not synced for more than 10 minutes
- **THEN** the header turns amber and says how long ago it last synced
Test: apps/web/tests/unit/door-verify.test.ts

### Requirement: Clock correction
The door device SHALL measure the offset between its clock and the server's at every sync and verify windows with the corrected time. A device that has never measured its offset MUST show a warning.

#### Scenario: Door phone clock is two minutes slow
- **WHEN** a device whose clock is two minutes slow has synced and scans a live code
- **THEN** the code is accepted
Test: apps/web/tests/unit/door-verify.test.ts

### Requirement: Clear scan feedback in the staff's language
Each scan SHALL give a full-screen green or red result with a short reason in Romanian or English, a sound and a vibration where the device supports it, and return to scanning within two seconds.

#### Scenario: Rejected scan in Romanian
- **WHEN** a device set to Romanian rejects a screenshot
- **THEN** it shows a red result with the reason in Romanian with diacritics, then returns to the camera
Test: apps/web/tests/e2e/door-scanning.spec.ts
