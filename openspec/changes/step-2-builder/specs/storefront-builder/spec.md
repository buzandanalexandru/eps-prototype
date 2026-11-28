# Spec Delta

## Purpose

Lets every organiser make the shared platform look like their own brand without code: a brand kit becomes design tokens, pages are built from typed sections with a live preview, and changes move from draft to preview link to published version with rollback, while checkout stays locked.

## ADDED Requirements

### Requirement: One storefront document per brand
Each brand SHALL have one storefront, stored as a versioned document of design tokens, languages, pages of sections and blocks, checkout appearance and message design. Every stored document MUST validate against the current schema.

#### Scenario: First visit to the builder
- **WHEN** an organiser opens the builder for a brand that chose the festival template at onboarding
- **THEN** a draft is created from the festival template and opens in the editor
Test: apps/web/tests/e2e/builder.spec.ts

#### Scenario: Invalid document refused
- **WHEN** a save contains an unknown section type, an unknown key or exceeds 512 KB
- **THEN** the save is refused with the path of the first error and the stored draft is unchanged
Test: apps/web/tests/unit/storefront-schema.test.ts

### Requirement: Starting templates
The builder SHALL offer festival, resort, club night and theatre templates, each a complete, publishable set of pages and sections with default tokens. Changing template on an existing draft MUST ask for confirmation and keep the brand's tokens.

#### Scenario: Every template is valid
- **WHEN** each template is generated for a brand with one event
- **THEN** the document validates, passes the contrast check and the publish checks
Test: apps/web/tests/unit/storefront-templates.test.ts

#### Scenario: Change template
- **WHEN** the organiser switches a customised festival draft to the resort template and confirms
- **THEN** the pages are replaced by the resort pages and the colours, fonts, radius and logo are kept
Test: apps/web/tests/e2e/builder.spec.ts

### Requirement: Brand kit becomes design tokens
The organiser SHALL set a logo, primary, surface, text and accent colours, a heading and a body font from a curated list, a corner radius and light, dark or automatic mode, and these MUST become the tokens every storefront section, the checkout and the message previews use.

#### Scenario: Changing the primary colour
- **WHEN** the organiser changes the primary colour
- **THEN** buttons, links and highlights in every section of the preview take the new colour, and no section keeps a colour of its own
Test: apps/web/tests/unit/storefront-tokens.test.tsx

#### Scenario: Logo upload
- **WHEN** the organiser uploads a PNG logo under 1 MB
- **THEN** it appears in the header of the preview and the builder proposes a palette from it
Test: apps/web/tests/e2e/builder.spec.ts

#### Scenario: Unsafe or oversized upload
- **WHEN** the upload is an SVG, a file whose content is not PNG, JPEG or WebP, or larger than 1 MB
- **THEN** it is refused with the reason
Test: apps/web/tests/unit/storefront-assets.test.ts

#### Scenario: Upload limits and who sees an upload
- **WHEN** an organiser uploads the same image twice, goes past 200 files or 100 MB, or a user uploads more than 30 images in 10 minutes
- **THEN** the same image is stored once and the others are refused with the reason; an upload is shown to visitors only while a published version or a valid preview link shows it, and one still used by any version cannot be deleted
Test: apps/web/tests/unit/storefront-assets.test.ts

### Requirement: Contrast check with a suggested fix
The builder SHALL check text and button colour pairs against WCAG 2.2 AA (4.5:1 for text, 3:1 for accents and controls) in every mode the brand uses, show each failing pair with a suggested colour that passes, and MUST block publishing while any pair fails.

#### Scenario: Hero text over an image
- **WHEN** the text colour passes on the page colour but not over a dark or light hero image
- **THEN** the pair over the image is reported with a suggested text colour and blocks publishing
Test: apps/web/tests/unit/storefront-contrast.test.ts

#### Scenario: Failing pair
- **WHEN** the organiser picks white text on a light yellow primary
- **THEN** the builder shows the ratio, marks it failing, and offers a darker shade of the same hue that passes
Test: apps/web/tests/unit/storefront-contrast.test.ts

#### Scenario: Automatic mode checks both palettes
- **WHEN** the mode is automatic and only the dark palette fails
- **THEN** the dark pair is reported and publish is blocked
Test: apps/web/tests/unit/storefront-contrast.test.ts

#### Scenario: Accept the suggestion
- **WHEN** the organiser accepts the suggested colour
- **THEN** the pair passes and the block on publishing is lifted
Test: apps/web/tests/unit/storefront-contrast.test.ts

### Requirement: Pages built from sections and blocks
The organiser SHALL be able to add, remove, reorder, hide and configure sections (hero, line-up, ticket grid, calendar, map, FAQ, gallery, countdown, text) and their blocks on each page, through forms generated from each section's settings. A locked footer MUST stay last on every page.

#### Scenario: Reorder and hide
- **WHEN** the organiser moves the FAQ above the line-up and hides the gallery
- **THEN** the preview shows the new order without the gallery, and the hidden gallery stays in the draft
Test: apps/web/tests/e2e/builder.spec.ts

