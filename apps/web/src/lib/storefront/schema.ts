// The storefront document, schema version 1 (design D2.2). The builder, the renderer, the seed and
// the AI copilot (step 11) all validate against this one schema: unknown keys and types are refused,
// links and images follow lib/storefront/links, the footer is always last, and the limits hold.
import * as z from "zod";
import { FONT_IDS } from "./fonts";
import { image, localized } from "./fields";
import { SECTIONS, SECTION_TYPES, type SectionDef, type SectionType } from "./sections";

export const SCHEMA_VERSION = 1;
export const LIMITS = { bytes: 512 * 1024, pages: 30, sectionsPerPage: 40, blocksPerSection: 50, slotsPerKind: 20 } as const;
export const LOCALES = ["ro", "en"] as const;
export type Locale = (typeof LOCALES)[number];

export const docId = z.string().regex(/^[a-z0-9][a-z0-9_-]{0,39}$/, "invalid_id");
const hex = z.string().regex(/^#[0-9a-f]{6}$/, "colour_invalid");

export const PALETTE_KEYS = ["primary", "onPrimary", "surface", "onSurface", "accent", "muted"] as const;
export type PaletteKey = (typeof PALETTE_KEYS)[number];
export const paletteSchema = z.strictObject({ primary: hex, onPrimary: hex, surface: hex, onSurface: hex, accent: hex, muted: hex });
export type Palette = z.infer<typeof paletteSchema>;

export const RADII = ["none", "s", "m", "l"] as const;
export const MODES = ["light", "dark", "auto"] as const;

export const tokensSchema = z.strictObject({
  mode: z.enum(MODES),
  light: paletteSchema,
  dark: paletteSchema,
  fontHeading: z.enum(FONT_IDS),
  fontBody: z.enum(FONT_IDS),
  radius: z.enum(RADII),
  logo: image(),
});
export type Tokens = z.infer<typeof tokensSchema>;

function blockUnion(def: SectionDef) {
  const types = Object.entries(def.blocks);
  if (!types.length) return z.array(z.never()).max(0, "too_many_blocks");
  const schemas = types.map(([type, b]) => z.strictObject({ id: docId, type: z.literal(type), settings: b.settings }));
  const union = schemas.length === 1 ? schemas[0] : z.discriminatedUnion("type", schemas as unknown as [z.ZodObject, z.ZodObject]);
  return z.array(union).max(def.maxBlocks, "too_many_blocks");
}

const sectionSchemas = SECTION_TYPES.map((type) => {
  const def = SECTIONS[type] as SectionDef;
  return z.strictObject({ id: docId, type: z.literal(type), hidden: z.boolean(), settings: def.settings, blocks: blockUnion(def) });
});
export const sectionSchema = z.discriminatedUnion("type", sectionSchemas as unknown as [z.ZodObject, z.ZodObject, ...z.ZodObject[]]);

export const pageSchema = z.strictObject({
  id: docId,
  kind: z.enum(["home", "event", "info"]),
  eventId: z.uuid().nullable(),
  slug: z.string().regex(/^$|^[a-z0-9][a-z0-9-]{0,58}[a-z0-9]$/, "slug_characters"),
  title: localized(120, true),
  sections: z.array(sectionSchema).min(1, "footer_required").max(LIMITS.sectionsPerPage, "too_many_sections"),
});

const slotId = z.string().regex(/^[a-z0-9][a-z0-9_-]{0,63}$/, "invalid_id");

/** The locked checkout (D2.6): a logo switch and the two named slots, nothing else. */
export const checkoutSchema = z.strictObject({
  showLogo: z.boolean(),
  slots: z.strictObject({
    afterContact: z.array(slotId).max(LIMITS.slotsPerKind),
    consents: z.array(slotId).max(LIMITS.slotsPerKind),
  }),
});

export const messagesSchema = z.strictObject({
  email: z.strictObject({ footer: localized(500), showLogo: z.boolean() }),
  ticket: z.strictObject({ showLogo: z.boolean(), accent: z.enum(["primary", "accent"]) }),
});

export const documentSchema = z
  .strictObject({
    schemaVersion: z.literal(SCHEMA_VERSION),
    tokens: tokensSchema,
    locales: z.strictObject({ default: z.enum(LOCALES), enabled: z.array(z.enum(LOCALES)).min(1).max(2) }),
    pages: z.array(pageSchema).min(1).max(LIMITS.pages, "too_many_pages"),
    checkout: checkoutSchema,
    messages: messagesSchema,
  })
  .superRefine((doc, ctx) => {
    const issue = (path: (string | number)[], message: string) => ctx.addIssue({ code: "custom", path, message });
    if (new Set(doc.locales.enabled).size !== doc.locales.enabled.length) issue(["locales", "enabled"], "duplicate");
    if (!doc.locales.enabled.includes(doc.locales.default)) issue(["locales", "default"], "default_not_enabled");
    const ids = new Set<string>();
    const unique = (id: string, path: (string | number)[]) => {
      if (ids.has(id)) issue(path, "duplicate_id");
      ids.add(id);
    };
    const slugs = new Set<string>();
    doc.pages.forEach((p, pi) => {
      unique(p.id, ["pages", pi, "id"]);
      if (pi === 0 && (p.kind !== "home" || p.slug !== "")) issue(["pages", pi, "kind"], "home_first");
      if (pi > 0 && p.kind === "home") issue(["pages", pi, "kind"], "one_home");
      if (pi > 0 && p.slug === "") issue(["pages", pi, "slug"], "required");
      if (slugs.has(p.slug)) issue(["pages", pi, "slug"], "taken");
      slugs.add(p.slug);
      if (p.kind === "event" && !p.eventId) issue(["pages", pi, "eventId"], "required");
      if (p.kind !== "event" && p.eventId) issue(["pages", pi, "eventId"], "invalid");
      const sections = p.sections as { id: string; type: SectionType; hidden: boolean; blocks: { id: string }[] }[];
      sections.forEach((s, si) => {
        unique(s.id, ["pages", pi, "sections", si, "id"]);
        s.blocks.forEach((b, bi) => unique(b.id, ["pages", pi, "sections", si, "blocks", bi, "id"]));
        const last = si === sections.length - 1;
        if (s.type === "footer" && !last) issue(["pages", pi, "sections", si], "footer_last");
        if (s.type === "footer" && s.hidden) issue(["pages", pi, "sections", si, "hidden"], "footer_locked");
      });
      if (sections.at(-1)?.type !== "footer") issue(["pages", pi, "sections"], "footer_last");
    });
  });

export type Block = { id: string; type: string; settings: Record<string, unknown> };
export type Section = { id: string; type: SectionType; hidden: boolean; settings: Record<string, unknown>; blocks: Block[] };
export type Page = Omit<z.infer<typeof pageSchema>, "sections"> & { sections: Section[] };
export type StorefrontDocument = Omit<z.infer<typeof documentSchema>, "pages"> & { pages: Page[] };

export type ParseResult = { ok: true; doc: StorefrontDocument } | { ok: false; path: string; code: string };

const utf8 = new TextEncoder();
export function byteSize(raw: unknown): number {
  return utf8.encode(typeof raw === "string" ? raw : (JSON.stringify(raw) ?? "")).length;
}

/** Validates a document; on failure, the path and code of the first problem. */
export function parseDocument(raw: unknown): ParseResult {
  let size: number;
  try {
    size = byteSize(raw);
  } catch {
    return { ok: false, path: "", code: "invalid" };
  }
  if (size > LIMITS.bytes) return { ok: false, path: "", code: "document_too_large" };
  const r = documentSchema.safeParse(raw);
  if (r.success) return { ok: true, doc: r.data as unknown as StorefrontDocument };
  const first = r.error.issues[0];
  const code =
    first.code === "unrecognized_keys"
      ? "unknown_key"
      : first.code === "invalid_union"
        ? "unknown_type"
        : first.code === "too_big"
          ? first.message.startsWith("too_") ? first.message : "too_long"
          : /^[a-z_]+$/.test(first.message)
            ? first.message
            : "invalid";
  return { ok: false, path: first.path.join("."), code };
}

/** The document schema as JSON Schema, for the AI copilot's tool definitions (step 11). */
export function documentJsonSchema(): Record<string, unknown> {
  return z.toJSONSchema(documentSchema, { unrepresentable: "any" }) as Record<string, unknown>;
}

/** Fresh ids for sections, blocks and pages inside a document (not database ids). */
export function newDocId(prefix: string): string {
  const rnd = globalThis.crypto.getRandomValues(new Uint8Array(6));
  return `${prefix}-${Array.from(rnd, (b) => b.toString(36).padStart(2, "0")).join("").slice(0, 10)}`;
}

/** Every event id a document refers to (event pages and catalogue sections). */
export function referencedEvents(doc: StorefrontDocument): string[] {
  const out = new Set<string>();
  for (const p of doc.pages) {
    if (p.eventId) out.add(p.eventId);
    for (const s of p.sections as Section[]) {
      const id = (s.settings as { eventId?: string }).eventId;
      if (id) out.add(id);
    }
  }
  return [...out];
}

/** Every uploaded asset a document refers to. */
export function referencedAssets(doc: StorefrontDocument): string[] {
  const refs: string[] = [doc.tokens.logo];
  for (const p of doc.pages)
    for (const s of p.sections as Section[]) {
      refs.push(String((s.settings as { image?: string }).image ?? ""));
      for (const b of s.blocks) refs.push(String((b.settings as { image?: string }).image ?? ""));
    }
  return [...new Set(refs.filter((r) => r.startsWith("asset:")).map((r) => r.slice(6)))];
}
