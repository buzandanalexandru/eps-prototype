// Starting templates (design D2.7; B2): festival, resort, club night and theatre. Each is a function
// returning a complete document for a brand (and its first event, if any) that validates, passes the
// contrast check and the publish checks. A brand that chose "blank" at onboarding gets a minimal
// home page. Section, block and page ids are fresh per call.
import type { Localized } from "../../catalogue/schema";
import { SECTIONS, type SectionType } from "../sections";
import { newDocId, SCHEMA_VERSION, type Locale, type Page, type StorefrontDocument, type Tokens } from "../schema";

export const TEMPLATE_IDS = ["festival", "resort", "club", "theatre"] as const;
export type TemplateId = (typeof TEMPLATE_IDS)[number];

export type TemplateInput = {
  brandName: string;
  languages: Locale[];
  event?: { id: string; slug: string; name: Localized } | null;
};

const L = (ro: string, en: string): Localized => ({ ro, en });

type SectionSpec = { type: SectionType; settings?: Record<string, unknown>; blocks?: { type: string; settings: Record<string, unknown> }[]; hidden?: boolean };

function section(spec: SectionSpec, eventId?: string) {
  const def = SECTIONS[spec.type];
  return {
    id: newDocId(spec.type.replace("_", "")),
    type: spec.type,
    hidden: spec.hidden ?? false,
    settings: { ...def.defaults({ eventId }), ...(spec.settings ?? {}) },
    blocks: (spec.blocks ?? []).map((b) => ({ id: newDocId("b"), type: b.type, settings: b.settings })),
  };
}

function page(p: { kind: Page["kind"]; slug: string; title: Localized; eventId?: string | null; sectionEvent?: string; sections: SectionSpec[] }): Page {
  const sections = [...p.sections, { type: "footer" as const }].map((s) => section(s, p.sectionEvent ?? p.eventId ?? undefined));
  return { id: newDocId("p"), kind: p.kind, eventId: p.eventId ?? null, slug: p.slug, title: p.title, sections } as Page;
}

const TOKENS: Record<TemplateId, Tokens> = {
  festival: {
    mode: "auto",
    light: { primary: "#6d28d9", onPrimary: "#ffffff", surface: "#faf7ff", onSurface: "#1c1530", accent: "#be185d", muted: "#57506b" },
    dark: { primary: "#c4b5fd", onPrimary: "#1c1530", surface: "#120d1f", onSurface: "#f5f3ff", accent: "#f472b6", muted: "#b8b2c9" },
    fontHeading: "space_grotesk",
    fontBody: "inter",
    radius: "l",
    logo: "",
  },
  resort: {
    mode: "light",
    light: { primary: "#0e7490", onPrimary: "#ffffff", surface: "#fffbf5", onSurface: "#1f2937", accent: "#c2410c", muted: "#57534e" },
    dark: { primary: "#67e8f9", onPrimary: "#0c1a1f", surface: "#0c1a1f", onSurface: "#f1f5f9", accent: "#fdba74", muted: "#b6c2c9" },
    fontHeading: "playfair",
    fontBody: "lora",
    radius: "m",
    logo: "",
  },
  club: {
    mode: "dark",
    light: { primary: "#0e7490", onPrimary: "#ffffff", surface: "#fafafa", onSurface: "#18181b", accent: "#4d7c0f", muted: "#52525b" },
    dark: { primary: "#22d3ee", onPrimary: "#0a0a0a", surface: "#0a0a0f", onSurface: "#f4f4f5", accent: "#a3e635", muted: "#a1a1aa" },
    fontHeading: "oswald",
    fontBody: "inter",
    radius: "none",
    logo: "",
  },
  theatre: {
    mode: "light",
    light: { primary: "#7f1d1d", onPrimary: "#ffffff", surface: "#fdfaf3", onSurface: "#1c1917", accent: "#92400e", muted: "#57534e" },
    dark: { primary: "#fca5a5", onPrimary: "#1c1917", surface: "#1c1412", onSurface: "#fafaf9", accent: "#fbbf24", muted: "#c4bab4" },
    fontHeading: "dm_serif",
    fontBody: "lora",
    radius: "s",
    logo: "",
  },
};

export function templateTokens(id: TemplateId): Tokens {
  return structuredClone(TOKENS[id]);
}

