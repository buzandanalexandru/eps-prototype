# Tasks

## 1. Document schema and storage

- [x] 1.1 Create branch `step-2-builder`; add the control migration for `storefronts`, `storefront_versions` (one-draft partial unique index, immutability trigger), `preview_links`, `assets`, all with forced RLS; verify `services/internal/tenancy/schema_test.go` and `apps/web/tests/unit/storefront-isolation.test.ts`
- [x] 1.2 Write the v1 document schema and section registry (`hero`, `lineup`, `ticket_grid`, `calendar`, `map`, `faq`, `gallery`, `countdown`, `text`, locked `footer`), limits, link and image rules, JSON Schema export; verify `apps/web/tests/unit/storefront-schema.test.ts` (unknown keys and types, size, footer, limits, dangerous links, checkout keys)
- [x] 1.3 Implement asset upload with content sniffing (PNG, JPEG, WebP, ≤ 1 MB) and `/assets/<id>` with immutable caching; verify `apps/web/tests/unit/storefront-assets.test.ts`

## 2. Tokens, contrast and templates

- [x] 2.1 Implement tokens → CSS variables and the curated font list via `next/font`; verify `apps/web/tests/unit/storefront-tokens.test.tsx` (every section reads only variables)
- [x] 2.2 Implement the contrast module (WCAG ratios, OKLCH lightness fix, light and dark checks) and palette proposal from a logo; verify `apps/web/tests/unit/storefront-contrast.test.ts`
- [x] 2.3 Write the festival, resort, club night and theatre templates; verify `apps/web/tests/unit/storefront-templates.test.ts` (valid, contrast passes, publish checks pass)

## 3. Renderer

- [x] 3.1 Build the shared section renderer in `apps/web/src/components/storefront/` (server components; countdown and gallery hydrate), RO/EN platform labels, escaped Markdown subset; verify `apps/web/tests/unit/storefront-render-safety.test.tsx`
- [x] 3.2 Bind ticket grid, line-up and calendar to catalogue data with all-in prices from `lib/pricing`; verify `apps/web/tests/unit/storefront-catalogue-binding.test.ts` (live price, foreign event refused)

## 4. Editor and live preview

- [x] 4.1 Build the builder layout (tree, frame, settings form generated from schemas; tabs under 1024 px), add/remove/hide, drag and keyboard reorder, change template with confirmation; verify `apps/web/tests/e2e/builder.spec.ts` locally
- [x] 4.2 Build the frame route and the postMessage protocol (`eps:draft`, `eps:rendered`, `eps:select`, origin and source checks, seq ordering), device toggle and language switch; verify `apps/web/tests/unit/preview-frame.test.ts` and `apps/web/tests/e2e/builder-preview.spec.ts`
- [x] 4.3 Implement autosave with revision conflict (409 and reload prompt); verify `apps/web/tests/unit/storefront-versions.test.ts` (draft separate from live, two tabs)
- [x] 4.4 Build the brand kit panel (logo, colours, fonts, radius, mode, inline contrast results and accept suggestion) and the locked Checkout tab; verify the brand kit and checkout scenarios in `builder.spec.ts`

## 5. Versions and links

- [x] 5.1 Implement publish (validation, contrast, translations, ticket grid on event pages; one transaction; idempotent per draft revision) with a hook point for step 3; verify `apps/web/tests/unit/storefront-publish.test.ts`
- [x] 5.2 Implement history and restore-as-new-version; verify the rollback scenario in `storefront-versions.test.ts`
- [x] 5.3 Implement preview links (create, list, revoke, 7-day expiry, hashed tokens) and `/p/<token>` (banner, noindex, no-store, buying disabled); verify `apps/web/tests/unit/preview-links.test.ts`
- [x] 5.4 Show the brand address with copy and live state; verify `apps/web/tests/e2e/builder-publish.spec.ts` (first publish, preview link signed out, address state)

## 6. Messages and seed

- [x] 6.1 Build the email and ticket previews with the brand kit and a sample order; verify with a Vitest render test and the manual preview check
- [x] 6.2 Extend the step 1 seed so both NIBIRU brands and Sunwave have published storefronts (festival, resort and club night templates with sample images); verify the seed test still passes twice and each storefront validates

## 7. Step checks (CLAUDE.md loop)

- [x] 7.1 Run the step's tests locally, then the full suite once; verify all pass
- [x] 7.2 Run `npx openspec validate step-2-builder --strict`; verify it passes
- [x] 7.3 Open the PR to `main`; verify CI is green and the preview is deployed and seeded
- [x] 7.4 Run the three adversarial reviewers in parallel (correctness; security and data, including injection through documents and postMessage; spec and scale), fix every confirmed finding or record its rejection, re-run affected tests; verify no open confirmed finding
- [x] 7.5 Run Playwright against the deployed preview URL (builder, live preview, publish, preview link, rollback; desktop and phone, RO and EN, no console errors, no horizontal scroll); verify all pass
  - Re-run after the review fixes on 8ec651a (preview run 27, deployed URL): 44 passed, 14 skipped, 0 failed. Desktop ran all six builder tests (first visit, template change keeping colours, fonts, radius and logo, logo upload and contrast fix, reorder/keyboard/hide, two-tab conflict, live title within 1 s, preview language following the field, publish v1 → v2 → restore v3 with the warning, preview link signed out with banner, no buying, own-origin images, privacy line, DENY framing, revoke → 404). Phone ran only the builder test that creates no data (390 px frame, no horizontal scroll, stacked ticket grid, Romanian prices and labels, click-to-select, locked checkout); the five data-creating builder tests are desktop-only, as in step 1. RO is checked in the frame and in unit render tests; Studio screens in RO/EN are covered by sample-data.spec.
- [x] 7.6 Squash-merge when green; verify the production deploy and the Playwright smoke test pass

## Workflow follow-up

- Archive with `/opsx:archive`: merge the deltas into `openspec/specs/`, add `## Status` to the proposal and tick step 2 in `docs/planning/build-plan.md`.
- Verify the archived specs validate.
