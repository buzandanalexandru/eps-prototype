# Design

## Context

Step 1 delivered organisers, brands (with a recorded template choice, languages and currency), the catalogue and `apps/web/src/lib/pricing/` (all-in prices). U3 (`docs/planning/architecture.md`) fixes the approach: a storefront is a versioned JSON document, the editor previews it live in a frame, and server components render the published version. The blueprint in `docs/research/competitors/whitelabel.md` ("Blueprint for the EPS builder", modules 1, 3, 4 and 5) is followed. The public storefront route and its caching are step 3; this step owns the document, the editor, the renderer and the version lifecycle.

## Goals / Non-Goals

**Goals:**
- One document schema that the editor, the renderer, the seed and the AI copilot all validate against.
- Organiser freedom on the storefront, none on checkout.
- Safe by construction: no organiser-supplied HTML or script reaches a fan's browser.

**Non-Goals:**
- Public `/s/<brand>` rendering, host routing and caching (step 3); sale snapshots (step 3); checkout questions and consents content (step 4, D9).
- Free drag-and-drop canvas, custom CSS, custom domains screen (B8), embed widget (B9), several shops per event (B11, step 10).

## Decisions

### D2.1 Data model
- `storefronts` (one per brand: `id`, `organiser_id`, `brand_id`, `live_version_id`).
- `storefront_versions` (`id`, `storefront_id`, `number`, `state` in `draft | published | archived`, `document` jsonb, `revision` for the draft, `created_by`, `published_at`, `restored_from`). At most one draft per storefront (partial unique index). Published rows are immutable: a trigger rejects updates to `document` once `state <> 'draft'`.
- Publish: in one transaction, refuse unless the draft is still at the requested revision (then a second publish of that revision returns the version already made), validate the draft, copy it into a new `published` version `number = max + 1`, mark the previous live version `archived`, move `live_version_id`, keep the draft (now equal to the live version). Rollback to version K, sent with the draft revision the organiser sees (refused if the draft moved on), creates version `max + 1` with K's document and `restored_from = K` and makes the draft equal to it, so history stays linear and every published version is kept. The builder saves pending edits first and stops if it cannot; its dialog says unpublished edits are replaced and that rollback restores the look, not old prices. Step 3 hooks the sale snapshot compile into this same transaction boundary.
- `preview_links` (`token_hash`, `storefront_id`, `expires_at`, `revoked_at`): 128-bit random token in the URL, SHA-256 stored, 7-day default life, at most 20 active per storefront, listed in full, with "revoke all".
- `assets` (`id`, `organiser_id`, `mime`, `bytes`, `sha256`, `width`, `height`): PNG, JPEG or WebP only (sniffed, not trusted from the client), at most 1 MB (checked from the request length before buffering), deduplicated per organiser by SHA-256, at most 200 files / 100 MB per organiser and 30 uploads per user per 10 minutes (checked inside the insert transaction). Served from `/assets/<id>` only while a live version shows the image (public, one day of caching) or through a valid preview link whose draft shows it (`?p=<token>`, private); Studio members see their own organiser's uploads. An upload can be deleted while no version refers to it. SVG is refused (script risk). Sample images ship as static files.

