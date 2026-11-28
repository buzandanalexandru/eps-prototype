// The public shop's pages (design D3.5, D3.6): every page renders the live storefront version and
// the live sale snapshots, read through the cached loader (no database query on a cache hit), with
// the shared section renderer in published mode. Unknown brands, brands with only a draft and
// unknown hostnames all get the same platform 404.
import type { Metadata } from "next";
import { cookies, headers } from "next/headers";
import { notFound, redirect } from "next/navigation";
import { LOCALE_COOKIE, isLocale, type Locale } from "@/i18n/locale";
import type { ShopCatalogue, ShopEvent } from "@/lib/storefront/catalogue";
import { fill, SHOP_LABELS, type ShopLabels } from "@/lib/storefront/labels";
import type { PublishedShop, ShopKey } from "@/lib/storefront/published";
import { keyFromParam, pickShopLocale, shopBase } from "@/lib/storefront/routing";
import type { Page } from "@/lib/storefront/schema";
import { getPublishedShop } from "@/lib/storefront/shop-data";
import { fromPrice, shopEventFromSnapshot } from "@/lib/storefront/shop-view";
import { pagePath, StorefrontView } from "@/components/storefront/storefront";
import { shopFontClass } from "@/components/storefront/fonts";
import { money, shopDate } from "@/components/storefront/all-in-price";

export type ShopRoute = { kind: "home" } | { kind: "event"; slug: string } | { kind: "info"; slug: string } | { kind: "checkout" };

/** The shop a request names, or null (a host form only counts on that host: the proxy set it). */
export async function shopFor(brandParam: string): Promise<{ shop: PublishedShop; key: ShopKey } | null> {
  const key = keyFromParam(brandParam);
  if ("host" in key) {
    const host = (await headers()).get("x-eps-brand-host");
    if (!host || host !== key.host) return null;
  }
  const shop = await getPublishedShop(key);
  return shop ? { shop, key } : null;
}

export function catalogueOf(shop: PublishedShop, now: Date): ShopCatalogue {
  const out: ShopCatalogue = {};
  for (const [id, snap] of Object.entries(shop.snapshots)) out[id] = shopEventFromSnapshot(snap, shop.maps[id] ?? null, now);
  return out;
}

function routePath(r: ShopRoute): string {
  return r.kind === "home" ? "" : r.kind === "event" ? `/e/${r.slug}` : r.kind === "info" ? `/${r.slug}` : "/checkout";
}

/** Redirects to the same page in a language the shop offers, when the URL names another. */
export async function enabledLocale(shop: PublishedShop, key: ShopKey, locale: string, route: ShopRoute): Promise<Locale> {
  const enabled = shop.document.locales.enabled;
  if (isLocale(locale) && enabled.includes(locale)) return locale;
  const h = await headers();
  const to = pickShopLocale({ cookie: (await cookies()).get(LOCALE_COOKIE)?.value, acceptLanguage: h.get("accept-language"), enabled, fallback: shop.document.locales.default });
  redirect(`${shopBase(key, to)}${routePath(route)}`);
}

export function findPage(shop: PublishedShop, cat: ShopCatalogue, route: ShopRoute): Page | null {
  const pages = shop.document.pages;
  if (route.kind === "home" || route.kind === "checkout") return pages[0] ?? null;
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

/** The event page's summary: dates, venue and the "from" price (all-in). */
export function EventSummary({ ev, locale, labels, now }: { ev: ShopEvent; locale: Locale; labels: ShopLabels; now: Date }) {
  const from = fromPrice(ev, now);
  const upcoming = ev.products.filter((p) => p.sale?.isTicket && p.sale.salesFrom && Date.parse(p.sale.salesFrom) > now.getTime()).map((p) => p.sale!.salesFrom!).sort()[0];
  return (
    <div className="mx-auto max-w-5xl px-4 pt-8" data-testid="event-summary">
      <h1 className="font-[family-name:var(--eps-font-heading)] text-3xl font-bold break-words sm:text-4xl">{ev.name[locale] || ev.name.ro || ev.name.en}</h1>
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
        {from && (
          <div>
            <dt className="sr-only">{labels.from}</dt>
            <dd className="font-semibold text-[var(--eps-primary)]" data-testid="from-price">
              {from.upcoming && upcoming
                ? fill(labels.upcomingFrom, { price: money(from.allInMinor, ev.currency, locale), date: shopDate(upcoming, locale, ev.timezone) })
                : fill(labels.fromPrice, { price: money(from.allInMinor, ev.currency, locale) })}
            </dd>
          </div>
        )}
      </dl>
    </div>
  );
}

const LANG_LABEL: Record<Locale, string> = { ro: "Română", en: "English" };

export async function renderShop(brandParam: string, locale: string, route: ShopRoute, extra?: (ctx: { shop: PublishedShop; key: ShopKey; locale: Locale; cat: ShopCatalogue; now: Date; base: string }) => React.ReactNode) {
  const found = await shopFor(brandParam);
  if (!found) notFound();
  const { shop, key } = found;
  const loc = await enabledLocale(shop, key, locale, route);
  const now = new Date();
  const cat = catalogueOf(shop, now);
  const page = findPage(shop, cat, route);
  if (!page) notFound();
  const labels = SHOP_LABELS[loc];
  const base = shopBase(key, loc);
  const here = route.kind === "checkout" ? "/checkout" : pagePath(page, cat) === "/" ? "" : pagePath(page, cat);
  const ev = page.kind === "event" && page.eventId ? cat[page.eventId] : null;
  return (
    <StorefrontView
      doc={shop.document}
      page={route.kind === "checkout" ? { ...page, sections: page.sections.filter((s) => s.type === "footer") } : page}
      fontClass={shopFontClass}
      banner={shop.isSample ? <SampleBanner labels={labels} /> : undefined}
      localeLinks={shop.document.locales.enabled.map((l) => ({ locale: l, href: `${shopBase(key, l)}${here}`, label: LANG_LABEL[l], current: l === loc }))}
      intro={route.kind === "checkout" && extra ? extra({ shop, key, locale: loc, cat, now, base }) : ev ? <EventSummary ev={ev} locale={loc} labels={labels} now={now} /> : undefined}
      ctx={{
        locale: loc,
        labels,
        catalogue: cat,
        brand: { name: shop.name, slug: shop.slug, currency: shop.currency, organiserName: shop.organiserName, isSample: shop.isSample },
        mode: "published",
        hrefBase: base,
        now: now.toISOString(),
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
    robots: shop.isSample || route.kind === "checkout" ? { index: false, follow: false } : undefined,
    other: { "eps-storefront-version": String(shop.versionNumber) },
  };
}
