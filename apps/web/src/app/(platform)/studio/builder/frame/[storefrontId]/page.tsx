// The builder's preview frame (design D2.4): loaded same-origin inside the builder, it renders the
// draft it receives by postMessage with the shared renderer and the organiser's live catalogue.
import { notFound } from "next/navigation";
import { getLocale } from "next-intl/server";
import { loadShopCatalogue } from "@/lib/storefront/catalogue-load";
import { getStorefront, shopContext } from "@/lib/storefront/domain";
import { referencedEvents } from "@/lib/storefront/schema";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { FrameClient } from "@/components/builder/frame-client";

export const metadata = { title: "Preview", robots: { index: false, follow: false } };

export default async function FramePage({ params, searchParams }: { params: Promise<{ storefrontId: string }>; searchParams: Promise<{ page?: string }> }) {
  const { storefrontId } = await params;
  const { page } = await searchParams;
  const s = await requireStudio();
  const data = await studioTx(s, async (c) => {
    const sf = await getStorefront(c, storefrontId);
    if (!sf) return null;
    const ctx = await shopContext(c, sf.brand_id);
    return { sf, ctx, catalogue: await loadShopCatalogue(c, referencedEvents(sf.document)) };
  });
  if (!data) notFound();
  const locale = await getLocale();
  const doc = data.sf.document;
  return (
    <FrameClient
      storefrontId={storefrontId}
      initialDoc={doc}
      initialPageId={doc.pages.find((p) => p.id === page)?.id ?? doc.pages[0].id}
      initialLocale={doc.locales.enabled.includes(locale as "ro" | "en") ? (locale as "ro" | "en") : doc.locales.default}
      initialCatalogue={data.catalogue}
      brand={{ name: data.ctx.brand.name, slug: data.ctx.brand.slug, currency: data.ctx.brand.currency, organiserName: data.ctx.organiser.name, isSample: data.ctx.organiser.is_sample }}
    />
  );
}
