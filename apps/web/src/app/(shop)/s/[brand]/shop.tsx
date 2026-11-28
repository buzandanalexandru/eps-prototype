// The public shop's pages (design D3.5, D3.6): every page renders the live storefront version and
// the live sale snapshots, read through the cached loader (no database query on a cache hit), with
// the shared section renderer in published mode. Unknown brands, brands with only a draft and
// unknown hostnames all get the same platform 404.
import type { Metadata } from "next";
import { notFound, redirect } from "next/navigation";
import { isLocale, type Locale } from "@/i18n/locale";
import type { ShopCatalogue, ShopEvent } from "@/lib/storefront/catalogue";
import { SHOP_LABELS, type ShopLabels } from "@/lib/storefront/labels";
import type { PublishedShop, ShopKey } from "@/lib/storefront/published";
import { keyFromParam, shopBase } from "@/lib/storefront/routing";
import type { Page } from "@/lib/storefront/schema";
import { getPublishedShop } from "@/lib/storefront/shop-data";
import { shopEventFromSnapshot } from "@/lib/storefront/shop-view";
import { pagePath, StorefrontView } from "@/components/storefront/storefront";
import { shopFontClass } from "@/components/storefront/fonts";
import { FromPrice } from "@/components/storefront/shop-tickets";

export type ShopRoute = { kind: "home" } | { kind: "event"; slug: string } | { kind: "info"; slug: string } | { kind: "checkout" } | { kind: "wallet" } | { kind: "queue" };

/**
 * The shop a request names, or null. Pages are static and read no request headers: the proxy
 * refuses the ~host form on platform hosts, so it only reaches here from its own hostname.
 */
export async function shopFor(brandParam: string): Promise<{ shop: PublishedShop; key: ShopKey } | null> {
  const key = keyFromParam(brandParam);
  const shop = await getPublishedShop(key);
  return shop ? { shop, key } : null;
}

export function catalogueOf(shop: PublishedShop, now: Date): ShopCatalogue {
  const out: ShopCatalogue = {};
  for (const [id, snap] of Object.entries(shop.snapshots)) out[id] = shopEventFromSnapshot(snap, shop.maps[id] ?? null, now);
  return out;
}

function routePath(r: ShopRoute): string {
  return r.kind === "home" ? "" : r.kind === "event" ? `/e/${r.slug}` : r.kind === "info" ? `/${r.slug}` : r.kind === "wallet" ? "/wallet" : r.kind === "queue" ? "/queue" : "/checkout";
}

/**
 * Redirects to the same page in the shop's default language when the URL names one it does not
 * offer (the visitor's own preference was already applied by the proxy on /s/<brand>).
 */
export function enabledLocale(shop: PublishedShop, key: ShopKey, locale: string, route: ShopRoute, query = ""): Locale {
  const enabled = shop.document.locales.enabled;
  if (isLocale(locale) && enabled.includes(locale)) return locale;
  redirect(`${shopBase(key, shop.document.locales.default)}${routePath(route)}${query}`);
}

export function findPage(shop: PublishedShop, cat: ShopCatalogue, route: ShopRoute): Page | null {
  const pages = shop.document.pages;
  if (route.kind === "home" || route.kind === "checkout" || route.kind === "wallet" || route.kind === "queue") return pages[0] ?? null;
  if (route.kind === "event") return pages.find((p) => p.kind === "event" && !!p.eventId && cat[p.eventId]?.slug === route.slug) ?? null;
  return pages.find((p) => p.kind === "info" && p.slug === route.slug) ?? null;
}

export function SampleBanner({ labels }: { labels: ShopLabels }) {
  return (
    <div role="note" data-testid="shop-sample-banner" className="bg-[var(--eps-accent)] px-4 py-2 text-center text-sm font-semibold text-[var(--eps-on-accent,var(--eps-on-surface))]">
      {labels.sampleData}
    </div>
  );
}

const intl = (l: Locale) => (l === "ro" ? "ro-RO" : "en-GB");

function dateRange(ev: ShopEvent, locale: Locale): string {
  const f = new Intl.DateTimeFormat(intl(locale), { timeZone: ev.timezone, day: "numeric", month: "long", year: "numeric" });
  const first = ev.days[0]?.date;
  const last = ev.days.at(-1)?.date;
  if (first && last) {
    const g = new Intl.DateTimeFormat(intl(locale), { timeZone: "UTC", day: "numeric", month: "long", year: "numeric" });
    return first === last ? g.format(new Date(`${first}T12:00:00Z`)) : `${g.format(new Date(`${first}T12:00:00Z`))} – ${g.format(new Date(`${last}T12:00:00Z`))}`;
  }
  return f.format(new Date(ev.startsAt));
}