### D2.2 Document schema (v1)
Defined with zod in `apps/web/src/lib/storefront/schema.ts` and exported as JSON Schema for the copilot:
```
{ schemaVersion: 1,
  tokens: { mode: light|dark|auto,
            light: { primary, onPrimary, surface, onSurface, accent, muted }, dark: {...same},
            fontHeading, fontBody (ids from the curated list), radius: none|s|m|l, logo ("" | asset:<id> | sample:<name>) },
  locales: { default: ro|en, enabled: [ro, en] },
  pages: [ { id, kind: home|event|info, eventId?, slug, title: {ro,en},
             sections: [ { id, type, hidden, settings, blocks: [ { id, type, settings } ] } ] } ],
  checkout: { showLogo: bool, slots: { afterContact: [], consents: [] } },
  messages: { email: { footer: {ro,en}, showLogo }, ticket: { showLogo, accent: primary|accent } } }
```
- Section registry (`apps/web/src/lib/storefront/sections/`): each type has a settings schema, allowed block types, limits and a renderer. Types: `hero`, `lineup`, `ticket_grid`, `calendar`, `map`, `faq`, `gallery`, `countdown`, `text`, and the locked `footer` (always last, not removable: organiser legal name, links to `/fees` and terms, "Sample data" label when applicable). The editor form for a section is generated from its schema.
- Text is plain or a small Markdown subset (bold, italic, links, lists) rendered with escaping; links allow only `https:`, `mailto:` and site-relative paths; images only from `assets` or the static sample folder. Limits: 512 KB per document (as JSON; the database check on `jsonb::text` sits at 1 MB so it never binds first; the error names the largest page), 30 pages, 40 sections per page, 50 blocks per section, 60 events. Page addresses: event pages by the event's slug (`/e/<event-slug>`), info pages by their own slug, never `checkout`, `e`, `p`, `assets`, `api` or `s`.
- `ticket_grid` and `lineup` and `calendar` bind to catalogue data by `eventId` and render it live (names, perks, all-in prices through step 1's pricing module); they hold no copied prices. The map section shows a static image of the venue's OpenStreetMap tile stored on our origin: at save and publish the server fetches the one tile needed (fixed zoom 15, identifying User-Agent and Referer per the OSM tile policy) into the platform table `control.map_tiles`, served from `/assets/maps/<z>/<x>/<y>.png` with "© OpenStreetMap contributors" and a link out. A fan's browser never contacts a map service; the shop footer and `/terms` say what a shop loads.
- Unknown keys are rejected; a `schemaVersion` bump comes with a migration function, so stored documents always parse.

### D2.3 Tokens and contrast
- Tokens compile to CSS custom properties (`--eps-primary`, `--eps-on-primary`, …, `--eps-radius`, `--eps-font-heading`) on the storefront root; components use only these variables, so the same renderer wears any brand.
- Fonts: a curated list of about 8 families loaded with `next/font` (self-hosted at build, no runtime third-party requests).
- Contrast (WCAG 2.2 AA): `onSurface`/`surface`, `muted`/`surface` and `onPrimary`/`primary` at least 4.5:1, `accent` and `primary` against `surface` at least 3:1 (they colour only large text, fills, borders and underlines, never small text), hero text over the image (the page colour mixed 25% with black and with white) at least 4.5:1, checked for light and, when mode is `dark` or `auto`, dark. The suggested fix moves the failing colour's OKLCH lightness in the smallest step that passes, keeping hue. A failing pair is shown inline and blocks publish.
- Brand kit to tokens: from a logo upload the editor proposes a palette (dominant colours) the organiser can accept; the four templates each have default tokens.

### D2.4 Live preview
- The editor holds the draft in client state. On each change it posts `{ type: 'eps:draft', seq, document }` to the frame at `/studio/builder/frame/<storefrontId>?page=<id>` (same origin). The frame checks `event.origin === location.origin` and `event.source === parent`, validates the document, renders it with the shared renderer, and replies `{ type: 'eps:rendered', seq }`; stale `seq` values are ignored. Catalogue data comes from a server action scoped to the organiser.
- Device toggle sets the frame width to 1280 or 390 px. Clicking a section in the frame posts `eps:select` back so the editor focuses its form.
- Autosave: 800 ms debounce, `PUT` with the draft's `revision`; a mismatch returns 409 and the editor shows "This draft changed in another tab" with reload (the draft stays dirty and nothing more is sent until then). A generation counter drops answers to saves sent before the draft was replaced (reload, restore, template change). Undo/redo is client-side per session. The frame applies only the newest catalogue answer. The preview starts in the Studio language when the shop has it and follows the language of the field being edited.

### D2.5 Preview links and draft rendering
`/p/<token>` renders the current draft server-side for anyone holding the token, with a "Preview, not published" banner, `noindex`, `Cache-Control: private, no-store`, and no buy action (buttons show "available after publishing"). Expired or revoked tokens give 404.

### D2.6 Locked checkout
The checkout appearance is not a page: the document can set only `showLogo` and fill named `slots` (question and consent ids, defined by step 4). The schema rejects any other key. The builder shows a "Checkout" tab: a non-editable preview with the brand tokens and slot placeholders, and a note explaining why the layout is fixed.

### D2.7 Templates, languages, address, email and ticket
- Templates are functions in `apps/web/src/lib/storefront/templates/` returning a valid document for a brand (and its first event if any). The first visit to the builder for a brand with no storefront creates the draft from the brand's template choice. "Change template" replaces pages after confirmation and keeps tokens.
- Languages: every fan-facing text field is `{ro, en}`; publish requires every enabled language to be filled. The frame has a language switch. Currency comes from the brand and is shown in the ticket grid.
- The builder header shows the brand's public address `/s/<slug>` (and configured hostnames), copying the absolute URL, plus "not yet published" until the first publish, then "Published · version N" with a note that the public shop opens with step 3.
- Email and ticket previews (`/studio/builder/messages`) render the confirmation email HTML and the ticket layout with sample order data, the logo and tokens, and a placeholder QR.

### D2.8 Studio UI
Studio uses shadcn/ui; the builder is three panes (page and section tree with drag-to-reorder, the frame, the settings form) collapsing to tabs below 1024 px. Drag and drop uses the browser's native drag events plus up/down buttons for keyboard users, to avoid a new dependency.

### D2.9 Security headers
Every page sends `X-Frame-Options: DENY` and `frame-ancestors 'none'`, except the builder frame (`SAMEORIGIN` / `'self'`). Studio, preview links and `/s/` shops add a Content-Security-Policy with a per-request script nonce (`'strict-dynamic'`), and images, fonts, styles and connections from our origin only (`vercel.live` on Vercel previews for its toolbar).

### Notes from implementation
- The logo is an image reference like every other image (`logo`: an upload or a platform sample image), so sample shops can carry sample logos.
- `storefront_versions.source` (`studio`, `template`, `seed`, `ai`) is added now for step 11's AI drafts.
- The theatre template joins the brand's template choice from onboarding; `blank` gets a minimal home page.
- Writes in the builder (save, publish, restore, links, uploads) need the owner or admin role; other members see it read-only, and only a designer's first visit creates the storefront.
- Preview links (`preview_link_target`) and `/assets/<id>` (`public_asset`) are the two cross-organiser reads, each a SECURITY DEFINER function owned by its own NOLOGIN NOBYPASSRLS role.
- The preview frame keeps its real width (1280 or 390 px) and is scaled down to fit the pane.

## Risks / Trade-offs

- [The same renderer serves Studio previews and step 3's public shop; a heavy component slows the shop] → sections are server components by default; only countdown and gallery hydrate.
- [postMessage on every keystroke with a large document] → changes are batched per animation frame; documents are capped at 512 KB.
- [Contrast suggestions that drift from the brand] → only lightness moves; the organiser can override by choosing another colour, but cannot publish a failing pair.
- [Images in Postgres] → 1 MB cap and immutable caching keep it cheap for the prototype; object storage is the named scale-up path.

## Open Questions

None that change the specs.
