import type { Localized } from "../catalogue/schema";

/** Fan-facing text in the screen's language, falling back to the other one. */
export function lt(v: Partial<Localized> | null | undefined, locale: string): string {
  const own = (locale === "en" ? v?.en : v?.ro)?.trim();
  return own || (locale === "en" ? v?.ro : v?.en)?.trim() || "";
}
