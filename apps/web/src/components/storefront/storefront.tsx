// The storefront root (design D2.3): puts the brand's tokens on one element as CSS variables and
// renders a page's sections with the shared renderer. Used by preview links, the builder frame and
// (step 3) the public shop.
import { tokenVars } from "@/lib/storefront/tokens";
import { imageUrl } from "@/lib/storefront/links";
import type { Page, Section, StorefrontDocument } from "@/lib/storefront/schema";
import { SectionView, tx, type RenderCtx } from "./sections";

export function pagePath(p: Pick<Page, "kind" | "slug">): string {
  return p.kind === "home" ? "/" : p.kind === "event" ? `/e/${p.slug}` : `/${p.slug}`;
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
};

export function StorefrontView({ doc, page, ctx, fontClass, selectedSectionId, scheme, banner }: StorefrontViewProps) {
  const logo = doc.tokens.logo ? imageUrl(doc.tokens.logo) : null;
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
                <a key={p.id} href={`${ctx.hrefBase}${pagePath(p)}`} data-eps-page={p.id} className={p.id === page.id ? "font-semibold text-[var(--eps-primary)] underline" : "hover:underline"}>
                  {tx(p.title, ctx.locale)}
                </a>
              ))}
            </nav>
          )}
        </div>
      </header>
      <main>
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
