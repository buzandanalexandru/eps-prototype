# Proposal

Step 3: Publish and storefront

**Demo act:** Act 1's payoff (the organiser presses Publish and the branded shop is live at `/s/<brand>` with honest all-in prices) and the base of Act 2 (the Go core sells only what was published).
**Covers:** feature D1 (all-in price everywhere); decision P1 (publish compiles an immutable, versioned sale snapshot that the Go core reads), with U3's rendering half (server components render the published version, cached until the next publish).

## Why

The organiser must be able to keep editing during a live on-sale without touching it, and every order must be traceable to the exact products, pools, prices and rules it was sold under. Compiling the catalogue into an immutable snapshot at publish time gives the Go core a fast, stable input and gives the audit trail for free. The public shop then has to show the price the fan will pay, everywhere: NIBIRU today shows 539 RON and adds the platform fee only at checkout (`docs/research/nibiru-eps-teardown.md` §4, §8), which EU consumer law does not allow.

## What Changes

- Publishing a brand's storefront (including a restore) also compiles one sale snapshot per event the storefront sells: products, pools expanded into day and slot instances, price ladders with precomputed all-in amounts, fee policy, buying rules and instalment settings, stored immutably with a version and a content hash. Publishing is refused if an event fails the readiness check or if a pool would shrink below what the core reports as sold or held.
- The Go core reads only published snapshots (through a narrow published-only read), caches them in Redis, is told about a new version right after publish, and serves a public availability endpoint per event.
- Studio shows the live snapshot version per event and whether the catalogue has unpublished changes.
- The public storefront at `/s/<brand>/<locale>/…`, rendered by server components from the live storefront version and the event snapshots, cached until the next publish; a live availability island from the core; the same shop answers on a brand's configured hostname.
- All-in prices on every fan-facing surface: ticket grid, "from" prices, product details, picker totals, price ladder notes and instalment summaries, all from the snapshot's precomputed amounts.
- A ticket picker with quantity limits and a running all-in total that hands the selection to the checkout route (built in step 4).

## Capabilities

### New Capabilities
- `sale-snapshots`: compiling, storing, versioning and serving the immutable sale snapshots, the publish gate and the core's availability read.
- `storefront`: the public shop at `/s/<brand>` and on configured hostnames, rendering and caching of the published version, all-in prices, availability display and the ticket picker hand-off.

### Modified Capabilities
None.

## Impact

- New control-schema table `sale_snapshots` (immutable rows, organiser-scoped) and pointer columns on `events`; `SECURITY DEFINER` read functions for published data only; `eps_core` read grant on them.
- Contract additions in `api/openapi.yaml`: `POST /v1/organiser/events/{eventId}/snapshots/{version}/activate`, `GET /v1/organiser/events/{eventId}/pool-usage`, `GET /v1/public/events/{eventId}/availability`.
- New Go package `services/internal/snapshot` (loader, Redis cache, availability) used by steps 4 and 5.
- Web: `apps/web/src/lib/snapshot/` (compiler, shared types), the storefront routes under `apps/web/src/app/s/[brand]/`, host-routing in middleware, cache tags revalidated on publish.

## Status

Done. Merged as `1d75efd`, PR #5, squash.

**What was built:**
- Immutable sale snapshots, schema v1 with sha256. Each price step carries precomputed all-in, fan-fee and organiser-net amounts. Rules include `holdMinutes` and `checkoutQuestions`, ready for step 4.
- Publishing compiles snapshots for the events the shop sells, and the core is told to activate them. Events the new version no longer sells go off sale.
- `control.live_snapshot()`: core only, never above the live version. `control.public_snapshot` for the shop: no organiser net, no fee settings, no hidden products.
- The public shop at `/s/<brand>/<locale>` and on configured hostnames, as static pages cached until the next publish.
- All-in prices everywhere, with `Intl` money formatting.
- A ticket picker with caps and per-instance availability (sold-out days and slots greyed out) that hands the selection to the checkout route.
- Studio shows the live snapshot version and unpublished catalogue changes.
- In the Go core: the snapshot loader and availability, with HMAC-signed Redis copies, a 4 s negative cache and singleflight. Redis now requires a password, from the CI-made `eps-runtime` secret. A Traefik rate limit guards `/v1/public/`.

**Deviations (design.md D3.9, D3.10):**
- Shop pages are static, with their own root layout. Time and stock are worked out in the browser.
- The shop CSP uses Subresource Integrity plus `'unsafe-inline'` instead of a nonce, on shop pages only. Studio keeps its nonce CSP.
- The availability route refuses query strings.
- The data-cache key includes the deployment id, so previews never read another deployment's cache.
- The seed publishes snapshots only on a brand's first publish. Production brands published in step 2 were therefore published once more from Studio (demo user) after the deploy, to compile their snapshots.

**Checks run:**
- Locally: Go vet and tests with testcontainers pass. Vitest 317/317. tsc, eslint, next build (shop routes static), helm lint and actionlint are clean.
- Local Playwright against `next start`: 64 passed, 16 skipped. `openspec validate --all --strict`: 18/18. The contract drift check is in sync.
- Three adversarial reviews (correctness; security and data; spec and scale). All confirmed findings were fixed and none were rejected. The table of findings and their fixing commits is in the PR body.
- All 17 PR checks pass on `6b0a87c`, including the deployed e2e. The first deployed e2e run failed on 2 causes, both fixed: a CSP `frame-src` console error, and Vercel's data cache being shared across deployments.
- Independent Playwright check of the preview: `/s/nibiru/{en,ro}` and the Galaxia 2027 event page at 1440 px and 390 px. Status 200, no console errors, no horizontal scroll, GA shown as `RON 552.48` in English and `552,48 RON` in Romanian.
- `x-vercel-cache: HIT` on repeat requests for the shop pages. An unknown brand returns 404.
- Production: deploy run 8 is green. The production seed (run 46108134349) is green.
- Both NIBIRU shops were then published from Studio, compiling the version 1 snapshots.
- Production Playwright smoke test, same checks as on the preview: GA shows `RON 552.48` / `552,48 RON`, there are no console errors and no horizontal scroll, and repeat requests get `x-vercel-cache: HIT`.
- The production core is ready. It serves Galaxia 2027 availability (version 1) through `/api/public/events/<id>/availability`, refuses `activate` without a token (401) and returns 404 for unknown events.
