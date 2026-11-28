import { isSafeLink } from "@/lib/storefront/links";

/** A document link on the shop: site-relative paths are under the shop's base; unsafe ones go nowhere. */
export function shopHref(href: string, base: string): string {
  if (!isSafeLink(href)) return "#";
  if (href.startsWith("/")) return base ? (href === "/" ? base : `${base}${href}`) : href;
  return href;
}
