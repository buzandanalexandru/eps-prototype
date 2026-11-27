# Tasks

## 1. Repository skeleton

- [x] 1.1 Create branch `step-0-foundation`; add npm workspaces, `.editorconfig`, `.gitignore` (env files, build output), `.nvmrc` (Node 22) and the folder layout from spec-map.md; verify `npm install` succeeds at the root
- [x] 1.2 Scaffold `apps/web` (Next.js App Router, TypeScript strict, Tailwind, shadcn/ui init, Vitest, Playwright with desktop and phone projects); verify `npm run build -w apps/web`, `npx vitest run` and `npx playwright test --list` succeed
- [x] 1.3 Add next-intl with `ro` (default) and `en`, cookie then Accept-Language detection, and `apps/web/messages/{ro,en}.json`; verify a unit test that switches the cookie renders Romanian text with diacritics and English text
- [x] 1.4 Scaffold the Go module `services/` with `cmd/{gateway,core,worker,fakepsp,simulator}` and `internal/{version,health}`; each binary logs JSON with slog and serves `/healthz`; verify `go build ./...` and `go vet ./...`
- [x] 1.5 Add one multi-stage `services/Dockerfile` with a target per binary (distroless static, CA certs only); verify `docker build --target core` and `services/cmd/core/image_test.go` (no env files or keys in the image)

## 2. Contract

- [x] 2.1 Write `api/openapi.yaml` (3.1) with `/healthz`, `/readyz`, `/v1/version`, `/v1/whoami`, bearer security and the problem+json error schema; verify it lints with the generator
- [x] 2.2 Generate TS types and `openapi-fetch` client into `apps/web/src/lib/core/` and Go strict-server stubs into `services/internal/api/`; add `npm run gen:api` and `make gen`; verify `services/internal/api/contract_test.go` (every operation has a handler; schema-invalid requests get problem+json)
- [x] 2.3 Add the drift check script that regenerates and runs `git diff --exit-code`; verify it fails after a hand edit to the YAML and passes after regeneration

## 3. Database, roles and isolation

- [x] 3.1 Add goose and migration folders `apps/web/db/migrations` (control) and `services/db/migrations` (sales) with separate version tables; `0001` creates schemas, NOLOGIN NOBYPASSRLS roles `eps_web` and `eps_core` with their grants; verify `services/internal/db/migrate_test.go` on testcontainers `postgres:18`
- [x] 3.2 Add `control.organisers`, `control.memberships` and a sample organiser table in `sales` (`sales.probe` used only by tests) with forced RLS and the organiser policy; verify `migrate_test.go` applies both schemas in order
- [x] 3.3 Implement the single transaction helper on each side (`SET LOCAL ROLE`, `set_config('app.organiser_id', …, true)`): Go in `services/internal/db`, TS in `apps/web/src/lib/db/tx.ts`; verify `services/internal/tenancy/rls_test.go` (filterless read, cross-organiser insert/update, no-scope empty, current role, cross-schema write refused)
- [x] 3.4 Add the platform allowlist and schema check; verify `services/internal/tenancy/schema_test.go` fails on a test-only table without `organiser_id` or without FORCE RLS
- [x] 3.5 Add the connection-string redaction for logs; verify `services/internal/db/redact_test.go`
- [x] 3.6 Add `/readyz` checks (Postgres, Redis, expected schema versions); verify `services/internal/health/ready_test.go` (Redis down → 503 naming Redis; schema behind → 503 with versions; `/healthz` stays 200)

## 4. Redis

- [x] 4.1 Add `services/internal/redisx` with the key builder `eps:{<org>}:<event|_>:<kind>:…` and platform keys; verify `services/internal/redisx/keys_test.go` (prefix, empty organiser rejected)
- [x] 4.2 Add the Redis sub-chart (official `redis:8` StatefulSet) to `deploy/helm/eps`; verify `helm template` renders it and a testcontainers `redis:8` ping test passes

## 5. Identity

