// The section registry (design D2.2): every section type with its settings schema, the block types
// it may hold, its limits and its defaults. The editor's forms, the renderer and the publish check
// all read this one table; adding a section type means adding it here and a renderer.
import * as z from "zod";
import { choice, datetime, eventRef, flag, image, link, localized, markdown } from "./fields";

const L = (ro: string, en: string) => ({ ro, en });

type BlockDef = { settings: z.ZodObject; defaults: () => Record<string, unknown> };

export type SectionDef = {
  settings: z.ZodObject;
  blocks: Record<string, BlockDef>;
  maxBlocks: number;
  /** Reads catalogue data of the event it names. */
  catalogue?: boolean;
  /** The footer: always last, not removable, not movable, not hideable. */
  locked?: boolean;
  /** Hydrates on the public page (all other sections are pure server output). */
  hydrates?: boolean;
  defaults: (ctx: { eventId?: string }) => Record<string, unknown>;
};

const heading = () => localized(80);

export const SECTIONS = {
  hero: {
    settings: z.strictObject({
      heading: localized(120, true),
      subheading: localized(300),
      image: image(),
      align: choice(["left", "center"] as const),
    }),
    blocks: {
      button: {
        settings: z.strictObject({ label: localized(40, true), href: link(true), style: choice(["primary", "secondary"] as const) }),
        defaults: () => ({ label: L("Bilete", "Tickets"), href: "/", style: "primary" }),
      },
    },
    maxBlocks: 3,
    defaults: () => ({ heading: L("Titlu nou", "New title"), subheading: L("", ""), image: "", align: "center" }),
  },
  lineup: {
    settings: z.strictObject({ heading: heading(), eventId: eventRef(true), layout: choice(["grid", "list"] as const) }),
    blocks: {},
    maxBlocks: 0,
    catalogue: true,
    defaults: ({ eventId }) => ({ heading: L("Line-up", "Line-up"), eventId: eventId ?? "", layout: "grid" }),
  },
  ticket_grid: {
    settings: z.strictObject({ heading: heading(), eventId: eventRef(true), showPerks: flag() }),
    blocks: {},
    maxBlocks: 0,
    catalogue: true,
    defaults: ({ eventId }) => ({ heading: L("Bilete", "Tickets"), eventId: eventId ?? "", showPerks: true }),
  },
  calendar: {
    settings: z.strictObject({ heading: heading(), eventId: eventRef(true) }),
    blocks: {},
    maxBlocks: 0,
    catalogue: true,
    defaults: ({ eventId }) => ({ heading: L("Program", "Schedule"), eventId: eventId ?? "" }),
  },
  map: {
    settings: z.strictObject({ heading: heading(), eventId: eventRef(true) }),
    blocks: {},
    maxBlocks: 0,
    catalogue: true,
    defaults: ({ eventId }) => ({ heading: L("Cum ajungi", "Getting there"), eventId: eventId ?? "" }),
  },
  faq: {
    settings: z.strictObject({ heading: heading() }),
    blocks: {
      item: {
        settings: z.strictObject({ question: localized(200, true), answer: markdown(2000, true) }),
        defaults: () => ({ question: L("Întrebare", "Question"), answer: L("Răspuns", "Answer") }),
      },
    },
    maxBlocks: 50,
    defaults: () => ({ heading: L("Întrebări frecvente", "FAQ") }),
  },
  gallery: {
    settings: z.strictObject({ heading: heading(), layout: choice(["grid", "carousel"] as const) }),
    blocks: {
      image: {
        settings: z.strictObject({ image: image(true), alt: localized(200) }),
        defaults: () => ({ image: "sample:festival-crowd", alt: L("Imagine de exemplu", "Sample image") }),
      },
    },
    maxBlocks: 50,
    hydrates: true,
    defaults: () => ({ heading: L("Galerie", "Gallery"), layout: "grid" }),
  },
  countdown: {
    settings: z.strictObject({ heading: heading(), eventId: eventRef(), until: datetime() }),
    blocks: {},
    maxBlocks: 0,
    catalogue: true,
    hydrates: true,
    defaults: ({ eventId }) => ({ heading: L("Începe în", "Starts in"), eventId: eventId ?? "", until: "" }),
  },
  text: {
    settings: z.strictObject({ heading: heading(), body: markdown(5000, true), align: choice(["left", "center"] as const) }),
    blocks: {},
    maxBlocks: 0,
    defaults: () => ({ heading: L("", ""), body: L("Text nou.", "New text."), align: "left" }),
  },
  footer: {
    settings: z.strictObject({ note: localized(300) }),
    blocks: {},
    maxBlocks: 0,
    locked: true,
    defaults: () => ({ note: L("", "") }),
  },
} satisfies Record<string, SectionDef>;

export type SectionType = keyof typeof SECTIONS;
export const SECTION_TYPES = Object.keys(SECTIONS) as [SectionType, ...SectionType[]];
/** Types the organiser can add (the footer comes with every page). */
export const ADDABLE_SECTIONS = SECTION_TYPES.filter((t) => !(SECTIONS[t] as SectionDef).locked);

export function sectionDef(type: string): SectionDef | null {
  return Object.prototype.hasOwnProperty.call(SECTIONS, type) ? (SECTIONS[type as SectionType] as SectionDef) : null;
}
