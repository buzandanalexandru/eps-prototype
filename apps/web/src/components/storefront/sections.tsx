// The shared section renderer (design D2.2, D2.3): one component per section type, styled only with
// the storefront's CSS variables (var(--eps-*)), so the same code wears any brand. These are plain
// components with no server-only or client-only APIs: the public shop renders them as server
// components, the builder's frame renders them in the browser. Countdown and gallery hydrate.
import type { Localized } from "@/lib/catalogue/schema";
import type { ShopBrand, ShopCatalogue, ShopEvent } from "@/lib/storefront/catalogue";
import type { ShopLabels } from "@/lib/storefront/labels";
import { imageUrl } from "@/lib/storefront/links";
import { osmLink } from "@/lib/storefront/maps";
import type { Locale, Section } from "@/lib/storefront/schema";
import { formatMoney } from "@/lib/pricing/money";
import { Countdown } from "./countdown";
import { Gallery } from "./gallery";
import { shopHref } from "./href";
import { Markdown } from "./markdown";

export type RenderMode = "published" | "preview" | "link";

export type RenderCtx = {
  locale: Locale;
  labels: ShopLabels;
  catalogue: ShopCatalogue;
  brand: ShopBrand;
  mode: RenderMode;
  /** Prefix of site-relative links ("/s/<brand>/<locale>" on the public shop). */
  hrefBase: string;
  /** On preview-link pages: the link's token, so the draft's uploads load. */
  previewToken?: string | null;
};

export function tx(v: Partial<Localized> | undefined | null, locale: Locale): string {
  return (v?.[locale] ?? "").trim();
}

const intlLocale = (l: Locale) => (l === "ro" ? "ro-RO" : "en-GB");

function fmtDay(date: string, l: Locale) {
  return new Intl.DateTimeFormat(intlLocale(l), { timeZone: "UTC", weekday: "short", day: "numeric", month: "long" }).format(new Date(`${date}T12:00:00Z`));
}

const card = "rounded-[var(--eps-radius)] border border-[color-mix(in_srgb,var(--eps-on-surface)_15%,transparent)] bg-[color-mix(in_srgb,var(--eps-on-surface)_4%,var(--eps-surface))]";
const h2 = "font-[family-name:var(--eps-font-heading)] text-2xl font-bold sm:text-3xl";
const btn = "inline-flex min-h-11 items-center justify-center rounded-[var(--eps-radius)] px-5 py-2 text-sm font-semibold";
export const primaryButton = `${btn} bg-[var(--eps-primary)] text-[var(--eps-on-primary)]`;
const secondaryButton = `${btn} border-2 border-[var(--eps-primary)] text-[var(--eps-primary)]`;

function Heading({ v, ctx }: { v: unknown; ctx: RenderCtx }) {
  const text = tx(v as Localized, ctx.locale);
  return text ? <h2 className={h2}>{text}</h2> : null;
}

function Missing({ ctx }: { ctx: RenderCtx }) {
  return <p className="text-sm text-[var(--eps-muted)]">{ctx.labels.unknownEvent}</p>;
}

function eventOf(s: Section, ctx: RenderCtx): ShopEvent | null {
  const id = (s.settings as { eventId?: string }).eventId;
  return id ? (ctx.catalogue[id] ?? null) : null;
}

type P = { s: Section; ctx: RenderCtx };
type S<T> = { s: Section & { settings: T }; ctx: RenderCtx };

