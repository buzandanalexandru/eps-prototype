// Private preview links (design D2.5): anyone holding the token sees the current draft, marked
// "Preview, not published", never indexed or cached, with buying disabled. Unknown, expired and
// revoked tokens are all the same 404.
import type { Metadata } from "next";
import { cookies, headers } from "next/headers";
import { notFound } from "next/navigation";
import { getAppPool } from "@/lib/db/pool";
import { withTx } from "@/lib/db/tx";
import { LOCALE_COOKIE, resolveLocale } from "@/i18n/locale";
import { loadShopCatalogue } from "@/lib/storefront/catalogue-load";
import { getStorefront, resolvePreviewToken, shopContext } from "@/lib/storefront/domain";
import { SHOP_LABELS } from "@/lib/storefront/labels";
import { referencedEvents, type Locale } from "@/lib/storefront/schema";
import { pagePath, PreviewBanner, StorefrontView } from "@/components/storefront/storefront";
import { shopFontClass } from "@/components/storefront/fonts";

export const dynamic = "force-dynamic";
export const metadata: Metadata = { title: "Preview", robots: { index: false, follow: false, nocache: true } };

export default async function PreviewLinkPage({ params, searchParams }: { params: Promise<{ token: string }>; searchParams: Promise<{ page?: string; lang?: string }> }) {
  const { token } = await params;
  const sp = await searchParams;
  const pool = getAppPool();
  const target = await withTx(pool, {}, (c) => resolvePreviewToken(c, token));
  if (!target) notFound();
  const data = await withTx(pool, { organiserId: target.organiser_id }, async (c) => {
    const sf = await getStorefront(c, target.storefront_id);
    if (!sf) return null;
    return { sf, ctx: await shopContext(c, sf.brand_id), catalogue: await loadShopCatalogue(c, referencedEvents(sf.document)) };
  });
  if (!data) notFound();
  const doc = data.sf.document;
  const preferred = sp.lang === "ro" || sp.lang === "en" ? sp.lang : resolveLocale((await cookies()).get(LOCALE_COOKIE)?.value, (await headers()).get("accept-language"));
  const locale: Locale = doc.locales.enabled.includes(preferred) ? preferred : doc.locales.default;
  const page = doc.pages.find((p) => pagePath(p, data.catalogue) === (sp.page || "/")) ?? doc.pages[0];
  const labels = SHOP_LABELS[locale];
  return (
    <StorefrontView
      doc={doc}
      page={page}
      fontClass={shopFontClass}
      banner={<PreviewBanner title={labels.previewBanner} lead={labels.previewBannerLead} />}
      ctx={{
        locale,
        labels,
        catalogue: data.catalogue,
        brand: { name: data.ctx.brand.name, slug: data.ctx.brand.slug, currency: data.ctx.brand.currency, organiserName: data.ctx.organiser.name, isSample: data.ctx.organiser.is_sample },
        mode: "link",
        hrefBase: `/p/${token}?page=`,
        previewToken: token,
      }}
    />
  );
}
