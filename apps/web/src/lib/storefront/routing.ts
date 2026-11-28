// Shop addresses (design D3.5): /s/<brand>/<locale>/... on platform hosts, and /<locale>/... at the
// root of a brand's configured hostname, which the proxy rewrites to /s/~<host>/<locale>/... (a
// form no brand slug can take). Every link inside a shop is built from shopBase, so links stay on
// whichever form the visitor used. Pure: the proxy, the pages and the tests share it.
import { isLocale, type Locale } from "@/i18n/locale";
import type { ShopKey } from "./published";

export const HOST_MARKER = "~";

/** The [brand] route parameter → a slug, or a hostname for the rewritten host form. */
export function keyFromParam(param: string): ShopKey {
  const p = decodeURIComponent(param);
  return p.startsWith(HOST_MARKER) ? { host: p.slice(1).toLowerCase() } : { slug: p };
}

/** The prefix of every link inside the shop for this locale. */
export function shopBase(key: ShopKey, locale: Locale): string {
  return "host" in key ? `/${locale}` : `/s/${key.slug}/${locale}`;
}

/** Paths a brand host serves as they are (assets, the app's own files and APIs, platform legal pages). */
const PASS_THROUGH = ["/assets/", "/samples/", "/_next/", "/api/", "/favicon.ico", "/fees", "/terms"];

/** On a brand hostname: the internal shop path a request is served from, or null to serve it as is. */
export function hostRewrite(pathname: string, host: string): string | null {
  if (PASS_THROUGH.some((p) => pathname === p || pathname.startsWith(p.endsWith("/") ? p : `${p}/`) || pathname === p.replace(/\/$/, ""))) return null;
  return `/s/${HOST_MARKER}${host}${pathname === "/" ? "" : pathname}`;
}

/** The shop locale of an internal shop path (/s/<brand>/<ro|en>/...), for the page's language. */
export function shopLocaleOfPath(pathname: string): Locale | null {
  const m = /^\/s\/[^/]+\/(ro|en)(?:\/|$)/.exec(pathname);
  return m ? (m[1] as Locale) : null;
}

/** Browser languages by preference (q values), primary tags only. */
function ranked(acceptLanguage: string): string[] {
  return acceptLanguage
    .split(",")
    .slice(0, 20)
    .map((part, i) => {
      const [tag, ...params] = part.trim().split(";");
      const q = params.map((x) => x.trim()).find((x) => x.startsWith("q="));
      return { lang: tag.toLowerCase().split("-")[0], q: q ? Number(q.slice(2)) || 0 : 1, i };
    })
    .filter((x) => x.q > 0)
    .sort((a, b) => b.q - a.q || a.i - b.i)
    .map((x) => x.lang);
}

/** The shop's language for a visitor: cookie, then browser language, then the shop default, among the shop's languages. */
export function pickShopLocale(p: { cookie?: string; acceptLanguage?: string | null; enabled: Locale[]; fallback: Locale }): Locale {
  if (isLocale(p.cookie) && p.enabled.includes(p.cookie)) return p.cookie;
  for (const l of ranked(p.acceptLanguage ?? "")) if (isLocale(l) && p.enabled.includes(l)) return l;
  return p.enabled.includes(p.fallback) ? p.fallback : p.enabled[0];
}
