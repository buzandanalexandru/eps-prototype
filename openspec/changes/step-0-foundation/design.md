# Design

## Context

The repository holds only planning documents and OpenSpec. Accounts exist: a Vercel team ("devblocks", `team_pR8jlYM98Wtxqefs0u6pHuXP`, GitHub app installed on the repo, no project yet), a Neon project (`eps-prototype`, `odd-sun-50771070`, aws-eu-central-1, Postgres 18, Neon–Vercel integration that gives each preview its own branch and `DATABASE_URL`) and a DigitalOcean account with no cluster. GitHub Actions has the secrets `DIGITALOCEAN_ACCESS_TOKEN` and `NEON_API_KEY`. The session reaches `api.digitalocean.com` through a proxy that injects the token (`Authorization: Bearer proxy-injected`); it has no database password, so migrations run only in CI. Architecture decisions S1–S3, P2, U1, U2, U5 and R1 (`docs/planning/architecture.md`) are fixed; this document records only the concrete choices they leave open.

## Goals / Non-Goals

**Goals:**
- A green pipeline in which every PR gets a public, working preview of both halves, and `main` is production.
- Isolation by organiser that holds even when application code forgets a filter.
- One sign-in that the Go core trusts without calling back to Next.js on each request.
- Reproducible infrastructure: the cluster and its add-ons can be rebuilt from the repo.

**Non-Goals:**
- Any product screen beyond sign-in and `/status` (organisers, catalogue and builder start in steps 1–2).
- Autoscaling, final resource sizing and k6 (step 12); metrics and dashboards (step 6); the outbox and streams (step 4).
- Custom domains: routing reads the host so they become configuration, but no domain screen (B8 is cut).

## Decisions

### D0.1 Repository and tooling (S1, S2, S3)
- npm workspaces at the root (`apps/web`, plus the existing OpenSpec dev dependency). Node 22 LTS, Go 1.25+.
- `services/` is one Go module (`github.com/<owner>/eps-prototype/services`): `cmd/{gateway,core,worker,fakepsp,simulator}` and `internal/{api,auth,db,tenancy,redisx,health,version}`. Logging with `log/slog` as JSON. HTTP with the standard library router.
- `api/openapi.yaml` (OpenAPI 3.1) is the single contract. TypeScript types with `openapi-typescript` (+ `openapi-fetch` client) into `apps/web/src/lib/core/`; Go strict-server stubs with `oapi-codegen` (std-http target) into `services/internal/api/`. Errors use `application/problem+json` (RFC 9457) with a `code` and a `requestId`. Generated files are committed; CI regenerates and fails on any diff.
- Step 0 endpoints: `GET /healthz`, `GET /readyz` (Postgres and Redis reachable, migrations at the expected version), `GET /v1/version`, `GET /v1/whoami` (verifies the token, echoes its claims). Every binary serves `/healthz` and `/readyz`; the simulator is a Kubernetes Job, disabled by default.

### D0.2 Database: schemas, roles, migrations (P2, U2)
- Migrations are plain SQL run by `pressly/goose`, forward-only: `apps/web/db/migrations/` for `control` and `services/db/migrations/` for `sales`, each with its own version table (`control.goose_db_version`, `sales.goose_db_version`). Better Auth's tables are generated once with its CLI into a control migration.
- Migration `0001` creates two NOLOGIN, NOBYPASSRLS roles: `eps_web` (all on `control`, read on `sales`) and `eps_core` (all on `sales`, read on the `control` tables the core needs: `organisers` and the public-key columns of the auth key table). The branch's connection user is granted both.
- Every application transaction begins with `SET LOCAL ROLE eps_web|eps_core` and `select set_config('app.organiser_id', $1, true)`. Tenant tables have `ENABLE` and `FORCE ROW LEVEL SECURITY` with one policy: `organiser_id = nullif(current_setting('app.organiser_id', true), '')::uuid`. With no organiser set a query sees nothing and an insert fails. Fan-facing reads that cross organisers (a public storefront) go through `SECURITY DEFINER` functions or published snapshots that expose only public columns, added by the steps that need them.
- Platform tables without `organiser_id` (users, sessions, auth keys, the organiser registry itself) are listed in one allowlist; a test fails if any other table lacks `organiser_id NOT NULL` or RLS.
- Step 0 creates `control.organisers (id uuid, slug, name, created_at)` and `control.memberships (organiser_id, user_id, role)` so isolation can be tested; step 1 extends them.
- Web uses `DATABASE_URL` (pooled) for app queries inside transactions; Better Auth uses `DATABASE_URL_UNPOOLED` with `search_path=control`. Go uses pgx with the branch URL from a Kubernetes secret.

