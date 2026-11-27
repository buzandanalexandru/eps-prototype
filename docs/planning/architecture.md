# Architecture

Agreed in the planning session on 27 Nov 2026 (OpenSpec explore mode). Every decision below was accepted on the EPS Architecture Review page.
Scope is in `feature-scope.md`. Build steps are in `build-plan.md`.

## Hosting and fixed choices

- **Control plane:** Next.js (TypeScript) on Vercel, Frankfurt region: EPS Studio (builder, catalogue, sale settings, dashboards), storefronts, fan account, wallet, squad and AI.
- **Data plane:** Go on-sale core on DigitalOcean Kubernetes (DOKS), Frankfurt: gateway, core API, worker, fake payment providers, simulator.
- **Redis** runs inside the cluster (queue, stock shards, rate limits, streams).
- **Postgres** is on Neon, Frankfurt: one database, a control schema owned by Next.js and a sales schema owned by Go.
- **Addresses:** shops live at `/s/<brand>` because there is no demo domain. Routing also reads the host, so a custom domain later is configuration.
- **Load story:** a live in-system simulator for the 100k on-sale, plus measured k6 throughput per instance and the scaling maths in the vision page. No claim of a real 100k-client test.
- **No real payments:** every payment path, including Apple Pay, Google Pay and saved cards, runs through simulated providers behind a real network boundary.
- **Data stays in the EU** (GDPR): all hosting in Frankfurt.
- **Accounts** (DigitalOcean, Neon, Vercel, Anthropic) are ready before the build. Keys are entered through secure forms, never in chat or in the repo.

## System sketch

```
 Fans / organisers
        |
        v
 +--------------------------- Vercel (fra1) ----------------------------+
 | Next.js: EPS Studio (builder, catalogue, sale settings, dashboards)  |
 |          storefronts /s/<brand>, fan account, wallet, squad, AI     |
 +-----------+----------------------------------------------+-----------+
             | publish = versioned sale snapshot            | HTTPS + JWT
             v                                              v
 +---- Neon Postgres ----+        +------------- DOKS Frankfurt -----------------+
 | control schema        |<------>| gateway (queue, passes)    HPA               |
 | sales schema (orders) |        | core API (holds, checkout, payments) HPA     |
 | outbox                |        | worker (issuing, webhooks, reconcile)        |
 +-----------------------+        | fake-psp A / B (knobs: fail %, latency)      |
                                  | simulator job (virtual fans + bots)          |
                                  | Redis (queue, stock shards, rate, streams)   |
                                  +----------------------------------------------+
```

## Structure

How the code and services are laid out.

### S1 · One monorepo

**Decision:** One repository with apps/web (Next.js), services/ (Go), deploy/helm, openspec/ and docs/.

**Alternatives considered:** Separate repos per service.

**Why:** One place to review; OpenSpec changes can span both halves in one commit.

### S2 · One Go module, several deployables

**Decision:** A single Go module that builds separate binaries: gateway, core API, worker, fake payment provider, simulator. Each runs as its own Kubernetes deployment and scales on its own.

**Alternatives considered:** One Go binary for everything; or fully separate microservices with their own modules.

**Why:** Independent scaling where load differs (the gateway takes the crowd), without the overhead of many repos.

### S3 · HTTP + JSON with an OpenAPI contract

**Decision:** Next.js talks to Go over HTTP/JSON described by one OpenAPI file; TypeScript types and the Go server stubs are generated from it.

**Alternatives considered:** gRPC or Connect; or hand-written types on both sides.

**Why:** Easy to debug and demo with curl; types stay in sync; works through Vercel functions.

## Control plane to data plane

How what the organiser builds reaches the on-sale engine.

### P1 · Publish compiles a sale snapshot

**Decision:** Publishing in the builder compiles products, pools, prices and rules into an immutable, versioned sale snapshot (Postgres, cached in Redis). The Go core reads only snapshots, never the organiser's draft tables. Every order records the snapshot version it was sold under.

**Alternatives considered:** Go reads the control tables directly.

**Why:** The organiser can keep editing drafts during a live sale without touching it; clean audit of what was sold at which price.

### P2 · Shared Postgres, two schemas

**Decision:** One Neon database with a control schema (owned by Next.js) and a sales schema (owned by Go). Each side writes only its own schema.

**Alternatives considered:** Two databases; or one schema shared by both.

**Why:** Clear ownership without running two databases in 48 hours; easy to split later.

### P3 · Events through an outbox and Redis Streams

**Decision:** Changes that others must react to (order paid, ticket issued, snapshot published) are written to an outbox table in the same transaction, then relayed to Redis Streams. The worker, dashboards and webhooks read from there.

**Alternatives considered:** NATS JetStream; Kafka; direct calls.

**Why:** No lost events, no extra component (Redis is already there); NATS or Kafka named as the scale-up path in the vision.

## On-sale engine

Queue, stock, payments and tickets under load.

### O1 · Queue: pre-queue, shuffle, rate

**Decision:** Arrivals before opening join a pre-queue in Redis. At opening each gets a random position; later arrivals go behind in order. A token bucket admits fans at the organiser's rate and gives each a signed pass (10-minute life, bound to the session). Checkout rejects requests without a valid pass.

**Alternatives considered:** Plain first-come queue; a vendor such as Queue-it.

**Why:** Fair, explainable, the same pattern as Queue-it and Ticketmaster, and built in so it is part of the platform.

### O2 · Stock: sharded counters with timed holds

**Decision:** Each capacity pool's stock is split across several Redis counters. A hold takes stock atomically with a small Lua script and expires on its own. Postgres records the sale when payment succeeds; a reconciler fixes any drift.

