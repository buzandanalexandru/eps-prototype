# Step 11: AI

**Demo act:** Act 1 (the organiser types one sentence and gets a draft event, tiers, prices and a themed shop, then edits and publishes it in the builder), with Act 3 (the fan concierge fills a cart) and the vision (EPS events sold through AI assistants over MCP).
**Features:** H1 (Real), H2, H3 (Thin).
**Decisions:** U4 (AI writes drafts only), U3 (theme JSON and schemas), P1 (published snapshots are the only sale data AI reads), U2 (organiser isolation), S3.

## Why

Ticketmaster already sells inside ChatGPT, Alexa+ and Google AI Mode, and Fever runs an MCP server and a Gemini app (`competitors/ticketmaster.md`, `competitors/fan-first.md`); Eventbrite writes event copy with AI. Nobody in Romania does any of this, and EPS has none (`feature-matrix.md`, S6). An open platform for every organiser has to make setup take minutes, not a sales call; an AI copilot that drafts the whole shop from a brief is the clearest "next-gen" moment for Act 1, and the agent-ready distribution is the clearest vision item.

## What Changes

- **Organiser AI copilot (H1, Real):** in EPS Studio the organiser writes a brief in Romanian or English; Claude, called from the Next.js server with tools, returns a draft event, products, capacity pools, price ladders and a themed storefront, each validated by the catalogue's and the builder's own schemas. The result is saved as a draft labelled "AI draft" that the organiser edits in the builder. It never publishes.
- **Fan AI concierge (H2, Thin):** a chat panel on a storefront turns a request ("weekend for 4, hip-hop, 300 RON each") into a suggested cart from that shop's published products, with all-in prices from the snapshot. The fan confirms; the concierge never holds, buys or pays.
- **MCP server for AI assistants (H3, Thin):** a public, read-only MCP endpoint with event search, event details and cart links, so assistants such as Claude, ChatGPT or Gemini can find EPS events and hand the fan a link to the organiser's own shop.

## Capabilities

### New Capabilities
- `ai-copilot`: brief to validated draft event, products, prices and themed storefront; drafts only.
- `ai-concierge`: fan request to a suggested cart the fan confirms.
- `ai-agent-access`: the MCP server exposing event search, details and cart links to AI assistants.

### Modified Capabilities
None.

## Impact

- **apps/web:** Studio copilot screen and API route, storefront concierge panel and API route, the `/api/mcp` endpoint, a model-client module with a fake for tests, prompt and tool definitions.
- **Dependencies:** `@anthropic-ai/sdk` (the Claude API, named in the stack through `ANTHROPIC_API_KEY`) and `@modelcontextprotocol/sdk` for the MCP endpoint; the second is outside the stack list, so it is confirmed with the owner before it is added (CLAUDE.md).
- **Secrets:** `ANTHROPIC_API_KEY` only as a Vercel environment variable; tests and CI use the fake model client and never call the API.
- **Go services:** none changed; the concierge and MCP read published snapshots and the existing availability endpoint.
