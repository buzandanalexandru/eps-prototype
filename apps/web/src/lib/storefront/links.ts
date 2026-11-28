// Link and image rules for organiser content (design D2.2): links are https:, mailto: or
// site-relative paths; images are the organiser's uploads or the platform's sample images. Anything
// else is refused at save time, so nothing an organiser types can become script in a fan's browser.

/** Platform sample images (static files under /samples), labelled as sample imagery. */
export const SAMPLE_IMAGES = [
  "festival-stage",
  "festival-crowd",
  "festival-night",
  "beach-sunset",
  "resort-pool",
  "resort-promenade",
  "club-lights",
  "club-crowd",
  "theatre-stage",
  "theatre-seats",
  "logo-nibiru",
  "logo-nibiru-resort",
  "logo-sunwave",
] as const;
export type SampleImage = (typeof SAMPLE_IMAGES)[number];

const CONTROL = /[\u0000-\u001f\u007f\s\\]/;

/** True when `href` is an allowed link: https://host/..., mailto:addr, or /path (not //host). */
export function isSafeLink(href: string): boolean {
  if (!href || href.length > 500 || CONTROL.test(href)) return false;
  if (href.startsWith("/")) return !href.startsWith("//") && !href.startsWith("/\\");
  if (/^mailto:/i.test(href)) return /^mailto:[^@<>"'\s]+@[^@<>"'\s]+$/i.test(href);
  if (!/^https:\/\//i.test(href)) return false;
  try {
    const u = new URL(href);
    return u.protocol === "https:" && !!u.hostname && !u.username && !u.password;
  } catch {
    return false;
  }
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export type ImageRef = { kind: "asset"; id: string } | { kind: "sample"; name: SampleImage };

/** Parses "asset:<uuid>" or "sample:<name>"; null for anything else. */
export function parseImageRef(ref: string): ImageRef | null {
  if (ref.startsWith("asset:")) {
    const id = ref.slice(6);
    return UUID.test(id) ? { kind: "asset", id } : null;
  }
  if (ref.startsWith("sample:")) {
    const name = ref.slice(7);
    return (SAMPLE_IMAGES as readonly string[]).includes(name) ? { kind: "sample", name: name as SampleImage } : null;
  }
  return null;
}

/**
 * The URL an image reference is served from (always on our own origin). `previewToken` is added to
 * uploads on preview-link pages, where the draft's images are not public yet.
 */
export function imageUrl(ref: string, previewToken?: string | null): string | null {
  const r = parseImageRef(ref);
  if (!r) return null;
  if (r.kind === "sample") return `/samples/${r.name}.svg`;
  return previewToken ? `/assets/${r.id}?p=${encodeURIComponent(previewToken)}` : `/assets/${r.id}`;
}
