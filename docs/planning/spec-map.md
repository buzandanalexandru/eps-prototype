# Spec map and conventions

Every OpenSpec change uses these capability names, so specs stay consistent across the 14 steps.

## Capabilities by step

Capability names are kebab-case and flat (`openspec/specs/<name>/spec.md`).

| Step | New capabilities | Feature / decision IDs |
|---|---|---|
| 0 foundation | `delivery-pipeline` (CI, preview and production deploys, health), `identity` (sign-in, tokens, guest buyers), `tenancy` (organiser isolation) | S1–S3, P2, U1, U2, U5, I1, I5 |
| 1 organisers and catalogue | `organisers` (sign-up, brands, roles, fee policy), `catalogue` (events, product types, capacity pools, price ladders, buying rules, instalment settings), `sample-data` (NIBIRU seed) | A1–A3, A6, C1–C5, C8–C10 |
| 2 builder | `storefront-builder` (brand kit, templates, sections and blocks, live preview, versions and publishing, locked checkout, languages, email and ticket design) | B1–B7, B10, B12, U3 |
| 3 publish and storefront | `sale-snapshots`, `storefront` (public shop at /s/<brand>, all-in prices) | P1, D1 |
| 4 checkout and payments | `checkout` (holds, order states, status page, guest checkout, questions, resume link, instalment checkout), `payments` (provider adapters, fake providers, failover, idempotency, Apple/Google Pay, saved cards), `fan-wallet` (ledger, top-ups, pay from balance, season credit), `event-stream` (outbox and streams) | D2–D9, D11–D13, O3, O4, O6, P3 |
| 5 on-sale engine | `sale-settings`, `waiting-room` (pre-queue, shuffle, signed passes, admission rate), `bot-defence` (scoring, caps per person, risk-based OTP), `inventory` (sharded holds, reconciliation) | E1–E6, E12, O1, O2 |
| 6 simulator and dashboard | `onsale-simulator`, `onsale-dashboard`, `observability`, `graceful-degradation` | E7, E8, E11, I4, R2, R3 |
| 7 tickets and wallet | `tickets` (issuing, rotating QR, personalisation, online check-in, wallet pass preview), `trip-wallet`, `fan-account`, `door-scanning` | F2–F5, F8, G2, O5 |
| 8 squad buy | `squad-buy` | F1, O7 |
| 9 resale, waitlist, lottery | `resale`, `waitlist`, `lottery-sales`, `upgrades` | F6, E9, E10, C7 |
| 10 organiser growth | `organiser-dashboard`, `audience`, `webhooks`, `promoter-hub`, `sales-channels`, `marketplace`, `payouts` | G1, G3, G5, C6, B11, A4, A5 |
| 11 AI | `ai-copilot`, `ai-concierge`, `ai-agent-access` (MCP server) | H1–H3, U4 |
| 12 scale proof | `scale-proof` (Helm, autoscaling, k6 results, roadmap page) | I2, I3, R1, R4, G6 (as roadmap) |
| 13 deliverables | none (`skip_specs: true`): video script and one-page vision as docs | — |

## Rules for writing changes
- **Each change writes only its own capabilities** (the ones listed for its step above, each a new spec with a `## Purpose`). It never adds, modifies or removes requirements in another step's capability, so all 14 changes validate before any is archived. A later step that needs a change to an earlier requirement is revised with `/opsx:update` once the earlier step is archived. Cross-step behaviour is written as a requirement in the later step's own capability; for example, squad buy's rule on holds lives in `squad-buy`, not in `checkout`.
- **Specs describe behaviour**, not libraries or class names. The architecture lives in `design.md` and refers to `architecture.md` by decision ID.
- Every proposal starts with the step number and title, lists its feature and decision IDs, and says which demo act it serves.
- **Every scenario ends with a test line**, `Test: <path>` naming the test that will cover it:
  - Go tests: `services/internal/<area>/..._test.go` or `services/cmd/<service>/..._test.go`
  - Web unit tests: `apps/web/tests/unit/<name>.test.ts`
  - Playwright: `apps/web/tests/e2e/<name>.spec.ts`
  - k6: `load/<name>.js`

  Until the code exists the path is the planned test file. Thin features may use `Test: manual check on the deployed preview (<what to look at>)`.
- **Depth is honoured:** Real features get full requirements and edge cases; Thin features get a short requirement for the clickable screen and the simulated logic; Vision items appear only on the roadmap page (step 12).
- **Task lists** follow the CLAUDE.md loop and end with the step's checks: local tests, the PR, the three reviewers, Playwright on the deployed preview, merge, the production smoke test, archive.

