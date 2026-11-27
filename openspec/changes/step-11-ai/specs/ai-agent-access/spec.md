# Spec Delta

## Purpose

Lets AI assistants such as Claude, ChatGPT or Gemini find EPS events and send fans to the organiser's own shop, through a public read-only MCP server.

## ADDED Requirements

### Requirement: Read-only MCP tools
The system SHALL serve an MCP endpoint offering three read-only tools: search published events, get one event with its products and all-in prices, and create a cart link.

#### Scenario: Assistant lists the tools
- **WHEN** an MCP client connects and lists tools
- **THEN** it receives `search_events`, `get_event` and `create_cart_link` with their input schemas
- Test: apps/web/tests/unit/mcp-server.test.ts

#### Scenario: Search events
- **WHEN** an assistant calls `search_events` with a city and a date range
- **THEN** it receives only published, marketplace-listed events in that range, each with an all-in "from" price and its shop URL, and sample events marked as sample data
- Test: apps/web/tests/unit/mcp-server.test.ts

### Requirement: Cart links, never purchases
The system SHALL answer `create_cart_link` with a signed shop link that pre-fills the cart for 30 minutes and records the AI assistant as the sale source, and SHALL not create a hold or an order.

#### Scenario: Link opens a pre-filled cart
- **WHEN** a fan opens a cart link from an assistant within 30 minutes
- **THEN** the organiser's shop opens with the items in the cart at all-in prices, and no stock was held before the fan acted
- Test: apps/web/tests/e2e/mcp-cart-link.spec.ts

#### Scenario: Expired or altered link
- **WHEN** a cart link is older than 30 minutes or its token was changed
- **THEN** the shop opens with an empty cart and a short message
- Test: apps/web/tests/unit/mcp-server.test.ts

### Requirement: No private data and rate limits
The system SHALL expose no personal data, no exact stock counts and no draft data through the MCP endpoint, and SHALL limit each client address to 60 calls per minute.

#### Scenario: Availability without stock numbers
- **WHEN** an assistant calls `get_event`
- **THEN** each product shows available, low or sold out, never a count
- Test: apps/web/tests/unit/mcp-server.test.ts