- [x] 5.1 Add Better Auth with the control-schema adapter, generate its tables into a control migration, add magic link with the sender interface and the demo mailbox; verify `apps/web/tests/unit/auth-magic-link.test.ts` (first sign-in, reused and expired links)
- [x] 5.2 Add Google behind configuration and account linking by verified email; verify `apps/web/tests/unit/auth-config.test.ts`
- [x] 5.3 Add the demo sign-in path behind `EPS_DEMO_SIGNIN` (shared demo organiser, fresh demo fan per click, per-IP rate limit); verify `auth-config.test.ts` (off → 404, no buttons)
- [x] 5.4 Add the Studio guard, `/sign-in` page (RO/EN) and sign-out; verify `apps/web/tests/unit/auth-session.test.ts` and `apps/web/tests/e2e/sign-in.spec.ts` locally (redirect and return, demo organiser, two demo fans, demo mailbox)
- [x] 5.5 Add the JWT plugin (EdDSA, 5-minute core tokens, claims per design D0.4, 30-day rotation with 7-day overlap), `/api/core-token` and guest cookie; verify `apps/web/tests/unit/core-token.test.ts` (organiser, fan, guest claims, rotation)
- [x] 5.6 Implement Go verification in `services/internal/auth` (keys read from the control key table, 60 s cache, one reload per minute on unknown kid, 30 s leeway, kind checks) and wire `/v1/whoami`; verify `services/internal/auth/verify_test.go`
- [x] 5.7 Add organiser API scoping middleware (organiser from token, 404 for other organisers' ids, 403 for fan and guest); verify `services/internal/tenancy/api_scope_test.go`

## 6. Web shell and status

- [x] 6.1 Add host-reading middleware (platform hosts route by path; others marked as candidate brand domains) and the core URL resolver with the slug function; add `deploy/preview-slug-vectors.json`; verify `apps/web/tests/unit/core-url.test.ts` (explicit URL wins, long and symbolic branch names, vectors)
- [x] 6.2 Add the `/status` page (web commit, schema versions, core version and whoami echo; "not ready" when tables are missing); verify a Vitest test of the status loader with a fake core
- [x] 6.3 Add CORS on core and gateway for the production web origin and the project's preview origins; verify `services/cmd/core/cors_test.go`

## 7. Infrastructure

- [x] 7.1 Write `deploy/cluster/up.sh` (idempotent, curl + jq, `DO_TOKEN` defaulting to `proxy-injected`; region ams3, 1.37.2-do.0, `s-2vcpu-4gb` pool autoscaling 2 to 4, matching the cluster already created); run it from the session; verify it finds the existing cluster `eps-prototype-ams3` running and creates nothing
- [x] 7.2 Write `deploy/cluster/addons.sh` (Traefik, cert-manager with Let's Encrypt HTTP-01 ClusterIssuer, metrics-server; prints the load balancer IP) and the `cluster-bootstrap.yml` workflow; verify the workflow run prints the IP and `kubectl get clusterissuer` is Ready
- [x] 7.3 Write the umbrella chart `deploy/helm/eps` with sub-charts per binary (Deployment, Service, probes, Ingress with cert-manager annotations for `core.<slug>` and `gw.<slug>`; simulator Job disabled) and values for preview and production; verify `helm lint` and `helm template` in CI
- [x] 7.4 Create the Vercel project `eps-web` (team devblocks, root `apps/web`, fra1) through the Vercel connector, link the repo, turn off preview deployment protection, set `CORE_BASE_DOMAIN` and `EPS_DEMO_SIGNIN`; verify `get_project` shows the settings and a branch push builds
- [ ] 7.5 Add `bootstrap-secrets.yml` to generate `BETTER_AUTH_SECRET` per Vercel environment; if no `VERCEL_TOKEN` Actions secret exists, stop and ask the owner; verify the variable exists (value hidden) in `filter_project_envs`

## 8. CI and deploys

- [ ] 8.1 Write `ci.yml` (web lint, typecheck, Vitest, build; Go vet and tests with testcontainers; contract drift; helm lint/template; migration dry run); verify a run is green and a deliberately failing test turns it red on a scratch commit
- [ ] 8.2 Write `preview.yml` (images to ghcr.io, public packages; get-or-create Neon branch `preview/<branch>` waiting for the integration; migrate both schemas; namespace secret; `helm upgrade --install --atomic`; wait for the Vercel `deployment_status`; Playwright against the preview; PR comment with URLs); verify on this step's PR that the comment appears and `/healthz` answers over HTTPS
- [ ] 8.3 Write `preview-cleanup.yml` (helm uninstall, delete namespace, delete a leftover Neon branch); verify on a throwaway PR that both are gone after closing
- [ ] 8.4 Write `deploy.yml` for `main` (migrate default branch, images, `helm upgrade --install --atomic eps` in `eps-prod`, core smoke, wait for Vercel production, Playwright smoke); verify after merge (task 9.6)
- [x] 8.5 Write `apps/web/tests/e2e/preview-roundtrip.spec.ts` (demo sign-in, `/status` shows web and core commits, both schema versions and the whoami echo; core `/healthz` over HTTPS) and `apps/web/tests/e2e/smoke.spec.ts`; verify they pass locally against `next dev` with a local core

## 9. Step checks (CLAUDE.md loop)

- [x] 9.1 Run the step's tests locally (Vitest, Go tests with testcontainers), then the full suite once; verify all pass
- [x] 9.2 Run `npx openspec validate step-0-foundation --strict`; verify it passes
- [ ] 9.3 Open the PR to `main`; verify CI is green and the preview comment lists the web and core URLs
- [ ] 9.4 Run the three adversarial reviewers in parallel (correctness; security and data; spec and scale), fix every confirmed finding or record its rejection in the PR, re-run affected tests; verify no open confirmed finding
- [ ] 9.5 Run Playwright against the deployed preview URL (desktop and phone, RO and EN on the sign-in page, no console errors, no horizontal scroll) plus API checks on the preview core (`/healthz`, `/v1/whoami` with and without a token); verify all pass
- [ ] 9.6 Squash-merge when CI, reviews and deployed tests are green; verify the production deploy job and the Playwright smoke test against production pass

## Workflow follow-up

- Archive with `/opsx:archive`: merge the deltas into `openspec/specs/`, add `## Status` to the proposal (what was built, deviations, checks with results) and tick step 0 in `docs/planning/build-plan.md`.
- Verify the archived specs validate.
