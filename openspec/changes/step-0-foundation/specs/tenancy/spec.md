# Spec Delta

## Purpose

Keeps every organiser's data apart at every layer: each record belongs to one organiser, the database itself refuses access across organisers, cache keys are scoped, and APIs never reveal that another organiser's records exist.

## ADDED Requirements

### Requirement: Every organiser record carries its organiser
Every table in the control and sales schemas SHALL have a non-null organiser id and row-level security enabled and forced, except the tables on a reviewed platform allowlist (users, sessions, keys, the organiser registry).

#### Scenario: New table without organiser id
- **WHEN** a migration adds a table that has no organiser id and is not on the allowlist
- **THEN** the schema check test fails and names the table
Test: services/internal/tenancy/schema_test.go

#### Scenario: Table without forced row-level security
- **WHEN** an organiser table has row-level security enabled but not forced
- **THEN** the schema check test fails and names the table
Test: services/internal/tenancy/schema_test.go

### Requirement: The database isolates organisers
Within a transaction scoped to one organiser, reads SHALL return only that organiser's rows and writes for any other organiser MUST be refused by the database, whatever filter the query uses.

#### Scenario: Query without a filter
- **WHEN** code scoped to organiser A runs `select * from` an organiser table holding rows of A and B
- **THEN** only A's rows are returned
Test: services/internal/tenancy/rls_test.go

#### Scenario: Insert for another organiser
- **WHEN** code scoped to organiser A inserts a row with organiser B's id
- **THEN** the database rejects the insert
Test: services/internal/tenancy/rls_test.go

#### Scenario: Update moving a row to another organiser
- **WHEN** code scoped to organiser A updates one of its rows to organiser B's id
- **THEN** the database rejects the update
Test: services/internal/tenancy/rls_test.go

### Requirement: No organiser scope means no data
A transaction that has not set an organiser scope SHALL see no organiser rows and MUST NOT be able to write them.

#### Scenario: Scope forgotten
- **WHEN** application code queries an organiser table without setting the scope
- **THEN** the result is empty and an insert fails
Test: services/internal/tenancy/rls_test.go

### Requirement: Application connections cannot bypass isolation
Both the web app and the Go core SHALL run every application query under a database role that cannot bypass row-level security and does not own the tables.

#### Scenario: Current role in a transaction
- **WHEN** either side opens an application transaction
- **THEN** the effective role is the side's application role, which has no bypass privilege and owns no table
Test: services/internal/tenancy/rls_test.go

### Requirement: Each side writes only its own schema
The web app's role SHALL be able to write only the control schema and the Go core's role only the sales schema; each MAY read only the other side's tables it has been granted.

#### Scenario: Core writes control
- **WHEN** the Go core's role tries to insert into a control table
- **THEN** the database refuses with a permission error
Test: services/internal/tenancy/rls_test.go

#### Scenario: Web writes sales
- **WHEN** the web app's role tries to update a sales table
- **THEN** the database refuses with a permission error
Test: services/internal/tenancy/rls_test.go

### Requirement: Redis keys are scoped by organiser and event
Every Redis key holding organiser data SHALL start with the platform prefix, the organiser id and the event id (or a placeholder for organiser-wide keys), and platform keys MUST use a separate platform prefix.

#### Scenario: Key for event stock
- **WHEN** the core builds a key for event E of organiser A
- **THEN** the key starts with `eps:{A}:E:`
Test: services/internal/redisx/keys_test.go

#### Scenario: Missing organiser
- **WHEN** code asks for an organiser key with an empty organiser id
- **THEN** the key builder returns an error instead of a key
Test: services/internal/redisx/keys_test.go

### Requirement: APIs never reveal other organisers' records
Organiser API calls SHALL act only within the organiser named in the caller's token, and a request for another organiser's record MUST get the same not-found answer as a record that does not exist.

#### Scenario: Guessing another organiser's id
- **WHEN** an organiser of A requests a record id that belongs to B
- **THEN** the response is 404, identical to that for an unknown id
Test: services/internal/tenancy/api_scope_test.go

#### Scenario: Token without organiser
- **WHEN** a fan or guest token calls an organiser API
- **THEN** the response is 403 and no organiser scope is set
Test: services/internal/tenancy/api_scope_test.go