### D0.3 Redis keys (U2)
Every key is built by one Go helper: `eps:{<organiser_id>}:<event_id|_>:<kind>:<rest>`; platform keys use `eps:platform:`. The brace hash tag keeps an organiser's keys together if Redis Cluster arrives later; step 5 may add a shard suffix inside the event part. Redis in the chart: a small own sub-chart running the official `redis:8` image as a StatefulSet (no Bitnami dependency), AOF off, one replica per environment.

### D0.4 Sign-in and tokens (U1)
- Better Auth in `apps/web` with the magic-link plugin, Google (enabled only when `GOOGLE_CLIENT_ID/SECRET` exist), and the JWT plugin (EdDSA / Ed25519, keys stored in the control schema, private keys encrypted with `BETTER_AUTH_SECRET`, published at `/api/auth/jwks`, rotated every 30 days with the old key kept for 7).
- Signing keys therefore live per database branch rather than as CI-generated Vercel secrets: each preview gets its own keys with no Vercel write from CI. This is a deliberate refinement of the CLAUDE.md line on signing keys; the rotating-QR key (step 7) still comes from CI.
- Core token: `iss` = web origin, `aud` = `eps-core`, `sub` = user id or `guest:<id>`, `kind` = `organiser|fan|guest`, `org` (active organiser, organiser tokens only), `role`, `sid`, `jti`, lifetime 5 minutes. Next.js server code gets one per call to Go; the browser can get one from `GET /api/core-token` for direct gateway calls (step 5).
- Go verifies the signature, `aud`, `exp` (30 s leeway) and `kind`, reading public keys from the control key table through its read grant (cached 60 s, re-read at most once a minute on an unknown `kid`). Reading from the shared database avoids a callback over Vercel preview protection and keeps each preview's core paired with its own keys.
- Demo path: when `EPS_DEMO_SIGNIN=on` (previews and the public demo, never by default), `/sign-in` shows "Continue as demo organiser" (one shared demo organiser user, made owner of the sample organiser by step 1's seed) and "Continue as demo fan" (a fresh `demo-fan-<random>` user per click, so visitors never share carts). Rate-limited per IP. Playwright uses these buttons on deployed previews.
- Magic-link mail goes through a sender interface. Without a mail provider key, previews and the demo show the link in an on-screen "demo mailbox" for addresses under the demo domain only, and log nothing sensitive.
- Guests: a signed, httpOnly `eps_guest` cookie with a random id, created on the first buy action; tokens with `kind=guest`. Claiming into an account is step 4 (D8).

### D0.5 Hosting, addresses and previews (R1, I5)
- Vercel project `eps-prototype` (id prj_Wxv6KSuf1LnYTusYGNwtuHdkMXvD, team devblocks), root `apps/web`, functions in fra1, already created through the Vercel connector with Vercel Authentication turned off; it is linked to the repo by the owner in the Vercel UI (or CI deploys with `VERCEL_TOKEN` if the link is missing). Preview deployment protection is turned off so Playwright and interviewers can open previews (they hold no real data). Non-secret env vars set once: `CORE_BASE_DOMAIN`, `EPS_DEMO_SIGNIN`.
- Cluster `eps-prototype-ams3` (DOKS, region ams3 because fra1 had no capacity for small nodes, 1.37.2-do.0, node pool `pool-main` of `s-2vcpu-4gb` autoscaling 2 to 4 nodes; already created on 27 Nov, id 920c7f76-7300-4be7-8210-d8a5cfc9248a) created by `deploy/cluster/up.sh`: idempotent, curl + jq against the DigitalOcean API, token from `DO_TOKEN` (default `proxy-injected` for the session, the Actions secret in CI). Add-ons (Traefik as ingress controller, since ingress-nginx is retired; cert-manager with a Let's Encrypt HTTP-01 `ClusterIssuer`; metrics-server) are installed by `deploy/cluster/addons.sh`, run by the `cluster-bootstrap` workflow because the session may not reach the cluster's API.
- Addresses: the Traefik load balancer IP gives `CORE_BASE_DOMAIN = <ip>.sslip.io`. Each environment has a slug (`prod` for `main`; for a PR the branch name lowercased, non-alphanumerics to `-`, cut to 40 characters plus a 6-character hash when cut). The core is at `https://core.<slug>.<CORE_BASE_DOMAIN>` and the gateway at `https://gw.<slug>.<CORE_BASE_DOMAIN>`, each with its own cert-manager certificate.
- How a preview finds its core: the web app resolves the core URL at runtime as `CORE_API_URL` if set (production), otherwise from `VERCEL_GIT_COMMIT_REF` through the same slug function. The function is implemented once in TypeScript and once in the CI script, both tested against `deploy/preview-slug-vectors.json`.
- CORS on the core and gateway allows the production web origin and `https://eps-web-*-devblocks.vercel.app`.

### D0.6 CI workflows (R1)
- `ci.yml` on every PR and push: web lint, typecheck, Vitest, build; Go vet, test (testcontainers with `postgres:18` and `redis:8`); OpenAPI drift; Helm lint and template; migration dry run against an ephemeral Postgres.
- `preview.yml` on PR open/sync: build one image per binary (one multi-stage Dockerfile, target per binary) to `ghcr.io/<owner>/eps-<binary>:<sha>` (public packages, no pull secret); get or create the Neon branch `preview/<branch>` (the integration's naming) through the Neon API, waiting up to 5 minutes for the integration's branch to avoid a duplicate; run both schemas' migrations; write the branch URL to a secret in namespace `eps-<slug>`; `helm upgrade --install eps-<slug>`; wait for readiness; then wait for the Vercel deployment (GitHub `deployment_status` from the Vercel app) and run Playwright against the preview URL. A PR comment lists the web and core URLs.
- `preview-cleanup.yml` on PR close: `helm uninstall`, delete the namespace; the Neon branch is removed by the integration (or by this job if it remains).
- `deploy.yml` on push to `main`: migrate the Neon default branch, build images, `helm upgrade --install eps` in namespace `eps-prod`, smoke-test the core, wait for the Vercel production deployment and run the Playwright smoke test.
- `bootstrap-secrets.yml` (manual): generates `BETTER_AUTH_SECRET` per Vercel environment through the Vercel API. It needs a `VERCEL_TOKEN` Actions secret; if that does not exist, the step stops and asks the owner (an account action under CLAUDE.md).

### D0.7 Web app shell (U5)
Next.js App Router with Tailwind and shadcn/ui; next-intl with `ro` (default) and `en`, chosen from a `NEXT_LOCALE` cookie then `Accept-Language`, without a URL prefix for Studio. Middleware reads the host: platform hosts (`*.vercel.app`, `localhost`) route by path; any other host is passed on as a candidate brand domain (resolved in step 3). `/status` shows web version, database schema versions, and the core's `/v1/version` and `/v1/whoami` result.

## Risks / Trade-offs

- [Neon–Vercel integration and CI both creating the preview branch] → CI waits for the integration's branch name before creating one, and treats "already exists" as success.
- [Vercel may build before CI has migrated the branch] → pages handle missing tables as "not ready" on `/status`; the deployed Playwright run starts only after the migrate job.
- [Let's Encrypt rate limits on the shared `sslip.io` domain] → one certificate per environment host, reused across pushes; if issuance is refused, fall back to `nip.io` with the same scheme (one variable change).
- [`SET LOCAL ROLE` forgotten in a code path] → it lives only in the single transaction helper of each side; a test opens a raw connection through the helper and checks `current_user` and that RLS applies.
- [Long branch names breaking DNS labels] → the slug function caps length and is tested on shared vectors.
- [Load balancer IP changes if the controller is reinstalled] → `CORE_BASE_DOMAIN` is a single Vercel variable and a chart value, updated by `addons.sh` output.

## Migration Plan

Greenfield. Order: Vercel project → cluster and add-ons → first PR (this step) with previews → merge → production. Rollback: `helm rollback` for the core; Vercel instant rollback for the web app; migrations are forward-only, so a bad migration is fixed by a new one.

## Open Questions

- Mail provider for magic links outside the demo domain (none in the decided stack). Until the owner chooses one, real-address magic links are disabled and the demo mailbox serves demo addresses.
- Google OAuth client credentials (owner account action); Google sign-in stays hidden until they exist.
