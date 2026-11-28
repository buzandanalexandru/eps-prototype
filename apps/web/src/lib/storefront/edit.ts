// Pure edits of a storefront document, used by the builder (and tested on their own). Every edit
// returns a new document and keeps the rules the schema enforces: the footer stays last and cannot be
// removed, moved or hidden; limits are not exceeded.
import { sectionDef, SECTIONS, type SectionType } from "./sections";
import { LIMITS, newDocId, RESERVED_PAGE_SLUGS, type Block, type Page, type Section, type StorefrontDocument } from "./schema";

type Doc = StorefrontDocument;

function mapPage(doc: Doc, pageId: string, fn: (p: Page) => Page): Doc {
  return { ...doc, pages: doc.pages.map((p) => (p.id === pageId ? fn(p) : p)) };
}
function sectionsOf(p: Page): Section[] {
  return p.sections as Section[];
}
function withSections(p: Page, sections: Section[]): Page {
  return { ...p, sections } as Page;
}
function mapSection(doc: Doc, pageId: string, sectionId: string, fn: (s: Section) => Section): Doc {
  return mapPage(doc, pageId, (p) => withSections(p, sectionsOf(p).map((s) => (s.id === sectionId ? fn(s) : s))));
}

export function canAddSection(p: Page): boolean {
  return p.sections.length < LIMITS.sectionsPerPage;
}

/** Adds a section of `type` just before the footer (or at `index`), with its defaults. */
export function addSection(doc: Doc, pageId: string, type: SectionType, opts: { eventId?: string; index?: number } = {}): { doc: Doc; id: string | null } {
  const def = sectionDef(type);
  const page = doc.pages.find((p) => p.id === pageId);
  if (!def || def.locked || !page || !canAddSection(page)) return { doc, id: null };
  const id = newDocId(type.replace("_", ""));
  const s = { id, type, hidden: false, settings: def.defaults({ eventId: opts.eventId ?? page.eventId ?? undefined }), blocks: [] } as unknown as Section;
  return {
    id,
    doc: mapPage(doc, pageId, (p) => {
      const list = sectionsOf(p);
      const footerAt = list.findIndex((x) => x.type === "footer");
      const end = footerAt === -1 ? list.length : footerAt;
      const at = Math.min(Math.max(opts.index ?? end, 0), end);
      return withSections(p, [...list.slice(0, at), s, ...list.slice(at)]);
    }),
  };
}

export function removeSection(doc: Doc, pageId: string, sectionId: string): Doc {
  return mapPage(doc, pageId, (p) => withSections(p, sectionsOf(p).filter((s) => s.id !== sectionId || s.type === "footer")));
}

/** Moves a section from one index to another; the footer and the slot after it never move. */
export function moveSection(doc: Doc, pageId: string, from: number, to: number): Doc {
  return mapPage(doc, pageId, (p) => {
    const list = [...sectionsOf(p)];
    const last = list.length - 1 - (list.at(-1)?.type === "footer" ? 1 : 0);
    if (from < 0 || from > last || list[from]?.type === "footer") return p;
    const target = Math.min(Math.max(to, 0), last);
    if (target === from) return p;
    const [s] = list.splice(from, 1);
    list.splice(target, 0, s);
    return withSections(p, list);
  });
}

export function toggleHidden(doc: Doc, pageId: string, sectionId: string): Doc {
  return mapSection(doc, pageId, sectionId, (s) => (s.type === "footer" ? s : { ...s, hidden: !s.hidden }));
}

export function setSectionSetting(doc: Doc, pageId: string, sectionId: string, key: string, value: unknown): Doc {
  return mapSection(doc, pageId, sectionId, (s) => ({ ...s, settings: { ...s.settings, [key]: value } }));
}

export function canAddBlock(s: Section): boolean {
  const def = sectionDef(s.type);
  return !!def && Object.keys(def.blocks).length > 0 && s.blocks.length < def.maxBlocks;
}

export function addBlock(doc: Doc, pageId: string, sectionId: string, type: string): Doc {
  return mapSection(doc, pageId, sectionId, (s) => {
    const def = sectionDef(s.type)?.blocks[type];
    if (!def || !canAddBlock(s)) return s;
    return { ...s, blocks: [...s.blocks, { id: newDocId("b"), type, settings: def.defaults() }] };
  });
}

export function removeBlock(doc: Doc, pageId: string, sectionId: string, blockId: string): Doc {
  return mapSection(doc, pageId, sectionId, (s) => ({ ...s, blocks: s.blocks.filter((b) => b.id !== blockId) }));
}

export function moveBlock(doc: Doc, pageId: string, sectionId: string, from: number, to: number): Doc {
  return mapSection(doc, pageId, sectionId, (s) => {
    if (from < 0 || from >= s.blocks.length || to < 0 || to >= s.blocks.length) return s;
    const blocks = [...s.blocks];
    const [b] = blocks.splice(from, 1);
    blocks.splice(to, 0, b);
    return { ...s, blocks };
  });
}

export function setBlockSetting(doc: Doc, pageId: string, sectionId: string, blockId: string, key: string, value: unknown): Doc {
  return mapSection(doc, pageId, sectionId, (s) => ({
    ...s,
    blocks: s.blocks.map((b: Block) => (b.id === blockId ? { ...b, settings: { ...b.settings, [key]: value } } : b)),
  }));
}

export function setPage(doc: Doc, pageId: string, patch: Partial<Pick<Page, "title" | "slug" | "eventId">>): Doc {
  return mapPage(doc, pageId, (p) => ({ ...p, ...patch }));
}

export function addPage(doc: Doc, kind: "info" | "event", opts: { eventId?: string; slug?: string } = {}): { doc: Doc; id: string | null } {
  if (doc.pages.length >= LIMITS.pages) return { doc, id: null };
  if (kind === "event" && (!opts.eventId || doc.pages.some((p) => p.kind === "event" && p.eventId === opts.eventId))) return { doc, id: null };
  const taken = new Set(doc.pages.map((p) => p.slug));
  let slug = "";
  if (kind === "info") {
    const base = opts.slug || "info";
    slug = base;
    for (let i = 2; taken.has(slug) || (RESERVED_PAGE_SLUGS as readonly string[]).includes(slug); i++) slug = `${base}-${i}`;
  }
  const id = newDocId("p");
  const footer = { id: newDocId("footer"), type: "footer", hidden: false, settings: SECTIONS.footer.defaults(), blocks: [] };
  const first =
    kind === "event"
      ? { id: newDocId("ticketgrid"), type: "ticket_grid", hidden: false, settings: SECTIONS.ticket_grid.defaults({ eventId: opts.eventId }), blocks: [] }
      : { id: newDocId("text"), type: "text", hidden: false, settings: SECTIONS.text.defaults(), blocks: [] };
  const page = { id, kind, eventId: kind === "event" ? (opts.eventId ?? null) : null, slug, title: { ro: "Pagină nouă", en: "New page" }, sections: [first, footer] } as unknown as Page;
  return { id, doc: { ...doc, pages: [...doc.pages, page] } };
}

export function removePage(doc: Doc, pageId: string): Doc {
  const pages = doc.pages.filter((p, i) => i === 0 || p.id !== pageId);
  return { ...doc, pages };
}

export function findSection(doc: Doc, sectionId: string | null): { page: Page; section: Section; index: number } | null {
  if (!sectionId) return null;
  for (const page of doc.pages) {
    const index = sectionsOf(page).findIndex((s) => s.id === sectionId);
    if (index !== -1) return { page, section: sectionsOf(page)[index], index };
  }
  return null;
}