const HERO_IMAGE: Record<TemplateId, string> = {
  festival: "sample:festival-stage",
  resort: "sample:resort-pool",
  club: "sample:club-lights",
  theatre: "sample:theatre-stage",
};
const GALLERY: Record<TemplateId, string[]> = {
  festival: ["sample:festival-crowd", "sample:festival-night", "sample:festival-stage"],
  resort: ["sample:resort-promenade", "sample:beach-sunset", "sample:resort-pool"],
  club: ["sample:club-crowd", "sample:club-lights", "sample:festival-night"],
  theatre: ["sample:theatre-seats", "sample:theatre-stage"],
};

const COPY: Record<TemplateId, { tagline: Localized; about: Localized; faq: [Localized, Localized][] }> = {
  festival: {
    tagline: L("Trei zile de muzică, la malul mării.", "Three days of music by the sea."),
    about: L(
      "Scene mari, artiști internaționali și camping lângă plajă. Prețurile afișate sunt **finale**, cu toate taxele incluse.",
      "Big stages, international artists and camping by the beach. Prices shown are **final**, with every fee included.",
    ),
    faq: [
      [L("Pot plăti în rate?", "Can I pay in instalments?"), L("Da, pentru abonamentele marcate cu rate.", "Yes, for passes marked with instalments.")],
      [L("Ce vârstă minimă este?", "What is the minimum age?"), L("Accesul este permis de la 16 ani.", "Entry is from 16 years.")],
      [L("Pot transfera biletul?", "Can I transfer my ticket?"), L("Da, din contul tău, până la deschiderea porților.", "Yes, from your account, until the gates open.")],
    ],
  },
  resort: {
    tagline: L("O vară întreagă de concerte, sport și relaxare.", "A whole summer of concerts, sport and relaxation."),
    about: L("Alege o zi sau tot sezonul. Fiecare zi are programul ei.", "Pick one day or the whole season. Every day has its own programme."),
    faq: [
      [L("Biletul de o zi e valabil la toate scenele?", "Is a day ticket valid on every stage?"), L("Da, pentru ziua aleasă.", "Yes, for the day you choose.")],
      [L("Există parcare?", "Is there parking?"), L("Da, parcarea se cumpără separat.", "Yes, parking is sold separately.")],
    ],
  },
  club: {
    tagline: L("O noapte. Un line-up. Fără pauze.", "One night. One line-up. No breaks."),
    about: L("Intrarea se face cu biletul din telefon. Codul se schimbă la câteva secunde.", "Entry is with the ticket on your phone. The code changes every few seconds."),
    faq: [
      [L("La ce oră se deschid ușile?", "When do the doors open?"), L("La ora afișată în program.", "At the time shown in the schedule.")],
      [L("Pot intra după miezul nopții?", "Can I come in after midnight?"), L("Da, cu același bilet.", "Yes, with the same ticket.")],
    ],
  },
  theatre: {
    tagline: L("Spectacole în fiecare săptămână.", "Shows every week."),
    about: L("Locurile sunt pe categorii. Sosește cu 15 minute înainte de început.", "Seats are by category. Please arrive 15 minutes before the start."),
    faq: [
      [L("Pot schimba data?", "Can I change the date?"), L("Da, până cu 48 de ore înainte.", "Yes, up to 48 hours before.")],
      [L("Există acces pentru scaune rulante?", "Is there wheelchair access?"), L("Da, în categoria A.", "Yes, in category A.")],
    ],
  },
};

