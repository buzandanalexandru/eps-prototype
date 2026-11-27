# Proposal

Step 0: Foundation

**Demo act:** all acts (nothing on screen yet; every later act runs on what this step builds, and the closing architecture segment shows it).
**Covers:** features I1 (organisers kept apart), I5 (public demo deployment); decisions S1, S2, S3, P2, U1, U2, U5.

## Why

Every later step needs the same base: one repository holding the Next.js control plane and the Go data plane, a contract between them, a database whose organisers cannot see each other, a sign-in that both halves trust, and a pipeline that puts every pull request on a real, public preview. Building it first and proving it with a hello-world round trip (browser → Vercel → Go on DOKS → Neon and Redis) means steps 1 to 13 only add features, never plumbing.

## What Changes

- Monorepo per S1: `apps/web` (Next.js App Router, TypeScript, Tailwind + shadcn/ui, next-intl with `ro` and `en`), `services/` (one Go module with `cmd/gateway`, `cmd/core`, `cmd/worker`, `cmd/fakepsp`, `cmd/simulator`, each a hello-world binary with health endpoints), `api/openapi.yaml` (the contract, S3) with generated TypeScript types and Go server stubs, `deploy/helm/eps` (umbrella chart, one sub-chart per Go service plus Redis), `deploy/cluster/` (reproducible DOKS cluster script), `load/` (empty).
- Postgres on Neon (Postgres 18) with two schemas, `control` (owned by Next.js) and `sales` (owned by Go), forward-only SQL migrations run by CI, and row-level security keyed on `organiser_id` (P2, U2).
- Sign-in with Better Auth in Next.js: email magic link, Google, and a "continue as demo organiser / demo fan" path for the demo and for Playwright on deployed previews; guest identities for buyers without an account; short-lived tokens signed by Next.js and verified by Go against a published key set (U1).
- Redis in the cluster with organiser- and event-prefixed keys (U2).
- CI on GitHub Actions: tests, contract drift check, images to ghcr.io, per-PR preview (Vercel preview + Neon branch + the PR's Go images in their own namespace on DOKS), production deploy on merge, migrations in CI.
- A DOKS cluster in fra1 created from a script, a public HTTPS address for the Go core through an ingress on the cluster load balancer (`<ip>.sslip.io` hostnames, TLS via cert-manager), and a `/status` page that shows the full round trip.
- The Vercel project (team "devblocks", root `apps/web`) is created and linked to the repo.

## Capabilities

### New Capabilities
- `delivery-pipeline`: CI checks, contract sync, preview and production deploys, migrations, cluster creation, public addresses, health and the status round trip.
- `identity`: sign-in (magic link, Google, demo path), sessions, guest buyers, short-lived tokens for the Go core and their verification.
- `tenancy`: organiser isolation in Postgres (row-level security, schema ownership), in Redis keys and in API scoping.

### Modified Capabilities
None (first step).

## Impact

- New code everywhere: `apps/web`, `services/`, `api/`, `deploy/`, `.github/workflows/`.
- External systems: Vercel project (new), Neon project `eps-prototype` (`odd-sun-50771070`, aws-eu-central-1) gains the schemas and preview branches, DigitalOcean gains a DOKS cluster and a load balancer in fra1, ghcr.io gains one image per Go binary.
- New dependencies inside the decided stack: Better Auth (with its JWT plugin), next-intl, shadcn/ui, Vitest, Playwright, pgx, go-redis, testcontainers-go. Small utilities: `openapi-typescript`, `oapi-codegen`, `jose`, `pressly/goose` (migrations). Cluster add-ons: Traefik, cert-manager, metrics-server.
- Account actions the owner may need to take (see design.md, Open Questions): Google OAuth client, a mail sender for magic links, and a Vercel token for CI.
