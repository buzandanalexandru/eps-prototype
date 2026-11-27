# Proposal

Step 2: Builder

**Demo act:** Act 1, the spine of the demo: the organiser turns a brand kit into a themed shop from a template, edits sections with a live preview on desktop and phone, shares a preview link, publishes and rolls back.
**Covers:** features B1, B2, B3, B4, B5, B6, B7, B10 (Thin), B12 (Thin); decision U3 (themes as versioned JSON, live preview in a frame, published version rendered by server components).

## Why

White label is the product: every organiser must be able to make the shared platform look like their own without code and without breaking checkout at peak load. Nobody in ticketing offers a Shopify-grade builder (`docs/research/competitors/whitelabel.md`, "Blueprint for the EPS builder"); a schema-driven document of pages, sections and design tokens, with drafts, preview links and rollback, is both the demo's opening moment and the format the AI copilot (step 11) will write.

## What Changes

- One storefront per brand, stored as a versioned JSON document (schema version 1): design tokens, languages, pages of typed sections and blocks, a locked checkout appearance and email and ticket design settings.
- Brand kit editor: logo, colours, two fonts from a curated list, corner radius, light, dark or automatic mode, turned into design tokens and CSS variables, with a WCAG contrast check that suggests a fix and blocks publishing.
- Four starting templates (festival, resort, club night, theatre); a brand's template choice from onboarding becomes its first draft.
- Page editor: add, remove, reorder, hide and configure sections (hero, line-up, ticket grid, calendar, map, FAQ, gallery, countdown, text, locked footer) and their blocks, with forms generated from each section's schema.
- Live preview in a frame updated by `postMessage` on every change, with a desktop / phone toggle and the catalogue's real products at all-in prices.
- Draft autosave with conflict detection, private expiring preview links, publish into immutable versions, version history and one-click rollback.
- Locked checkout: only tokens and named slots (questions and consents, filled by step 4), never free layout.
- Languages (RO, EN) and currency shown per shop; the brand's public address `/s/<brand>` shown in the builder; previews of the confirmation email and the ticket with the brand kit applied.

## Capabilities

### New Capabilities
- `storefront-builder`: brand kit and tokens, contrast check, templates, sections and blocks, live preview, drafts, preview links, versions, publishing and rollback, locked checkout, languages and currency, brand address, email and ticket design.

### Modified Capabilities
None.

## Impact

- New control-schema tables: `storefronts`, `storefront_versions`, `preview_links`, `assets` (all organiser-scoped with forced RLS).
- New code: `apps/web/src/lib/storefront/` (document schema, section registry, tokens, contrast, templates), the shared section renderer in `apps/web/src/components/storefront/` (reused by step 3's public shop), the builder at `/studio/builder`, the preview frame route and `/p/<token>` preview links.
- Interface for later steps: step 3 renders published versions at `/s/<brand>` and extends publishing to compile the sale snapshot; step 4 fills the checkout slots; step 11 produces documents against the same schema.
- No new dependencies beyond the stack: fonts are self-hosted through `next/font`; colour maths is a small local module.