function Hero({ s, ctx }: S<{ heading: Localized; subheading: Localized; image: string; align: "left" | "center" }>) {
  const img = s.settings.image ? imageUrl(s.settings.image, ctx.previewToken) : null;
  const center = s.settings.align === "center";
  return (
    <div className="relative overflow-hidden">
      {img && (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={img} alt="" className="absolute inset-0 h-full w-full object-cover opacity-25" />
      )}
      <div className={`relative mx-auto flex min-h-[18rem] max-w-5xl flex-col justify-center gap-4 px-4 py-14 sm:min-h-[24rem] ${center ? "items-center text-center" : "items-start"}`}>
        <h1 className="font-[family-name:var(--eps-font-heading)] text-4xl font-bold break-words sm:text-6xl">{tx(s.settings.heading, ctx.locale)}</h1>
        {tx(s.settings.subheading, ctx.locale) && <p className="max-w-2xl text-lg">{tx(s.settings.subheading, ctx.locale)}</p>}
        {s.blocks.length > 0 && (
          <div className="flex flex-wrap gap-3">
            {s.blocks.map((b) => {
              const st = b.settings as { label: Localized; href: string; style: "primary" | "secondary" };
              return (
                <a key={b.id} data-eps-block={b.id} href={shopHref(st.href, ctx.hrefBase)} className={st.style === "primary" ? primaryButton : secondaryButton}>
                  {tx(st.label, ctx.locale)}
                </a>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
}

function Lineup({ s, ctx }: S<{ heading: Localized; layout: "grid" | "list" }>) {
  const ev = eventOf(s, ctx);
  const byDay = new Map<string, ShopEvent["lineup"]>();
  for (const l of ev?.lineup ?? []) byDay.set(l.date, [...(byDay.get(l.date) ?? []), l]);
  return (
    <Wrap>
      <Heading v={s.settings.heading} ctx={ctx} />
      {!ev ? (
        <Missing ctx={ctx} />
      ) : !ev.lineup.length ? (
        <p className="text-[var(--eps-muted)]">{ctx.labels.lineupEmpty}</p>
      ) : (
        <div className="space-y-6">
          {[...byDay].map(([date, slots]) => (
            <div key={date} className="space-y-2">
              <h3 className="border-l-4 border-[var(--eps-accent)] pl-2 text-sm font-semibold tracking-wide uppercase">{fmtDay(date, ctx.locale)}</h3>
              <ul className={s.settings.layout === "grid" ? "grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3" : "divide-y divide-[color-mix(in_srgb,var(--eps-on-surface)_15%,transparent)]"}>
                {slots.map((l, i) => (
                  <li key={i} className={s.settings.layout === "grid" ? `${card} p-4` : "flex flex-wrap justify-between gap-2 py-2"}>
                    <span className="font-[family-name:var(--eps-font-heading)] text-lg font-semibold">{l.artist}</span>
                    <span className="block text-sm text-[var(--eps-muted)]">
                      {l.starts}
                      {l.stage ? ` · ${l.stage}` : ""}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
      )}
    </Wrap>
  );
}

function TicketGrid({ s, ctx }: S<{ heading: Localized; showPerks: boolean }>) {
  const ev = eventOf(s, ctx);
  const buyLabel = ctx.mode === "published" ? ctx.labels.checkoutSoon : ctx.labels.availableAfterPublish;
  return (
    <Wrap id="tickets">
      <Heading v={s.settings.heading} ctx={ctx} />
      {!ev ? (
        <Missing ctx={ctx} />
      ) : !ev.products.length ? (
        <p className="text-[var(--eps-muted)]">{ctx.labels.noProducts}</p>
      ) : (
        <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3" data-testid="ticket-grid">
          {ev.products.map((p) => (
            <li key={p.id} className={`${card} flex min-w-0 flex-col gap-3 p-5`} data-testid="ticket-card">
              <h3 className="font-[family-name:var(--eps-font-heading)] text-xl font-semibold break-words">{tx(p.name, ctx.locale) || tx(p.name, ctx.locale === "ro" ? "en" : "ro")}</h3>
              {s.settings.showPerks && p.perks.length > 0 && (
                <ul className="ml-5 list-disc space-y-1 text-sm">
                  {p.perks.map((k, i) => (
                    <li key={i}>{tx(k, ctx.locale)}</li>
                  ))}
                </ul>
              )}
              <div className="mt-auto">
                <p className="text-2xl font-bold text-[var(--eps-primary)]" data-testid="ticket-price">
                  {formatMoney(p.allInMinor, ev.currency, ctx.locale)}
                </p>
                <p className="text-xs text-[var(--eps-muted)]">{p.fanFeeMinor > 0 ? `${ctx.labels.allIn} · ${ctx.labels.includesFee}` : ctx.labels.allIn}</p>
              </div>
              <button type="button" disabled className={`${primaryButton} cursor-not-allowed opacity-80`} data-testid="buy-disabled">
                {buyLabel}
              </button>
            </li>
          ))}
        </ul>
      )}
    </Wrap>
  );
}

function Calendar({ s, ctx }: S<{ heading: Localized }>) {
  const ev = eventOf(s, ctx);
  return (
    <Wrap>
      <Heading v={s.settings.heading} ctx={ctx} />
      {!ev ? (
        <Missing ctx={ctx} />
      ) : (
        <ul className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-6">
          {ev.days.slice(0, 120).map((d) => (
            <li key={d.date} className={`${card} p-3 text-sm`}>
              <span className="block font-semibold">{fmtDay(d.date, ctx.locale)}</span>
              <span className="text-[var(--eps-muted)]">
                {d.opens}–{d.closes}
              </span>
            </li>
          ))}
        </ul>
      )}
    </Wrap>
  );
}

function MapSection({ s, ctx }: S<{ heading: Localized }>) {
  const ev = eventOf(s, ctx);
  const v = ev?.venue;
  const hasPoint = v && v.lat !== null && v.lng !== null;
  return (
    <Wrap>
      <Heading v={s.settings.heading} ctx={ctx} />
      {!ev ? (
        <Missing ctx={ctx} />
      ) : (
        <div className={`${card} grid gap-4 overflow-hidden p-4 ${v?.map ? "sm:grid-cols-[256px_1fr]" : ""}`}>
          {v?.map && (
            // A static map stored on our own origin and a link out: the fan's browser makes no
            // request to a map service (review decision on step 2).
            // eslint-disable-next-line @next/next/no-img-element
            <img src={v.map} alt="" width={256} height={256} loading="lazy" className="h-auto w-full max-w-64 rounded-[var(--eps-radius)]" data-testid="map-image" />
          )}
          <div className="min-w-0 space-y-2">
            <p className="font-semibold">{v?.name}</p>
            <p className="text-sm text-[var(--eps-muted)]">{v?.address}</p>
            {hasPoint && (
              <a className="text-sm font-semibold underline" rel="noopener noreferrer" href={osmLink(v!.lat!, v!.lng!)}>
                {ctx.labels.map}
              </a>
            )}
            {v?.map && <p className="text-xs text-[var(--eps-muted)]">{ctx.labels.mapAttribution}</p>}
          </div>
        </div>
      )}
    </Wrap>
  );
}

function Faq({ s, ctx }: S<{ heading: Localized }>) {
  return (
    <Wrap>
      <Heading v={s.settings.heading} ctx={ctx} />
      <div className="space-y-2">
        {s.blocks.map((b) => {
          const st = b.settings as { question: Localized; answer: Localized };
          return (
            <details key={b.id} data-eps-block={b.id} className={`${card} p-4`}>
              <summary className="cursor-pointer font-semibold">{tx(st.question, ctx.locale)}</summary>
              <Markdown source={tx(st.answer, ctx.locale)} hrefBase={ctx.hrefBase} className="mt-2 text-[var(--eps-on-surface)]" />
            </details>
          );
        })}
      </div>
    </Wrap>
  );
}

function GallerySection({ s, ctx }: S<{ heading: Localized; layout: "grid" | "carousel" }>) {
  const images = s.blocks.flatMap((b) => {
    const st = b.settings as { image: string; alt: Localized };
    const src = imageUrl(st.image, ctx.previewToken);
    return src ? [{ id: b.id, src, alt: tx(st.alt, ctx.locale), sample: st.image.startsWith("sample:") }] : [];
  });
  return (
    <Wrap>
      <Heading v={s.settings.heading} ctx={ctx} />
      <Gallery images={images} layout={s.settings.layout} labels={{ close: ctx.labels.close, previous: ctx.labels.previous, next: ctx.labels.next, sample: ctx.labels.sampleImage }} />
    </Wrap>
  );
}

function CountdownSection({ s, ctx }: S<{ heading: Localized; until: string }>) {
  const ev = eventOf(s, ctx);
  const target = ev?.startsAt ?? (s.settings.until || null);
  return (
    <Wrap>
      <Heading v={s.settings.heading} ctx={ctx} />
      {target ? (
        <Countdown target={target} labels={{ days: ctx.labels.days, hours: ctx.labels.hours, minutes: ctx.labels.minutes, seconds: ctx.labels.seconds, started: ctx.labels.started }} />
      ) : (
        <Missing ctx={ctx} />
      )}
    </Wrap>
  );
}

function Text({ s, ctx }: S<{ heading: Localized; body: Localized; align: "left" | "center" }>) {
  return (
    <Wrap>
      <div className={`max-w-3xl space-y-3 ${s.settings.align === "center" ? "mx-auto text-center" : ""}`}>
        <Heading v={s.settings.heading} ctx={ctx} />
        <Markdown source={tx(s.settings.body, ctx.locale)} hrefBase={ctx.hrefBase} className="text-lg leading-relaxed" />
      </div>
    </Wrap>
  );
}

/** The locked footer: organiser, fees and terms links, and the sample-data label. */
function Footer({ s, ctx }: S<{ note: Localized }>) {
  return (
    <footer className="border-t border-[color-mix(in_srgb,var(--eps-on-surface)_15%,transparent)]">
      <div className="mx-auto flex max-w-5xl flex-col gap-3 px-4 py-8 text-sm sm:flex-row sm:flex-wrap sm:items-center sm:justify-between">
        <div className="min-w-0 space-y-1">
          <p className="font-semibold">© {ctx.brand.organiserName}</p>
          {tx(s.settings.note, ctx.locale) && <p className="text-[var(--eps-muted)]">{tx(s.settings.note, ctx.locale)}</p>}
          {ctx.brand.isSample && (
            <p data-testid="shop-sample-label" className="inline-block rounded-[var(--eps-radius)] border border-[var(--eps-accent)] px-2 py-0.5 text-xs font-semibold">
              {ctx.labels.sampleData}
            </p>
          )}
        </div>
        <nav className="flex flex-wrap gap-4">
          <a href="/fees" className="underline">
            {ctx.labels.fees}
          </a>
          <a href="/terms" className="underline">
            {ctx.labels.terms}
          </a>
          <span className="text-[var(--eps-muted)]">{ctx.labels.poweredBy}</span>
        </nav>
      </div>
    </footer>
  );
}

function Wrap({ children, id }: { children: React.ReactNode; id?: string }) {
  return (
    <div id={id} className="mx-auto max-w-5xl space-y-5 px-4 py-10">
      {children}
    </div>
  );
}

const RENDERERS: Record<Section["type"], (p: P) => React.ReactNode> = {
  hero: Hero as (p: P) => React.ReactNode,
  lineup: Lineup as (p: P) => React.ReactNode,
  ticket_grid: TicketGrid as (p: P) => React.ReactNode,
  calendar: Calendar as (p: P) => React.ReactNode,
  map: MapSection as (p: P) => React.ReactNode,
  faq: Faq as (p: P) => React.ReactNode,
  gallery: GallerySection as (p: P) => React.ReactNode,
  countdown: CountdownSection as (p: P) => React.ReactNode,
  text: Text as (p: P) => React.ReactNode,
  footer: Footer as (p: P) => React.ReactNode,
};

export function SectionView({ s, ctx, selected }: P & { selected?: boolean }) {
  const R = RENDERERS[s.type];
  if (!R || s.hidden) return null;
  return (
    <section
      data-eps-section={s.id}
      data-eps-type={s.type}
      className={selected ? "outline-2 -outline-offset-2 outline-[var(--eps-accent)] outline-dashed" : undefined}
    >
      <R s={s} ctx={ctx} />
    </section>
  );
}
