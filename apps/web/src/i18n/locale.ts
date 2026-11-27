// Locale choice for Studio and platform pages (U5, design D0.7): the NEXT_LOCALE cookie, then
// Accept-Language, then Romanian. No URL prefix.
export const LOCALES = ["ro", "en"] as const;
export type Locale = (typeof LOCALES)[number];
export const DEFAULT_LOCALE: Locale = "ro";
export const LOCALE_COOKIE = "NEXT_LOCALE";

export function isLocale(v: string | undefined | null): v is Locale {
  return v === "ro" || v === "en";
}

export function resolveLocale(cookie: string | undefined, acceptLanguage: string | null | undefined): Locale {
  if (isLocale(cookie)) return cookie;
  if (acceptLanguage) {
    const ranked = acceptLanguage
      .split(",")
      .map((part, i) => {
        const [tag, ...params] = part.trim().split(";");
        const q = params.map((p) => p.trim()).find((p) => p.startsWith("q="));
        return { lang: tag.toLowerCase().split("-")[0], q: q ? Number(q.slice(2)) || 0 : 1, i };
      })
      .filter((x) => x.q > 0)
      .sort((a, b) => b.q - a.q || a.i - b.i);
    for (const r of ranked) if (isLocale(r.lang)) return r.lang;
  }
  return DEFAULT_LOCALE;
}
