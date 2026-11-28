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

## Status

Built and merged as PR #4 (squash c4bba1f) on 28 Nov 2026.

**What was built:**
- A brand kit that turns into design tokens, with WCAG contrast checks. Primary and accent colours are never used as small text, and hero text is checked against the image overlay.
- Templates: festival, resort, club night and theatre.
- Pages made of sections and blocks, a live preview (desktop and phone, RO and EN, following the language of the UI and of the field being edited), drafts with revision guards on save, restore and publish, an autosave race guard, preview links (capped, with "revoke all"), publish and rollback, and the locked checkout slots.
- Email and ticket design.
- Uploads with quotas, deduplication and a rate limit. An image is public only when a live version or a valid preview link uses it.
- OpenStreetMap tiles fetched by the server and served from our own origin, with attribution and a privacy notice.
- Strict JSON Schemas per part, for the step-11 copilot.
- frame-ancestors and nonce-based CSP headers.
- Sample storefronts for NIBIRU and Sunwave.

**Deviations, recorded in design.md:**
- The logo is an image reference in tokens.
- `storefront_versions.source` was added for step 11.
- A theatre template was added.
- Only owners and admins can write.
- The preview frame is scaled.
- Email and ticket design is checked by a Vitest render test.
- A /terms page was added.
- Previews allow 150 demo sign-ins per minute.
- A platform table `control.map_tiles` was added.
- Snapshot contract with step 3: the snapshot carries `slug`, `venue`, `lineup` and `days {date, opens, closes}`, and `ShopEvent` is the shared type. Recorded in spec-map.
- Money uses Intl formatting.
- Event URLs are `/e/<event-slug>`, and reserved page slugs are refused.

**Known limitation:** on each push to a PR, the preview's database is briefly unreachable (about 40 s, measured) while Neon restores the branch. The page itself keeps returning 200. Production is unaffected.

**Checks run:**
- Vitest 277 tests with Postgres; tsc, eslint and next build pass.
- Go vet and tests, actionlint and `openspec validate --all --strict` pass.
- All 17 PR checks are green. The deployed e2e had 44 passed and 14 skipped. Builder tests ran in full on desktop; on phone only the builder test that creates no data runs (task 7.5 re-run after the review fixes).
- Independent Playwright check of the preview builder at 1440 px and 390 px: no console errors, no horizontal scroll. X-Frame-Options DENY and frame-ancestors 'none' were confirmed. A screenshot shows the live preview of the NIBIRU Festivals shop.
- Three adversarial reviews found: correctness, 2 medium and 5 low; security and privacy, 3 medium and 3 low; spec, demo and scale, 2 high, 4 medium and 5 low. All confirmed findings were fixed. The PR body has the table of findings and the commits that fixed them.
- Production deploy run 6 is green. The production seed (run 46098723354) is green. A production Playwright check of /studio/builder at 1440 px and 390 px with demo sign-in passed: NIBIRU sample data shown, no console errors, no horizontal scroll, and X-Frame-Options DENY sent.