**Alternatives considered:** Row locks in Postgres; one Redis counter per pool.

**Why:** No single hot key at 100k fans; holds can't oversell; Postgres stays the record of truth.

### O3 · Payments: state machine and failover

**Decision:** Orders and payment attempts move through explicit states in Postgres. Every attempt has an idempotency key. Each provider sits behind one adapter interface with a circuit breaker; when provider A fails too often, new attempts go to B. Webhooks confirm payments; a status endpoint shows the truth.

**Alternatives considered:** Synchronous charge without states; a single provider.

**Why:** Exactly the gaps found in EPS (no declined state, blind success page); adapters make real providers (Stripe, Adyen, BT iPay, Netopia) a later swap.

### O4 · Fake payment providers

**Decision:** Two fake providers run as a small Go service with webhooks and live knobs: failure rate, latency, outage on or off, plus Apple Pay / Google Pay / saved-card paths and the OTP challenge.

**Alternatives considered:** Mock inside the core API.

**Why:** A real network boundary makes the failover genuine in the demo.

### O5 · Rotating QR with signed tokens

**Decision:** Each ticket has a signed token. The QR changes every 15 seconds: it encodes the ticket ID and the current time window, signed with Ed25519. The door page checks it offline with the public key, so screenshots and old codes fail.

**Alternatives considered:** A static QR; an HMAC secret on every scanner.

**Why:** Offline scanning without sharing a secret with every phone; the screenshot-rejected moment in the video.

### O6 · Wallet as a ledger

**Decision:** The EPS Wallet is a double-entry ledger in Postgres (top-ups, payments, refunds, season credit). Paying from the wallet during a drop is one atomic ledger move, so it is the fastest payment path.

**Alternatives considered:** A balance field on the user.

**Why:** Correct money handling is what interviewers probe; the ledger also shows the e-money partner route.

### O7 · Squad buy as a group hold

**Decision:** The squad leader takes a group hold for N tickets. Each friend gets a link and pays their share (or first instalment). Unpaid shares release when the hold expires, or the leader covers them.

**Alternatives considered:** Leader pays all, friends repay outside EPS.

**Why:** Real 'each pays their own' is the feature nobody has; it reuses holds and the payment state machine.

## Users and organisers

Sign-in, separation between organisers, the builder.

### U1 · Sign-in in Next.js, JWTs for Go

**Decision:** Sign-in runs in Next.js (email link and Google) with Better Auth. Next.js issues short-lived signed tokens that Go verifies with a public key. Guests can buy without an account and claim tickets later.

**Alternatives considered:** Auth in Go; a hosted provider (Clerk, Auth0).

**Why:** One sign-in for both halves; Go stays stateless and fast.

### U2 · Organiser separation in the database

**Decision:** Every row carries the organiser ID, and Postgres row-level security enforces it; Redis keys are prefixed by organiser and event.

**Alternatives considered:** Separation only in application code; a database per organiser.

**Why:** Mistakes in code can't leak another organiser's data; one database stays simple.

### U3 · Builder: themes as versioned JSON

**Decision:** A storefront is a JSON document of pages, sections, blocks and design tokens, saved as versions (draft, published, older). The editor shows a live preview in a frame that updates on every change; server components render the published version and cache it until the next publish.

**Alternatives considered:** Free drag-and-drop canvas; HTML templates.

**Why:** Safe and fast to render, easy rollback, and the AI copilot can produce the same JSON.

### U4 · AI copilot writes drafts only

**Decision:** The copilot calls Claude from the Next.js server with tools that return a draft event, products and theme JSON, checked against the same schemas as the builder. It never publishes; the organiser reviews and publishes. The fan concierge only builds a cart for the fan to confirm.

**Alternatives considered:** Free-form AI that edits live data.

**Why:** Keeps AI useful and safe, and matches the 'drafts, never acts alone' rule.

### U5 · Interface kit and languages

**Decision:** Tailwind CSS with shadcn/ui components for EPS Studio; storefronts use the brand tokens. Romanian and English through next-intl.

**Alternatives considered:** Another component library; English only.

**Why:** Fast to build a polished studio; RO and EN match NIBIRU.

## Running it

Deploy, observe, test, simulate.

### R1 · Helm, autoscaling, CI

**Decision:** One Helm chart with a sub-chart per Go service. Horizontal autoscaling on CPU and requests per second for the gateway and core API. GitHub Actions builds images to GitHub's container registry and runs helm upgrade; Vercel deploys from Git.

**Alternatives considered:** Plain manifests; Argo CD.

**Why:** Standard, repeatable, and shows autoscaling during the simulated on-sale.

### R2 · Metrics and the live dashboard

**Decision:** Go services expose Prometheus metrics. A small aggregator streams the on-sale numbers (queue, admitted per minute, sold, failures, failovers, bots, stock) to the dashboard over server-sent events.

**Alternatives considered:** Full Grafana stack; polling.

**Why:** The dashboard is a product screen, not an ops tool; Prometheus is there for the K8s story.

### R3 · Simulator sends real traffic

**Decision:** The simulator runs as a Kubernetes job that drives virtual fans and bots through the real gateway and checkout over HTTP, with knobs for fan count, bot share and provider outage. It does not fake the counters.

**Alternatives considered:** Counters animated in the browser.

**Why:** The dashboard shows the real system under load, so the demo is honest.

### R4 · Tests

**Decision:** Go unit and integration tests against real Postgres and Redis (testcontainers). Vitest for Next.js logic, Playwright for the main flows (build and publish, buy with failover, squad buy, scan). k6 scripts for the throughput numbers.

**Alternatives considered:** Fewer tests to save time.

**Why:** The video's claims are backed by tests; OpenSpec scenarios each name the test that covers them.
