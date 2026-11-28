// What must hold before a draft is published (design D5.1 of tasks, spec "Publishing creates an
// immutable version"): the document validates, every colour pair passes, every enabled language is
// filled, every event page sells tickets, and every referenced event is the organiser's. Pure, so the
// editor shows the same list the server enforces.
import * as z from "zod";
import { contrastFailures } from "./contrast";
import { describeFields } from "./fields";
import { sectionDef } from "./sections";
import { parseDocument, type Locale, type Section, type StorefrontDocument } from "./schema";

export type PublishProblem = {
  code: "invalid" | "contrast" | "missing_translation" | "required" | "event_page_needs_tickets" | "unknown_event" | "countdown_target";
  /** Dotted path of the field to fix. */
  path: string;
  pageId?: string;
  sectionId?: string;
  blockId?: string;
  locale?: Locale;
  detail?: string;
};

type Ctx = { knownEvents: ReadonlySet<string> };

function textProblems(
  value: unknown,
  required: boolean,
  locales: Locale[],
  base: Omit<PublishProblem, "code" | "locale">,
): PublishProblem[] {
  const v = (value ?? {}) as Partial<Record<Locale, string>>;
  const filled = locales.filter((l) => (v[l] ?? "").trim() !== "");
  if (!required && filled.length === 0) return [];
  return locales.filter((l) => !filled.includes(l)).map((l) => ({ ...base, code: "missing_translation" as const, locale: l, path: `${base.path}.${l}` }));
}

function settingsProblems(
  schema: z.ZodObject,
  settings: Record<string, unknown>,
  locales: Locale[],
  base: Omit<PublishProblem, "code" | "path"> & { path: string },
  ctx: Ctx,
): PublishProblem[] {
  const out: PublishProblem[] = [];
  for (const f of describeFields(schema)) {
    const path = `${base.path}.${f.key}`;
    const value = settings[f.key];
    if (f.kind === "localized" || f.kind === "markdown") out.push(...textProblems(value, !!f.required, locales, { ...base, path }));
    else if (f.required && (value === "" || value === undefined || value === null)) out.push({ ...base, code: "required", path });
    if (f.kind === "event" && typeof value === "string" && value && !ctx.knownEvents.has(value)) out.push({ ...base, code: "unknown_event", path });
  }
  return out;
}

export function publishProblems(raw: unknown, ctx: Ctx): PublishProblem[] {
  const parsed = parseDocument(raw);
  if (!parsed.ok) return [{ code: "invalid", path: parsed.path, detail: parsed.code }];
  const doc: StorefrontDocument = parsed.doc;
  const locales = doc.locales.enabled;
  const out: PublishProblem[] = [];
  for (const c of contrastFailures(doc.tokens)) out.push({ code: "contrast", path: `tokens.${c.scheme}.${c.fix}`, detail: `${c.fg}/${c.bg}` });
  out.push(...textProblems(doc.messages.email.footer, false, locales, { path: "messages.email.footer" }));
  doc.pages.forEach((p, pi) => {
    const pagePath = `pages.${pi}`;
    out.push(...textProblems(p.title, true, locales, { path: `${pagePath}.title`, pageId: p.id }));
    if (p.eventId && !ctx.knownEvents.has(p.eventId)) out.push({ code: "unknown_event", path: `${pagePath}.eventId`, pageId: p.id });
    const sections = p.sections as Section[];
    sections.forEach((s, si) => {
      const def = sectionDef(s.type);
      if (!def) return;
      const base = { pageId: p.id, sectionId: s.id, path: `${pagePath}.sections.${si}.settings` };
      const eventId = (s.settings as { eventId?: string }).eventId;
      if (s.hidden) {
        // Hidden sections are not shown, but may not point at another organiser's event.
        if (eventId && !ctx.knownEvents.has(eventId)) out.push({ ...base, code: "unknown_event", path: `${base.path}.eventId` });
        return;
      }
      out.push(...settingsProblems(def.settings, s.settings, locales, base, ctx));
      if (s.type === "countdown" && !eventId && !(s.settings as { until?: string }).until) out.push({ ...base, code: "countdown_target", path: `${base.path}.until` });
      s.blocks.forEach((b, bi) => {
        const bdef = def.blocks[b.type];
        if (bdef) out.push(...settingsProblems(bdef.settings, b.settings, locales, { pageId: p.id, sectionId: s.id, blockId: b.id, path: `${pagePath}.sections.${si}.blocks.${bi}.settings` }, ctx));
      });
    });
    if (p.kind === "event" && !sections.some((s) => s.type === "ticket_grid" && !s.hidden && (s.settings as { eventId?: string }).eventId === p.eventId)) {
      out.push({ code: "event_page_needs_tickets", path: `${pagePath}.sections`, pageId: p.id });
    }
  });
  return out;
}
