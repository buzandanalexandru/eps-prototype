# Design

## Context

After step 2, publishing creates an immutable storefront version in one transaction with a hook point for this step; restores create a new version too. Step 1 provides the catalogue tables, `eventReadiness`, `brand_hostnames` and the pure pricing functions (`feeBreakdown`, `activePriceStep`, `instalmentSchedule`). Step 0 provides the two schemas, the roles (`eps_web` writes control, `eps_core` reads granted control objects), forced RLS, the core token and the Go core with its Redis key builder (`eps:{org}:<event>:…`). P1 and U3 in `docs/planning/architecture.md` fix the approach; this document records the snapshot's shape and the rendering and caching choices.

## Goals / Non-Goals

**Goals:**
- The Go core never reads draft tables and never recomputes fees.
- Draft edits during a sale change nothing until the next publish.
- A published shop page costs no database query on a cache hit.

**Non-Goals:**
- Holds, checkout and orders (step 4), stock counters and caps (step 5), static fallback under extreme load (step 6, E11), several shops per event (step 10).
- Recording the snapshot version on orders: the requirement belongs to step 4's `checkout`; this step only guarantees every snapshot has a stable `(event_id, version)` and id.

## Decisions

### D3.1 What a publish compiles
- The events a storefront sells are those referenced by its pages (`eventId` on event pages or catalogue sections). Publish runs, in the step 2 transaction: readiness for each such event → pool-usage check → compile → insert snapshots → set `events.live_snapshot_version` and `events.status = 'published'` → commit. After commit: call the core's `activate` endpoint per event and `revalidateTag` for the brand. A failure of the activate call does not undo the publish: the core also picks up new versions on a cache miss (D3.4).
- If an event's compiled document hashes the same as its live snapshot, no new snapshot version is written (publishing only a design change does not churn sale versions).
- A restore (step 2 rollback) compiles from the **current** catalogue: rollback restores the look, never old prices. Studio says so in the restore dialog.
- Pool-usage gate: Studio calls `GET /v1/organiser/events/{eventId}/pool-usage` (units sold plus held per pool instance, from the sales schema). Until step 4 creates sales tables it returns zeros. A pool instance whose new capacity is below its usage, or a removed product or pool that has usage, refuses the publish with the numbers.