#### Scenario: Keyboard reorder
- **WHEN** a keyboard user moves a section with the up and down controls
- **THEN** the order changes as with dragging
Test: apps/web/tests/e2e/builder.spec.ts

#### Scenario: Footer cannot be removed
- **WHEN** a request removes or moves the footer
- **THEN** it is refused and the footer stays last
Test: apps/web/tests/unit/storefront-schema.test.ts

#### Scenario: Section limits
- **WHEN** a page would exceed 40 sections or a section 50 blocks
- **THEN** the add control is disabled and a save over the limit is refused
Test: apps/web/tests/unit/storefront-schema.test.ts

### Requirement: Organiser content cannot inject code
Storefront text SHALL be plain text or a small formatting subset rendered escaped; links MUST be `https:`, `mailto:` or site-relative, and images MUST come from the organiser's uploads or the platform's sample images.

#### Scenario: Script in text
- **WHEN** an FAQ answer contains `<script>` or an `onerror` attribute
- **THEN** it is rendered as visible text and nothing executes
Test: apps/web/tests/unit/storefront-render-safety.test.tsx

#### Scenario: Pages cannot take platform addresses
- **WHEN** an info page's address is `checkout`, `e`, `p`, `assets`, `api` or `s`, or an event page sets an address of its own
- **THEN** the save is refused; an event page is always at `/e/<event's address>`
Test: apps/web/tests/unit/storefront-schema.test.ts

#### Scenario: The shop cannot be framed and loads only from the platform
- **WHEN** any page except the builder's preview frame is requested
- **THEN** it refuses framing, and Studio and shop pages carry a content security policy that allows scripts only with the request's nonce and images, fonts and connections only from the platform
Test: apps/web/tests/unit/security-headers.test.ts

#### Scenario: Dangerous link
- **WHEN** a hero button links to `javascript:alert(1)` or a `data:` URL
- **THEN** the save is refused
Test: apps/web/tests/unit/storefront-schema.test.ts

### Requirement: Catalogue sections show live catalogue data
Ticket grid, line-up and calendar sections SHALL reference an event and render its current products, line-up and days; the ticket grid MUST show each product's all-in price in the brand's currency, never a price stored in the document.

#### Scenario: Price changed in the catalogue
- **WHEN** the organiser changes a product's face price while the builder is open
- **THEN** the next preview render shows the new all-in price
Test: apps/web/tests/unit/storefront-catalogue-binding.test.ts

#### Scenario: Venue map without a map service
- **WHEN** a map section shows an event whose venue has coordinates
- **THEN** the map is an image stored by the platform and served from its own address, credited to OpenStreetMap with a link out, and the fan's browser contacts no map service
Test: apps/web/tests/unit/storefront-maps.test.ts

#### Scenario: Event of another organiser
- **WHEN** a document references an event id of another organiser
- **THEN** the save is refused as an unknown event
Test: apps/web/tests/unit/storefront-catalogue-binding.test.ts

### Requirement: Live preview on desktop and phone
The builder SHALL show the draft in a preview frame that re-renders after every change without saving first, and SHALL switch between desktop and phone widths. The frame MUST accept updates only from the builder that opened it.

#### Scenario: Immediate update
- **WHEN** the organiser types a new hero title
- **THEN** the preview shows it within one second, before autosave completes
Test: apps/web/tests/e2e/builder-preview.spec.ts

#### Scenario: Phone width
- **WHEN** the organiser switches to phone
- **THEN** the frame is 390 px wide, the page has no horizontal scroll and the ticket grid stacks
Test: apps/web/tests/e2e/builder-preview.spec.ts

#### Scenario: Message from another origin
- **WHEN** a page on another origin posts a document to the frame
- **THEN** the frame ignores it
Test: apps/web/tests/unit/preview-frame.test.ts

#### Scenario: Select from the preview
- **WHEN** the organiser clicks a section in the frame
- **THEN** the editor opens that section's settings
Test: apps/web/tests/e2e/builder-preview.spec.ts

### Requirement: Drafts autosave and detect conflicts
Changes SHALL stay in a draft that saves automatically, separate from the published version. A save based on an older draft revision MUST be refused rather than overwrite newer work.

#### Scenario: Draft does not change the live shop
- **WHEN** the organiser edits a draft of a published storefront
- **THEN** the published version is unchanged
Test: apps/web/tests/unit/storefront-versions.test.ts

#### Scenario: Two tabs
- **WHEN** a draft is saved in one tab and then an older revision is saved from another
- **THEN** the second save is refused and that tab says the draft changed elsewhere and offers to reload
Test: apps/web/tests/unit/storefront-versions.test.ts

### Requirement: Private preview links
The organiser SHALL be able to create a private link that shows the current draft to anyone holding it, marked "Preview, not published", not indexable and not cached, with no buying. Links MUST expire after 7 days by default and be revocable.

