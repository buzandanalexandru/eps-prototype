// Organiser and brand slugs (design D1.2): 3 to 40 lowercase letters, digits and hyphens, unique
// across the platform, never on the reserved list. Availability goes through the SECURITY DEFINER
// function control.slug_available, which answers only "free or not".
import type { ClientBase } from "pg";

export const RESERVED_SLUGS = new Set(["studio", "api", "s", "status", "fees", "sign-in", "admin", "eps", "www"]);
export const SLUG_PATTERN = /^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$/;

export type SlugKind = "organiser" | "brand";
export type SlugProblem = "slug_length" | "slug_characters" | "slug_hyphen" | "slug_reserved";
export type SlugCheck = SlugProblem | "slug_taken" | "ok";

/** The first rule a slug breaks, or null. Pure, so forms can say which rule before asking the database. */
export function slugProblem(slug: string): SlugProblem | null {
  if (slug.length < 3 || slug.length > 40) return "slug_length";
  if (!/^[a-z0-9-]+$/.test(slug)) return "slug_characters";
  if (slug.startsWith("-") || slug.endsWith("-")) return "slug_hyphen";
  if (RESERVED_SLUGS.has(slug)) return "slug_reserved";
  return null;
}

/** Suggests a slug from a name: lowercase, diacritics stripped, spaces to hyphens. */
export function suggestSlug(name: string): string {
  return name
    .normalize("NFD")
    .replace(/[̀-ͯ]/g, "")
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40)
    .replace(/-+$/g, "");
}

export async function checkSlug(c: ClientBase, kind: SlugKind, slug: string): Promise<SlugCheck> {
  const p = slugProblem(slug);
  if (p) return p;
  const r = await c.query<{ free: boolean }>("select control.slug_available($1, $2) as free", [kind, slug]);
  return r.rows[0]?.free ? "ok" : "slug_taken";
}