### D3.2 Snapshot shape (`schemaVersion: 1`)
Stored in `control.sale_snapshots (id uuid, organiser_id, event_id, version int, schema_version, document_text, document jsonb generated from it, sha256, terms_sha256, storefront_version_id, published_by, published_at)`, unique `(event_id, version)`, an update/delete/truncate-rejecting trigger (only `published_by` may be cleared, for a deleted user). The document is stored as the exact text that was hashed (`document_text`), so Go verifies the SHA-256 over the same bytes (jsonb reorders keys); a check constraint ties `sha256` to the text and the document's ids and version to the row. `terms_sha256` hashes the document without id, version and time and decides whether an unchanged event gets a new version. Types in `apps/web/src/lib/snapshot/types.ts`, mirrored by a Go struct; a JSON Schema in `api/schemas/sale-snapshot.v1.json` (referenced from the OpenAPI file) is the contract both sides test against.
```
{ schemaVersion: 1, snapshotId, organiserId, brandId, eventId, version, publishedAt,
  currency, timezone, isSample,
  fee: { policy: absorb|pass_on|split, bps, fixedMinor },
  event: { slug, name:{ro,en}, startsAt, endsAt, timezone,
           venue: { name, address, lat, lng } | null,
           days:[{date, opens, closes}], lineup:[{date, artist, stage, starts}] },
  rules: { minAge?, consentUnderAge?, maxPerAccount? },
  pools: [ { poolId, name, scope: event|day|slot,
             instances: [ { instanceId, date?, slotStart?, capacity } ] } ],
  products: [ { productId, type, name:{ro,en}, perks:[{ro,en}], zone?, config, named,
                salesFrom?, salesUntil?, visible, maxPerOrder, sortOrder,
                poolIds:[...], requires:[...],
                priceSteps: [ { index, mode: by_date|by_sales, fromAt?, afterSold?,
                                faceMinor, allInMinor, fanFeeMinor, organiserNetMinor } ],
                instalments?: { depositMinor, monthlyCount, dayOfMonth, lastDueBy } } ] }
```
- Instance ids are deterministic: `<poolId>` (event scope), `<poolId>@<YYYY-MM-DD>` (day), `<poolId>@<YYYY-MM-DDTHH:MM>` (slot, event local time), so step 5's stock counters and step 4's holds can name them before any sale.
- Money is integer minor units; `allInMinor = faceMinor + fanFeeMinor` always; amounts come from step 1's `feeBreakdown`, so Go never computes fees. Instalment amounts per payment are derived from `allInMinor` by the same `instalmentSchedule` rule (deposit, equal parts, remainder on the last), which step 4 reimplements in Go against the shared vectors file.
- The document holds no personal data and no draft-only fields.
- **Renderer contract (decided in step 2's review):** the snapshot carries what step 2's shared renderer shows for an event: its `slug` (the canonical event address), the `venue` with coordinates, the `lineup`, and `days` as `{date, opens, closes}` (local `YYYY-MM-DD` and `HH:MM`). Published shops build `ShopEvent` (exported from `apps/web/src/lib/storefront/catalogue.ts`, the one shared type) from the live snapshot instead of from the draft catalogue; `venue.map` is the stored static map tile (`control.map_tiles`, see step 2) for the venue's coordinates, or null.

### D3.3 Narrow published-only reads
`control.live_snapshot(event_id uuid, version int default null)` (null: the live version; a number: that earlier version) and `control.public_storefront(brand_slug text, hostname text)` are `SECURITY DEFINER` functions (owned by the NOLOGIN NOBYPASSRLS roles `eps_snapshot_read` and `eps_shop_read`) owned by the migration role, returning only published rows (live storefront version, live snapshots, brand public fields). They are the only cross-organiser reads; `eps_core` and the storefront route get `EXECUTE` on them and nothing else new. After reading, Go scopes any sales write to the snapshot's `organiserId`.

### D3.4 Go side
`services/internal/snapshot`: `Load(eventID)` reads Redis `eps:{org}:<event>:snapshot:live` (version + document), falling back to `control.live_snapshot` and re-filling Redis; it verifies `sha256` on load and keeps a 5-second in-process copy. `activate` (organiser token, organiser must own the event) loads and caches the given version and replies with its hash. Old versions stay loadable by `(event, version)` for orders in flight. `GET /v1/public/events/{eventId}/availability` returns per product `status` (`on_sale | not_yet | ended | sold_out`), the current price step index and all-in amount, and `remaining` only when at most 50 or 5% of capacity remain; it is computed from the snapshot and the stock source (in this step, capacity minus usage, which is zero), cached 2 seconds in Redis, and served with `Cache-Control: public, max-age=2`.

### D3.5 Storefront routes and host routing
- Routes: `/s/<brand>` redirects (307) to `/s/<brand>/<locale>` where locale is the cookie, then `Accept-Language`, then the shop default, limited to enabled languages. Pages: `/s/<brand>/<locale>` (home), `/s/<brand>/<locale>/e/<event-slug>` (event page), `/s/<brand>/<locale>/<page-slug>` (info), `/s/<brand>/<locale>/checkout` (step 4).
- Host routing (as built): the proxy makes no database call. On a host that is not a platform host it rewrites every path except assets, the app's files, `/api/` and the platform's legal pages to `/s/~<host>/…` (a form no brand slug can take) and sets `x-eps-brand-host`; the page looks the host up through `public_storefront` with the same cached loader as slugs (one cache entry per host, revalidated on publish). Unknown hosts get the platform 404; a `/s/~<host>` request on a platform host is a 404 too (the header is only set by the proxy). Links inside the shop are built by one helper (`shopBase`) that knows which form is in use. Studio is never served on a brand host.
- Unknown brand, unpublished brand and another organiser's draft all give the same 404.

### D3.6 Rendering and caching
- Server components render with step 2's shared section renderer; catalogue sections read from the live snapshots (not draft tables) in published mode. Data loads are wrapped with Next.js cache tags `brand:<brandId>` and `event:<eventId>`; publish calls `revalidateTag` for the brand and its events. Shop pages are static (review decision): their own root layout under `app/(shop)/s/[brand]/[locale]/` reads no cookies or headers and loads no platform translations, the pages are generated on first request and kept at the edge until revalidated (an hour at most, a minute for hostname lookups, so a newly configured hostname appears without a publish). The data (live version, public snapshots, map tiles) is also tagged in the data cache. Because a static page cannot carry a per-request nonce, shop routes use a fixed CSP: scripts from our origin with Subresource Integrity (`experimental.sri`), plus `'unsafe-inline'` for Next.js's inline page-data scripts, whose content changes with every page and version and so cannot be listed by hash in a static header. That is acceptable on shop pages only, because they render nothing an organiser or fan can turn into markup (schema-validated JSON rendered as escaped React text; links limited to https:, mailto: and site paths; images from our origin). Studio and preview links keep the nonce CSP. `/s/<brand>` and `/` on a brand host are redirected by the proxy to the visitor's language; a static page redirects once more to the shop's default when it does not sell in that language. Publish calls `revalidateTag(tag, { expire: 0 })`, so the next request renders the new version; a brand's slug change revalidates the old and new address after commit.
- The availability island is a small client component that fetches the core's public endpoint **through our own origin** (`/api/public/events/<id>/availability`, a route that validates the core's answer with zod and is CDN-cached 2 s), so shop pages keep `connect-src 'self'`, CORS stays exact-origin only, and it works on brand hostnames. It updates status, "only N left", the current price of sales ladders and the picker's limits; HTML never contains live stock.
- Language and currency: `formatMoney` from `lib/pricing` (`Intl.NumberFormat` `ro-RO` / `en-GB` with the shop currency shown as its code: `552,48 RON` in Romanian, `RON 552.48` in English); dates in the event's time zone.
- Event pages are addressed by the EVENT's slug (`/e/<event-slug>`); page slugs are for info pages only and never one of `checkout`, `e`, `p`, `assets`, `api`, `s` (step 2's document validation refuses them).
- Shop pages carry the CSP and framing headers set in step 2 (`src/proxy.ts`): no framing, scripts by nonce, images, fonts and connections from our origin only.
- Sample organisers' shops show a "Sample data" / "Date de exemplu" banner and `noindex`.