## Repository layout (from step 0)
```
apps/web/            Next.js: EPS Studio, storefronts, fan screens, API routes for the control plane
services/            Go module: cmd/gateway, cmd/core, cmd/worker, cmd/fakepsp, cmd/simulator; internal/...
api/openapi.yaml     the contract between Next.js and Go
deploy/helm/eps/     umbrella chart with one sub-chart per Go service, plus Redis
load/                k6 scripts
openspec/            specs and changes
docs/                planning, research, deliverables
```

## Cross-step interfaces (agreed while writing the changes)
- **Core address per environment:** env slug `prod` for `main`, else `<body>-<6-char hash>`: the branch with non-ASCII-alphanumerics turned into `-` before lowercasing, cut to 40 characters, `br-` in front of a reserved body (prod, main, staging, eps, kube-*). The hash is always added, so no branch maps to `prod` or to another branch's slug. Core at `https://core.<slug>.<lb-ip>.sslip.io`, gateway at `gw.<slug>...`. The web app derives it from `VERCEL_GIT_COMMIT_REF` unless `CORE_API_URL` is set; `deploy/scripts/preview-names.sh` derives every other per-PR name from the slug.
- **Database:**
  - Roles `eps_web` and `eps_core` (NOLOGIN, no RLS bypass). The apps connect as the login roles `eps_web_login` / `eps_core_login` (LOGIN, NOBYPASSRLS, members of their role only, own nothing); the branch owner (`neondb_owner`) runs migrations only. CI sets the login passwords (`cmd/migrate -login`).
  - Each transaction runs `SET LOCAL ROLE` and `set_config('app.organiser_id')`. goose migrations with one version table per schema, forward-only and backward compatible (expand, then contract a release later); readiness requires each schema at **at least** the expected version.
  - A schema test fails on any table without `organiser_id`, except a platform allowlist.
  - **SECURITY DEFINER rule:** any SECURITY DEFINER function (step 3's `control.live_snapshot()` first) is owned by a dedicated NOLOGIN NOBYPASSRLS role created for it, never by `neondb_owner` (which bypasses RLS on Neon), and sets `search_path` explicitly.
  - Previews use a CI-made Neon branch `eps-preview/<slug>`, a child of the empty seed branch (schema-only, never production data), recreated per push; the web app reads `EPS_DATABASE_URL(_UNPOOLED)`, never the Neon–Vercel integration's `DATABASE_URL` on previews.
- **Core token claims:** `iss=urn:eps:web:<env slug>` (the Go core of that environment accepts only that issuer, `CORE_TOKEN_ISSUERS`), `aud=eps-core`, `kind` (organiser, fan or guest), `org`, `role` (owner, admin or staff), `sid`, 5-minute life. Guests use `sub=guest:<id>`. Simulator guests use a rehearsal-only issuer accepted only on rehearsal sales (step 6).
- **Redis keys:** `eps:{<org>}:<event|_>:<kind>:...` for organiser data (hash tag on the organiser). **Hot on-sale keys** (holds, stock counters, queues; steps 4 and 5) use `redisx.HotKey`: `eps:{<org>:<event>:<shard>}:<kind>:...`, tagged on organiser, event and shard so one big sale spreads over slots. Live snapshot at `eps:{org}:<event>:snapshot:live`.
- **Money:**
  - Integer minor units everywhere.
  - Platform fee 2.5%; fee policies absorb, pass on or split, with the fan's half rounded down in a split.
  - Pricing functions live in `apps/web/src/lib/pricing/`, with shared test vectors in `vectors.json` that the Go code must also pass.
  - Instalments: a plan's monthly count is a maximum, capped per buyer from the purchase date (due dates and the "last payment ≥ 7 days before the event" rule in the event's local dates; `maxMonthlyPayments`). Deposit is `deposit_minor` or `deposit_bps`.
