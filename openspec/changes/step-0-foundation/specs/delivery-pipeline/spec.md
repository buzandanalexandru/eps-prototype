# Spec Delta

## Purpose

Gets every change from a pull request to a public, working preview and then to production, with checks that block bad merges, a contract that keeps both halves in sync, migrations run only by CI, reproducible infrastructure and health endpoints that prove the full round trip.

## ADDED Requirements

### Requirement: Pull request checks gate merging
Every pull request to `main` SHALL run web lint, typecheck, unit tests and build, Go vet and tests, Helm lint and template rendering, and a migration dry run. A failing check MUST block the merge.

#### Scenario: A failing Go test blocks the merge
- **WHEN** a pull request contains a Go change whose test fails
- **THEN** the CI run reports a failed check and the pull request cannot be merged
Test: manual check on the deployed preview (the PR's checks list shows the failed `ci` job and the merge button is blocked)

#### Scenario: All checks pass
- **WHEN** a pull request passes every check
- **THEN** the CI run reports success for each job and the pull request is mergeable
Test: manual check on the deployed preview (all `ci` jobs green on the PR)

### Requirement: The API contract stays in sync
The OpenAPI document SHALL be the only source of the Next.js–Go contract. The generated TypeScript types and Go server stubs MUST be regenerated in CI and the run MUST fail when they differ from the committed files.

#### Scenario: Contract changed without regenerating
- **WHEN** a pull request edits `api/openapi.yaml` but not the generated files
- **THEN** the contract drift check fails and names the files that differ
Test: services/internal/api/contract_test.go

#### Scenario: Go server matches the contract
- **WHEN** the Go core starts
- **THEN** every operation in the contract has a registered handler, and a request that does not match the contract's schema is rejected with a problem+json error
Test: services/internal/api/contract_test.go

### Requirement: Each pull request gets its own preview environment
For every open pull request the pipeline SHALL deploy a web preview, a database branch of its own with both schemas migrated, and the pull request's Go images in an environment of their own on the cluster, and post their addresses on the pull request.

#### Scenario: Opening a pull request
- **WHEN** a pull request is opened or receives a new commit
- **THEN** a web preview is deployed, its database branch is migrated to the pull request's migrations, the Go services from that commit are running and ready, and a comment lists the web and core addresses
Test: apps/web/tests/e2e/preview-roundtrip.spec.ts

#### Scenario: Two pull requests at once
- **WHEN** two pull requests are open at the same time
- **THEN** each preview talks only to its own Go services and its own database branch
Test: apps/web/tests/unit/core-url.test.ts

### Requirement: A preview finds its own Go core
The web app SHALL resolve the Go core address from explicit configuration when set, and otherwise derive it from the deployment's branch with the same slug rule the pipeline uses. The slug MUST be a valid DNS label for any branch name.

#### Scenario: Branch name with symbols and great length
- **WHEN** the branch is `feature/Squad_Buy-and-a-very-long-description-of-the-change`
- **THEN** the web app and the pipeline derive the same lowercase slug of at most 47 characters, using only letters, digits and hyphens
Test: apps/web/tests/unit/core-url.test.ts

#### Scenario: Production uses explicit configuration
- **WHEN** the core address is configured explicitly
- **THEN** the web app uses it and ignores the branch name
Test: apps/web/tests/unit/core-url.test.ts

### Requirement: Closing a pull request removes its preview
When a pull request is closed or merged the pipeline SHALL remove that pull request's Go environment from the cluster and MUST leave no database branch of it behind.

#### Scenario: Pull request merged
- **WHEN** a pull request is merged
- **THEN** its cluster environment is uninstalled and deleted, and its database branch no longer exists
Test: manual check on the deployed preview (namespace and Neon branch are gone after the cleanup job)

### Requirement: Merging to main deploys production
A push to `main` SHALL migrate the production database, deploy the Go services from that commit to the production environment, deploy the web app to production and run a smoke test against both.

#### Scenario: Successful production deploy
- **WHEN** a pull request is squash-merged into `main`
- **THEN** the production Go core reports the merged commit at `/v1/version`, the production web app serves it, and the smoke test passes
Test: apps/web/tests/e2e/smoke.spec.ts

#### Scenario: Go rollout fails readiness
- **WHEN** the new Go pods never become ready
- **THEN** the deploy job fails and the previous release keeps serving traffic
Test: manual check on the deployed preview (helm upgrade with --atomic rolls back a release whose readiness never passes)

### Requirement: Migrations run only in the pipeline
Database migrations SHALL be plain, forward-only SQL with a version record per schema, and MUST be applied only by the pipeline, before the services that need them are deployed.

#### Scenario: Migration order
- **WHEN** a pull request adds a control and a sales migration
- **THEN** the pipeline applies both to the preview branch before the Go rollout, and each schema's version record shows the new version
Test: services/internal/db/migrate_test.go

#### Scenario: Service started against an older schema
- **WHEN** a Go service starts and the sales schema is behind the version it expects
- **THEN** its readiness check fails with the expected and actual versions
Test: services/internal/health/ready_test.go

### Requirement: The cluster is reproducible from the repository
A script in the repository SHALL create the Kubernetes cluster in an EU region (Amsterdam, ams3, because Frankfurt had no capacity) and install its ingress, certificate and metrics add-ons. Running it again MUST change nothing when the cluster already exists.

#### Scenario: First run
- **WHEN** the cluster script runs and no cluster of that name exists
- **THEN** a cluster on 1.37 is created in ams3 with an autoscaling node pool, and the scripts print its id and (once the add-ons are installed) the load balancer address
Test: manual check on the deployed preview (`deploy/cluster/up.sh` output and the cluster in the DigitalOcean API)

#### Scenario: Second run
- **WHEN** the script runs again
- **THEN** it reports the existing cluster and creates nothing new
Test: manual check on the deployed preview (second run of `deploy/cluster/up.sh` makes no create call)

### Requirement: The Go core has a public HTTPS address
Each environment's core and gateway SHALL be reachable over HTTPS with a valid certificate at hostnames derived from the environment slug and the cluster load balancer address.

#### Scenario: Calling the preview core
- **WHEN** a client calls `https://core.<slug>.<base domain>/healthz` for an open pull request
- **THEN** it gets 200 over a certificate trusted by standard clients
Test: apps/web/tests/e2e/preview-roundtrip.spec.ts

#### Scenario: Browser origin not allowed
- **WHEN** a browser page on an origin outside the platform's web origins calls the core
- **THEN** the core's CORS response does not allow that origin
Test: services/cmd/core/cors_test.go

### Requirement: Health and status show the full round trip
Every Go service SHALL expose liveness and readiness endpoints, and readiness MUST fail when Postgres or Redis is unreachable. The web app SHALL show a status page with its version, the schema versions and the result of an authenticated call to the core.

#### Scenario: Redis down
- **WHEN** Redis is unreachable
- **THEN** `/readyz` returns 503 naming Redis while `/healthz` still returns 200
Test: services/internal/health/ready_test.go

#### Scenario: Signed-in status round trip
- **WHEN** a signed-in user opens `/status` on a preview
- **THEN** the page shows the web commit, both schema versions, the core commit and the core's echo of the user's identity
Test: apps/web/tests/e2e/preview-roundtrip.spec.ts

### Requirement: Secrets stay out of the repository and logs
Keys and passwords SHALL reach the services only through the pipeline's secret stores and environment configuration, and MUST never be committed, baked into images or written to logs.

#### Scenario: Image contents
- **WHEN** a Go image is built
- **THEN** it contains only the binary and CA certificates, and no environment file or key
Test: services/cmd/core/image_test.go

#### Scenario: Logging a connection error
- **WHEN** a service fails to connect to Postgres
- **THEN** the log line names the host and the error but not the password or full connection string
Test: services/internal/db/redact_test.go