### D3.7 All-in display rules
One component (`<AllInPrice>`) renders every fan-facing price from a snapshot price step: the all-in amount, plus, when the policy is pass on or split, an "includes platform fee of X" line on product details. "From" prices are the lowest all-in amount among visible tickets currently on sale (or the lowest upcoming one, labelled); add-ons, which cannot be bought alone, do not count. The event page shows the event's dates, venue and "from" price above its sections. Ladder notes: "Price rises to Y on <date>" for date ladders, "Price rises to Y after the next N tickets" is not shown (it would invite gaming) — by-sales ladders show only "Price may rise as tickets sell". Instalments: "D now + N × P", computed from the all-in amount. A lint rule bans formatting money outside this component in storefront code.

### D3.8 Ticket picker hand-off
The picker enforces per-product `maxPerOrder`, add-on `requires`, sales windows and availability, shows the running all-in total, and on "Continue" navigates to `/s/<brand>/<locale>/checkout?event=<id>&v=<version>&items=<productId:qty[:instance]>,…`, where `instance` is the instance qualifier the buyer chose: `YYYY-MM-DD` for a product on day pools, `YYYY-MM-DDTHH:MM` (the slot's local start date) for a time slot; the pool instance is `<poolId>@<instance>` for each scoped pool of the product. The picker also enforces the event's maximum per account across tickets. Step 4 owns that route; until then it shows "Checkout opens soon" with the selection and total.

### D3.10 Review decisions (adversarial review of the PR)
- Time and stock live in the browser: the page carries snapshot facts only (all steps with dates, sales windows, the plan); `sale-view.ts` works out the step in force, statuses, instalments and the "from" price after hydration, from the core's answer when it is for the page's snapshot version (otherwise the page's own facts, asking again in 3 s). Failures back off with jitter from 10 s to 60 s; one poll per event per page.
- Publishing re-runs the snapshot hook even when the draft revision is already published (a catalogue-only change goes live). Events the new version no longer sells go off sale (`live_snapshot_version` null); `live_snapshot` then returns nothing for them, and never a version above the live one.
- Public pages read `control.public_snapshot`, which drops `organiserNetMinor`, the fee settings and hidden products; `requires` lists only visible products; `live_snapshot` is granted to the core only.
- Snapshot schema v1 gains `rules.holdMinutes` and `rules.checkoutQuestions` (for step 4) and `lineup[].sample` (sample artists labelled in the shop's language).
- The core signs every Redis copy (snapshots, "missing" marks, availability answers) with `SNAPSHOT_CACHE_KEY`, remembers unknown events for 4 s (in process and Redis) and loads each event at most once at a time (singleflight). Redis requires a password; both secrets come from the per-environment `eps-runtime` secret CI creates once. A Traefik rate limit (200/s, burst 400 per core host) guards `/v1/public/`.
- The availability route refuses query strings (400), caches good answers 2 s with stale-while-revalidate and stale-if-error, failures 5 s, and never a 404 publicly. The answer gains per-instance `instances[]` (sold out or few left); the picker greys out sold-out days and slots and caps every pool instance across all selected lines.
- The checkout placeholder reads the URL with own-property lookups, keeps only visible products with valid days and slots, merges duplicate lines and re-applies the per-order and per-account caps; the language switch keeps the selection; days are shown localised.
- One h1 per page (the event summary is not a heading); the sample hostname `shop.sunwave.test` is seeded for the local host-routing test.

### D3.9 Who publishes with snapshots (as built)
Step 2's `publishHooks` stays empty by default; Studio's publish and restore actions and the seed pass the snapshot hook explicitly. Studio's hook asks the core for pool usage with the organiser's own core token (4 s timeout; no token or no answer refuses the publish with a retry message). The seed publishes a brand's first version only, before any sale can exist, with a usage source that reports none. The seed's sample shops therefore publish Galaxia 2027, Promenada and Sunwave Beach Day, which become `published` (frozen for later seed runs, per the sample-data rules); the step 1 seed test now edits only draft events. The Go core requires control schema 8.

## Risks / Trade-offs

- [Activate call lost after commit] → the core reloads on a cache miss and on the 5-second in-process expiry, so the new version is live within seconds even without the call.
- [Revalidation missed, shop shows the old version] → the publish response waits for `revalidateTag`; a Playwright test checks the new content after publish; the published-version id is in a `meta` tag for debugging.
- [Snapshot documents grow with 74 day-instances × products] → instances are per pool, not per product; NIBIRU's resort is well under 1 MB; Redis holds one copy per live version.
- [The by-sales ladder hides the exact threshold] → deliberate, against gaming; the public note still states that prices never depend on the buyer.

## Open Questions

None that change the specs.
