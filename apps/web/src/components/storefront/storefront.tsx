// The storefront root (design D2.3): puts the brand's tokens on one element as CSS variables and
// renders a page's sections with the shared renderer. Used by preview links, the builder frame and
// (step 3) the public shop.
import { tokenVars } from "@/lib/storefront/tokens";
import { imageUrl } from "@/lib/storefront/links";
import type { ShopCatalogue } from "@/lib/storefront/catalogue";
import type { Page, Section, StorefrontDocument } from "@/lib/storefront/schema";
import { SectionView, tx, type RenderCtx } from "./sections";

/**
 * A page's address inside the shop (spec-map): "/" for home, "/e/<event slug>" for an event page
 * (by the event's own slug, from the catalogue), "/<page slug>" for an info page.
 */
export function pagePath(p: Pick<Page, "kind" | "slug" | "eventId">, catalogue: ShopCatalogue): string {
  if (p.kind === "home") return "/";
  if (p.kind === "event") return `/e/${(p.eventId && catalogue[p.eventId]?.slug) || ""}`;
  return `/${p.slug}`;
}

export type StorefrontViewProps = {
  doc: StorefrontDocument;
  page: Page;
  ctx: RenderCtx;
  fontClass?: string;
  selectedSectionId?: string | null;
  /** Forces one palette in the builder ("auto" brands previewed as dark). */
  scheme?: "light" | "dark" | null;
  banner?: React.ReactNode;
  /** Published shop: links to this page in the shop's other languages. */
  localeLinks?: { locale: string; href: string; label: string; current: boolean }[];
  /** Published event pages: the event's summary above its sections. */
  intro?: React.ReactNode;
};

export function StorefrontView({ doc, page, ctx, fontClass, selectedSectionId, scheme, banner, localeLinks, intro }: StorefrontViewProps) {
  const logo = doc.tokens.logo ? imageUrl(doc.tokens.logo, ctx.previewToken) : null;
  const nav = doc.pages.filter((p) => p.kind !== "home");
  const home = doc.pages[0];
  return (
    <div
      className={`eps-shop min-h-full overflow-x-hidden bg-[var(--eps-surface)] font-[family-name:var(--eps-font-body)] text-[var(--eps-on-surface)] ${fontClass ?? ""}`}
      data-eps-mode={scheme ?? doc.tokens.mode}
      style={tokenVars(doc.tokens) as React.CSSProperties}
      lang={ctx.locale}
      data-testid="storefront"
    >
      {banner}
      <header className="border-b border-[color-mix(in_srgb,var(--eps-on-surface)_15%,transparent)]">
        <div className="mx-auto flex max-w-5xl flex-wrap items-center justify-between gap-3 px-4 py-3">
          <a href={ctx.hrefBase || "/"} data-eps-page={home.id} className="flex min-w-0 items-center gap-2 font-[family-name:var(--eps-font-heading)] text-lg font-bold">
            {logo && (
              // eslint-disable-next-line @next/next/no-img-element
              <img src={logo} alt="" className="h-9 w-auto max-w-32 object-contain" data-testid="shop-logo" />
            )}
            <span className="truncate">{ctx.brand.name}</span>
          </a>
          {nav.length > 0 && (
            <nav aria-label={ctx.labels.navigation} className="flex min-w-0 flex-wrap gap-x-4 gap-y-1 text-sm">
              {nav.map((p) => (
                <a key={p.id} href={`${ctx.hrefBase}${pagePath(p, ctx.catalogue)}`} data-eps-page={p.id} className={p.id === page.id ? "font-semibold underline decoration-[var(--eps-primary)] decoration-2 underline-offset-4" : "hover:underline"}>
                  {tx(p.title, ctx.locale)}
                </a>
              ))}
            </nav>
          )}
          {localeLinks && localeLinks.length > 1 && (
            <nav aria-label={ctx.labels.languages} className="flex gap-2 text-sm" data-testid="shop-languages">
              {localeLinks.map((l) => (
                <a key={l.locale} href={l.href} hrefLang={l.locale} lang={l.locale} aria-current={l.current ? "page" : undefined} className={l.current ? "font-semibold underline underline-offset-4" : "hover:underline"} data-testid={`shop-lang-${l.locale}`}>
                  {l.label}
                </a>
              ))}
            </nav>
          )}
        </div>
      </header>
      <main>
        {intro}
        {(page.sections as Section[]).map((s) => (
          <SectionView key={s.id} s={s} ctx={ctx} selected={s.id === selectedSectionId} />
        ))}
      </main>
    </div>
  );
}

/** The "Preview, not published" banner of preview links (D2.5). */
export function PreviewBanner({ title, lead }: { title: string; lead: string }) {
  return (
    <div role="status" data-testid="preview-banner" className="bg-[var(--eps-on-surface)] px-4 py-2 text-center text-sm text-[var(--eps-surface)]">
      <strong>{title}</strong> · {lead}
    </div>
  );
}