function homeSections(id: TemplateId, input: TemplateInput, eventPath: string | null): SectionSpec[] {
  const ev = input.event;
  const copy = COPY[id];
  const hero: SectionSpec = {
    type: "hero",
    settings: { heading: L(input.brandName, input.brandName), subheading: copy.tagline, image: HERO_IMAGE[id], align: id === "theatre" ? "left" : "center" },
    blocks: [{ type: "button", settings: { label: L("Bilete", "Tickets"), href: eventPath ?? "/", style: "primary" } }],
  };
  const gallery: SectionSpec = {
    type: "gallery",
    settings: { heading: L("Galerie", "Gallery"), layout: id === "club" ? "carousel" : "grid" },
    blocks: GALLERY[id].map((image) => ({ type: "image", settings: { image, alt: L("Imagine de exemplu", "Sample image") } })),
  };
  const faq: SectionSpec = {
    type: "faq",
    blocks: copy.faq.map(([question, answer]) => ({ type: "item", settings: { question, answer } })),
  };
  const about: SectionSpec = { type: "text", settings: { heading: L("Despre", "About"), body: copy.about } };
  if (!ev) return [hero, about, gallery, faq];
  const byTemplate: Record<TemplateId, SectionSpec[]> = {
    festival: [hero, { type: "countdown" }, { type: "lineup" }, { type: "ticket_grid" }, gallery, faq, { type: "map" }],
    resort: [hero, about, { type: "calendar" }, { type: "ticket_grid" }, gallery, { type: "map" }, faq],
    club: [hero, { type: "countdown" }, { type: "lineup", settings: { heading: L("Line-up", "Line-up"), eventId: ev.id, layout: "list" } }, { type: "ticket_grid" }, gallery, faq],
    theatre: [hero, about, { type: "calendar" }, { type: "ticket_grid", settings: { heading: L("Bilete", "Tickets"), eventId: ev.id, showPerks: false } }, { type: "map" }, faq],
  };
  return byTemplate[id];
}

export function buildTemplate(id: TemplateId, input: TemplateInput): StorefrontDocument {
  const ev = input.event ?? null;
  const pages: Page[] = [
    page({ kind: "home", slug: "", title: L("Acasă", "Home"), eventId: null, sectionEvent: ev?.id, sections: homeSections(id, input, ev ? `/e/${ev.slug}` : null) }),
  ];
  if (ev) {
    pages.push(
      page({
        kind: "event",
        slug: ev.slug,
        title: { ro: ev.name.ro || ev.name.en, en: ev.name.en || ev.name.ro },
        eventId: ev.id,
        sections: [
          { type: "hero", settings: { heading: { ro: ev.name.ro || ev.name.en, en: ev.name.en || ev.name.ro }, subheading: COPY[id].tagline, image: HERO_IMAGE[id], align: "center" } },
          { type: "ticket_grid" },
          { type: id === "resort" || id === "theatre" ? "calendar" : "lineup" },
          { type: "map" },
        ],
      }),
    );
  }
  pages.push(page({ kind: "info", slug: "info", title: L("Informații", "Information"), sections: [{ type: "text", settings: { heading: L("Despre", "About"), body: COPY[id].about } }, { type: "faq", blocks: COPY[id].faq.map(([question, answer]) => ({ type: "item", settings: { question, answer } })) }] }));
  const languages = input.languages.length ? input.languages : (["ro", "en"] as Locale[]);
  return {
    schemaVersion: SCHEMA_VERSION,
    tokens: templateTokens(id),
    locales: { default: languages.includes("ro") ? "ro" : languages[0], enabled: [...languages] },
    pages,
    checkout: { showLogo: true, slots: { afterContact: [], consents: [] } },
    messages: {
      email: { footer: L(`Mulțumim că ai ales ${input.brandName}.`, `Thank you for choosing ${input.brandName}.`), showLogo: true },
      ticket: { showLogo: true, accent: "primary" },
    },
  } as StorefrontDocument;
}

/** A brand's template choice at onboarding (step 1's values) to a starting document. */
export function templateForBrand(choice: string, input: TemplateInput): StorefrontDocument {
  if (choice === "blank") {
    return {
      ...buildTemplate("festival", { ...input, event: null }),
      pages: [page({ kind: "home", slug: "", title: L("Acasă", "Home"), sections: [{ type: "hero", settings: { heading: L(input.brandName, input.brandName), subheading: L("", ""), image: "", align: "center" } }] })],
    } as StorefrontDocument;
  }
  return buildTemplate((TEMPLATE_IDS as readonly string[]).includes(choice) ? (choice as TemplateId) : "festival", input);
}

/** "Change template": the new template's pages and messages, the brand's own tokens kept (B2). */
export function changeTemplate(current: StorefrontDocument, id: TemplateId, input: TemplateInput): StorefrontDocument {
  const next = buildTemplate(id, { ...input, languages: current.locales.enabled });
  return { ...next, tokens: structuredClone(current.tokens), locales: structuredClone(current.locales), checkout: structuredClone(current.checkout) };
}