/** The event page's summary: its dates, venue and "from" price (worked out in the browser). One h1 per page: the hero's. */
export function EventSummary({ ev, locale, labels }: { ev: ShopEvent; locale: Locale; labels: ShopLabels }) {
  return (
    <div className="mx-auto max-w-5xl px-4 pt-8" data-testid="event-summary">
      <p className="font-[family-name:var(--eps-font-heading)] text-2xl font-bold break-words sm:text-3xl" data-testid="event-name">
        {ev.name[locale] || ev.name.ro || ev.name.en}
      </p>
      <dl className="mt-2 flex flex-wrap gap-x-6 gap-y-1 text-sm">
        <div>
          <dt className="sr-only">{labels.eventDates}</dt>
          <dd data-testid="event-dates">{dateRange(ev, locale)}</dd>
        </div>
        {ev.venue && (
          <div>
            <dt className="sr-only">{labels.venue}</dt>
            <dd data-testid="event-venue">{ev.venue.name}</dd>
          </div>
        )}
        <div>
          <dt className="sr-only">{labels.priceLabel}</dt>
          <dd className="font-semibold text-[var(--eps-primary)]">
            <FromPrice event={ev} labels={labels} locale={locale} />
          </dd>
        </div>
      </dl>
    </div>
  );
}

const LANG_LABEL: Record<Locale, string> = { ro: "Română", en: "English" };

export async function renderShop(
  brandParam: string,
  locale: string,
  route: ShopRoute,
  extra?: (ctx: { shop: PublishedShop; key: ShopKey; locale: Locale; cat: ShopCatalogue; now: Date; base: string }) => React.ReactNode,
  query = "",
) {
  const found = await shopFor(brandParam);
  if (!found) notFound();
  const { shop, key } = found;
  const loc = enabledLocale(shop, key, locale, route, query);
  const now = new Date();
  const cat = catalogueOf(shop, now);
  const page = findPage(shop, cat, route);
  if (!page) notFound();
  const labels = SHOP_LABELS[loc];
  const base = shopBase(key, loc);
  const tool = route.kind === "checkout" || route.kind === "wallet" || route.kind === "queue";
  const here = tool ? routePath(route) : pagePath(page, cat) === "/" ? "" : pagePath(page, cat);
  const ev = page.kind === "event" && page.eventId ? cat[page.eventId] : null;
  return (
    <StorefrontView
      doc={shop.document}
      page={tool ? { ...page, sections: page.sections.filter((s) => s.type === "footer") } : page}
      fontClass={shopFontClass}
      banner={shop.isSample ? <SampleBanner labels={labels} /> : undefined}
      localeLinks={shop.document.locales.enabled.map((l) => ({ locale: l, href: `${shopBase(key, l)}${here}${query}`, label: LANG_LABEL[l], current: l === loc }))}
      intro={tool && extra ? extra({ shop, key, locale: loc, cat, now, base }) : ev ? <EventSummary ev={ev} locale={loc} labels={labels} /> : undefined}
      ctx={{
        locale: loc,
        labels,
        catalogue: cat,
        brand: { name: shop.name, slug: shop.slug, currency: shop.currency, organiserName: shop.organiserName, isSample: shop.isSample },
        mode: "published",
        hrefBase: base,
      }}
    />
  );
}

export async function shopMetadata(brandParam: string, locale: string, route: ShopRoute): Promise<Metadata> {
  const found = await shopFor(brandParam);
  if (!found) return { title: "Not found", robots: { index: false, follow: false } };
  const { shop } = found;
  const loc: Locale = isLocale(locale) && shop.document.locales.enabled.includes(locale) ? locale : shop.document.locales.default;
  const cat = catalogueOf(shop, new Date());
  const page = findPage(shop, cat, route);
  const title = page && page.kind !== "home" ? `${page.title[loc] || page.title.ro || page.title.en} · ${shop.name}` : shop.name;
  return {
    title: { absolute: title },
    // Sample organisers' shops are never indexed (sample-data rule); the checkout never is.
    robots: shop.isSample || route.kind === "checkout" || route.kind === "wallet" || route.kind === "queue" ? { index: false, follow: false } : undefined,
    other: { "eps-storefront-version": String(shop.versionNumber) },
  };
}