#### Scenario: Open a preview link signed out
- **WHEN** a person without an account opens a valid preview link
- **THEN** they see the current draft with the preview banner, and buy buttons say tickets are available after publishing
Test: apps/web/tests/e2e/builder-publish.spec.ts

#### Scenario: Many links
- **WHEN** a storefront has 20 active preview links
- **THEN** a 21st is refused until one is revoked, every active link is listed, and "revoke all" revokes them in one action
Test: apps/web/tests/unit/preview-links.test.ts

#### Scenario: Revoked or expired
- **WHEN** the link is revoked or older than its expiry
- **THEN** it returns not found
Test: apps/web/tests/unit/preview-links.test.ts

#### Scenario: Guessing tokens
- **WHEN** someone requests preview links with random tokens
- **THEN** each returns not found and the stored data contains only hashes of real tokens
Test: apps/web/tests/unit/preview-links.test.ts

### Requirement: Publishing creates an immutable version
Publishing SHALL copy the draft into a new numbered version that becomes live and MUST never change afterwards. Publishing MUST be refused while the document fails validation, the contrast check, a required translation, or an event page lacks a ticket grid.

#### Scenario: First publish
- **WHEN** the organiser publishes a valid draft
- **THEN** version 1 is live, the history lists it with time and author, and the builder shows the brand's address as live
Test: apps/web/tests/e2e/builder-publish.spec.ts

#### Scenario: Publish blocked
- **WHEN** the English hero title is empty on a brand that sells in English
- **THEN** publish is refused with a list of problems linking to each field
Test: apps/web/tests/unit/storefront-publish.test.ts

#### Scenario: Published version is read-only
- **WHEN** any request tries to change the document of a published version
- **THEN** the database refuses it
Test: apps/web/tests/unit/storefront-versions.test.ts

#### Scenario: Double click on publish
- **WHEN** publish is sent twice for the same draft revision
- **THEN** exactly one new version is created
Test: apps/web/tests/unit/storefront-publish.test.ts

#### Scenario: Publish or restore after the draft moved on
- **WHEN** publish or restore is sent with a draft revision that is no longer the current one
- **THEN** it is refused as changed elsewhere and nothing is published or overwritten
Test: apps/web/tests/unit/storefront-versions.test.ts

### Requirement: Version history and rollback
The builder SHALL list every published version and restore any of them in one action; a restore MUST publish that version's document as a new version, so history is never rewritten.

#### Scenario: Roll back
- **WHEN** versions 1 to 3 exist and the organiser restores version 1
- **THEN** version 4 is live with version 1's document, marked as restored from 1, and versions 1 to 3 remain listed
Test: apps/web/tests/unit/storefront-versions.test.ts

### Requirement: Checkout stays locked
The checkout SHALL take only the brand's tokens, an optional logo and named slots for questions and consents. The document MUST NOT accept any other checkout setting, and the builder SHALL show the checkout as a locked preview.

#### Scenario: Layout key on checkout
- **WHEN** a document sets a section, colour override or free text on the checkout
- **THEN** validation refuses it
Test: apps/web/tests/unit/storefront-schema.test.ts

#### Scenario: Locked preview
- **WHEN** the organiser opens the Checkout tab
- **THEN** it shows the checkout in the brand's tokens with labelled slot placeholders and no editing controls
Test: apps/web/tests/e2e/builder-preview.spec.ts

### Requirement: Languages and currency per shop
Each storefront SHALL choose its languages from Romanian and English with a default, and show prices in the brand's currency. The preview SHALL switch language.

#### Scenario: English and Romanian preview
- **WHEN** a shop enables RO and EN and the organiser switches the preview to RO
- **THEN** section texts, platform labels and dates appear in Romanian with diacritics and prices show RON
Test: manual check on the deployed preview (language switch in the builder frame, diacritics correct)

### Requirement: Brand address shown in the builder
The builder SHALL show the brand's public address `/s/<brand>` and any configured hostnames with a copy button, marked as not live until the first publish.

#### Scenario: Before and after first publish
- **WHEN** the organiser views the builder before and then after publishing
- **THEN** the address is first marked "not published yet" and then "live"
Test: apps/web/tests/e2e/builder-publish.spec.ts

### Requirement: Email and ticket design preview
The builder SHALL preview the confirmation email and the ticket with the brand's logo, colours and fonts on a sample order, with a footer text per language.

#### Scenario: Brand kit on the ticket
- **WHEN** the organiser opens the ticket preview after changing the primary colour
- **THEN** the ticket shows the logo, the new colour and a placeholder QR code
Test: apps/web/tests/unit/storefront-messages.test.tsx

### Requirement: Storefronts stay with their organiser
Drafts, versions, uploads and preview links SHALL belong to the brand's organiser, and no organiser MUST be able to read or change another organiser's.

#### Scenario: Another organiser's draft
- **WHEN** a member of organiser A requests organiser B's storefront draft or versions
- **THEN** the response is not found
Test: apps/web/tests/unit/storefront-isolation.test.ts
