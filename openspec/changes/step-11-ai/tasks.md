# Tasks

## 1. Model client and settings

- [ ] 1.1 Re-check the current Claude model IDs and request rules in the claude-api reference and record the chosen model in one config constant; verify the constant matches the reference's current default
- [ ] 1.2 Add `@anthropic-ai/sdk`; write the `ModelClient` wrapper (streaming with `finalMessage()`, adaptive thinking, per-feature effort, `tool_choice: auto`, strict tools, server-side refusal fallback, typed error handling, timeouts) reading `ANTHROPIC_API_KEY` from the server only; verify a type check and that the module cannot be imported from a client component
- [ ] 1.3 Write `FakeModelClient` and the fixtures (valid draft, invalid-then-repaired, refusal, timeout, injection brief, concierge carts); verify `apps/web/tests/unit/ai-model-client.test.ts` passes with no key set and fails if the real client is built in tests
- [ ] 1.4 Confirm `ANTHROPIC_API_KEY` exists in the Vercel project for preview and production without reading its value (ask the owner if missing); verify with the Vercel env listing

## 2. Organiser copilot (H1)

- [ ] 2.1 Generate strict JSON schemas for `propose_event`, `propose_products`, `propose_storefront` from the step 1 and step 2 zod schemas, and write the read-only `list_templates` and `get_organiser_context` tools scoped to the caller's organiser; verify schema snapshot tests
- [ ] 2.2 Copilot loop: validate each proposal (schemas, cross-checks, contrast), return `is_error` results for repair, stop after 3 rounds, save all parts in one transaction as drafts marked `source=ai`; verify the repair, give-up, contrast, fee-policy and injection scenarios in `apps/web/tests/unit/ai-copilot.test.ts`
- [ ] 2.3 Limits and failures: brief length cap, 10 runs per organiser per hour, refusal and outage messages, usage logging without brief text; verify the limit and unavailable scenarios in `apps/web/tests/unit/ai-copilot.test.ts`
- [ ] 2.4 Studio copilot screen with streamed progress (event, tickets, shop), the AI draft banner and "Open in builder"; publish stays the builder's human action; verify `apps/web/tests/e2e/ai-copilot.spec.ts` with the fake client
- [ ] 2.5 Run the rehearsed demo brief once against the real API on the deployed preview and save the produced draft as a fixture; verify it opens in the builder and publishes by hand

## 3. Fan concierge (H2)

- [ ] 3.1 Concierge route with `search_products` and `suggest_cart` over the shop's published snapshot, server-side checks (existence, caps, availability without holds) and snapshot prices; verify `apps/web/tests/unit/ai-concierge.test.ts`
- [ ] 3.2 Storefront panel (RO and EN, AI suggestion label, Add to cart into the normal cart, no stored history); verify `apps/web/tests/e2e/ai-concierge.spec.ts` with the fake client

## 4. MCP server (H3)

- [ ] 4.1 Confirm the `@modelcontextprotocol/sdk` dependency with the owner (fallback: hand-written JSON-RPC subset); verify the decision is recorded in the PR
- [ ] 4.2 `/api/mcp` with `search_events`, `get_event` (availability bands) and `create_cart_link` (signed 30-minute token, `ref=mcp`), rate limit 60 per minute per address; verify `apps/web/tests/unit/mcp-server.test.ts`
- [ ] 4.3 Storefront reads the cart token and pre-fills the cart; verify `apps/web/tests/e2e/mcp-cart-link.spec.ts`
- [ ] 4.4 Connect a real MCP client (Claude desktop or the MCP inspector) to the deployed preview and search for the sample events; verify a working cart link and note the steps in the PR for the video

## 5. Delivery loop (CLAUDE.md)

- [ ] 5.1 Run the step's Vitest and Playwright tests locally, then the full suite once; verify all green with no API key in the environment
- [ ] 5.2 Open the PR `step-11-ai` to `main`; verify CI is green and the preview is deployed
- [ ] 5.3 Run the three adversarial reviewers in parallel (correctness: repair loop, partial drafts, cart checks; security and data: key handling, prompt injection, organiser isolation, no publish or buy path, MCP data exposure and abuse; spec and scale: every scenario tested, cost and rate limits, behaviour when the API is slow or down); verify each returns a ranked findings list
- [ ] 5.4 Fix every confirmed finding or record the rejection in the PR, then re-run the affected tests; verify green
- [ ] 5.5 Run Playwright against the deployed preview (desktop and phone, RO and EN for the concierge panel, no console errors, no horizontal scroll) plus an MCP tools/list check against the deployed endpoint; verify all pass
- [ ] 5.6 Squash-merge once CI, reviews and deployed tests are green; verify production with the Playwright smoke test (copilot screen loads, concierge panel opens, `/api/mcp` lists tools)

## Workflow follow-up

- Archive with `/opsx:archive step-11-ai`: merge the deltas into `openspec/specs/`, add `## Status` to the proposal, tick step 11 in `docs/planning/build-plan.md`.
