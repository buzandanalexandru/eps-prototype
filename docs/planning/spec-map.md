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
- **Core address per environment:** env slug `prod` for `main`, else the branch name (lowercase, at most 40 characters plus a 6-character hash). Core at `https://core.<slug>.<lb-ip>.sslip.io`, gateway at `gw.<slug>...`. The web app derives it from `VERCEL_GIT_COMMIT_REF` unless `CORE_API_URL` is set.
- **Database:** roles `eps_web` and `eps_core` (no RLS bypass). Each transaction runs `SET LOCAL ROLE` and `set_config('app.organiser_id')`. goose migrations with one version table per schema. A schema test fails on any table without `organiser_id`, except a platform allowlist.
- **Core token claims:** `aud=eps-core`, `kind` (organiser, fan or guest), `org`, `role`, `sid`, 5-minute life. Guests use `sub=guest:<id>`. Simulator guests use a rehearsal-only issuer accepted only on rehearsal sales (step 6).
- **Redis keys:** `eps:{<org>}:<event|_>:<kind>:...`. Live snapshot at `eps:{org}:<event>:snapshot:live`.
- **Money:**
  - Integer minor units everywhere.
  - Platform fee 2.5%; fee policies absorb, pass on or split, with the fan's half rounded down in a split.
  - Pricing functions live in `apps/web/src/lib/pricing/`, with shared test vectors in `vectors.json` that the Go code must also pass.
- **Sale snapshots:**
  - Stored immutable in `control.sale_snapshots`, in the shape of `api/schemas/sale-snapshot.v1.json`.
  - Each price step carries precomputed `allInMinor`, `fanFeeMinor` and `organiserNetMinor`.
  - Go reads them only through `control.live_snapshot()`.
  - Pool instance ids are `<poolId>[@date[Ttime]]`.
- **Checkout:**
  - Entry route `/s/<brand>/<locale>/checkout?event=&v=&items=productId:qty[:instance]`.
  - Every order records the snapshot `(event, version)`.
  - Default hold is 8 minutes, configurable from 5 to 15.
  - Checkout slots are `checkout.slots.afterContact` and `checkout.slots.consents`.
- **Sale settings** live in the sales schema (owned by Go) and are edited from Studio through the OpenAPI contract, so the admission rate changes live.
- **Holds:** step 4 uses one counter per pool; step 5 swaps in sharded counters behind the same interface and re-runs step 4's hold tests.
- **Issuing:** step 4 marks orders issued with a stub driven by `order.paid`; step 7 replaces it with real ticket issuing.
- **Live dashboard:** the browser connects to the Go gateway's server-sent events directly, not through Vercel functions.