- **Buying caps:** step 1's `max_per_order` (product) and `max_per_account` (event) are the defaults of a sale; step 5's sale settings may set their own, and the stricter of the two always applies.
- **Core token roles:** the `role` claim is the exact membership role (`owner`, `admin`, `box_office`, `finance`, or step 0's `staff`); the Go verifier accepts exactly these.
- **Sale snapshots:**
  - Stored immutable in `control.sale_snapshots`, in the shape of `api/schemas/sale-snapshot.v1.json`.
  - Each price step carries precomputed `allInMinor`, `fanFeeMinor` and `organiserNetMinor`.
  - Go reads them only through `control.live_snapshot(event, version)`: the live version or an earlier one, never one above the live version and nothing for an event that is off sale (`events.live_snapshot_version` null: a publish of its shop that no longer sells it). Public shop pages read `control.public_snapshot(event)`: no organiser net amounts, no fee settings, no hidden products.
  - `rules` carry `holdMinutes` (5 to 15, default 8, from `events.hold_minutes`) and `checkoutQuestions` (`{afterContact, consents}` slot ids from the selling shop's storefront document) for step 4's checkout; step 4 clamps holds to the live snapshot's capacities and re-prices every order from snapshot `v`.
  - The core signs every copy it keeps in Redis (snapshots, "missing" marks, availability answers) with `SNAPSHOT_CACHE_KEY`; Redis requires `REDIS_PASSWORD`. Both come from the per-environment Kubernetes secret `eps-runtime` (`deploy/scripts/runtime-secret.sh`).
  - Pool instance ids are `<poolId>[@date[Ttime]]`. The date is the event's local calendar date: for a day pool the event day; for a slot pool the local date the slot **starts** on, so a slot after midnight (01:00 on the night of 1 Jul) is `<poolId>@<2 Jul>T01:00`. One time grid per slot pool (step 1 readiness rule).
  - Pool scopes: passes, flexible days and add-ons draw on event pools; day pools serve single-day products; slot pools serve time slots only.
- **Storefront addresses (step 2 review):** home `/s/<brand>/<locale>`, event page `/s/<brand>/<locale>/e/<event-slug>` by the EVENT's slug (event pages store no slug), info page `/s/<brand>/<locale>/<page-slug>`. Reserved page slugs, refused by the document schema: `checkout`, `e`, `p`, `assets`, `api`, `s`.
- **Shop renderer contract (step 2 review):** `ShopEvent` in `apps/web/src/lib/storefront/catalogue.ts` is the one type catalogue sections render. Step 2 builds it from the draft catalogue (builder, preview links); step 3 builds it from the live snapshot, which therefore carries the event `slug`, `venue` (name, address, coordinates), `lineup` and `days` as `{date, opens, closes}`. Venue maps are static tiles the server stores in `control.map_tiles` (`/assets/maps/<z>/<x>/<y>.png`); a fan's browser never calls a map service.
- **Shop caching (step 3 review):** `/s/<brand>/<locale>/…` pages are static (their own root layout, no cookies or headers at render), cached at the edge until the brand's next publish (tags `shop:slug:<slug>`, `shop:host:<host>`, `brand:<id>`, `event:<id>`), with Subresource Integrity on scripts and a fixed shop CSP. Anything that depends on the time or on stock is worked out in the browser from the core's availability (`/api/public/events/<id>/availability`, our own origin, CDN 2 s). The availability answer lists per-instance (`instances[]`) only pool instances that are sold out or have few left; an answer for another snapshot version than the page's is not mixed with the page's prices.
- **Money display:** `formatMoney` (`lib/pricing`) is `Intl.NumberFormat` per locale with the currency code: `ro-RO` → `552,48 RON`, `en-GB` → `RON 552.48` (no-break space).
- **Page security headers:** `X-Frame-Options: DENY` and `frame-ancestors 'none'` everywhere except the builder frame (`SAMEORIGIN` / `'self'`); Studio, preview links and `/s/` shops carry a nonce-based CSP (`src/lib/security-headers.ts`).
- **Checkout:**
  - Entry route `/s/<brand>/<locale>/checkout?event=&v=&items=productId:qty[:instance]`.
  - Every order records the snapshot `(event, version)`.
  - Default hold is 8 minutes, configurable from 5 to 15.
  - Checkout slots are `checkout.slots.afterContact` and `checkout.slots.consents`.
- **Sale settings** live in the sales schema (owned by Go) and are edited from Studio through the OpenAPI contract, so the admission rate changes live.
- **Holds:** step 4 uses one counter per pool; step 5 swaps in sharded counters behind the same interface and re-runs step 4's hold tests.
- **Issuing:** step 4 marks orders issued with a stub driven by `order.paid`; step 7 replaces it with real ticket issuing.
- **Live dashboard:** the browser connects to the Go gateway's server-sent events directly, not through Vercel functions. The servers have no global write timeout; streams are exempt from the per-request 30 s write deadline (`Accept: text/event-stream`).
- **CORS:** exact origins only (production web origin, plus a preview's own Vercel URLs injected by CI); never a wildcard.
- **Metrics:** every Go binary serves Prometheus text on `:9090/metrics` (outside the ingress); step 6 builds on the same names.
