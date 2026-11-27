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
