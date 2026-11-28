// The storefront builder (design D2.4, D2.8; B1 to B7, B10, B12). The first visit for a brand creates
// its draft from the brand's template choice; everything after that is client state saved by
// autosave and the builder's server actions.
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import Link from "next/link";
import { listBrands } from "@/lib/organisers/domain";
import { eventChoices } from "@/lib/storefront/catalogue-load";
import { ensureStorefront, listPreviewLinks, listVersions, shopContext } from "@/lib/storefront/domain";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { BuilderClient } from "@/components/builder/builder-client";

export const metadata = { title: "Builder" };

export default async function BuilderPage({ searchParams }: { searchParams: Promise<{ brand?: string }> }) {
  const { brand: brandParam } = await searchParams;
  const s = await requireStudio();
  const t = await getTranslations("builder");
  const data = await studioTx(s, async (c) => {
    const brands = await listBrands(c);
    const brand = brandParam ? brands.find((b) => b.id === brandParam) : brands[0];
    if (!brand) return null;
    const sf = await ensureStorefront(c, brand.id, s.user.id);
    const [ctx, events, versions, links] = [await shopContext(c, brand.id), await eventChoices(c, brand.id), await listVersions(c, sf.id, { limit: 50 }), await listPreviewLinks(c, sf.id)];
    return { brands, brand, sf, ctx, events, versions, links };
  });
  if (!data) notFound();
  const { brands, brand, sf, ctx, events, versions, links } = data;
  return (
    <div className="space-y-3">
      {brands.length > 1 && (
        <nav aria-label={t("brandPicker")} className="flex flex-wrap gap-2 text-sm" data-testid="builder-brands">
          {brands.map((b) => (
            <Link key={b.id} href={`/studio/builder?brand=${b.id}`} aria-current={b.id === brand.id ? "page" : undefined} className={`rounded-full border px-3 py-1 ${b.id === brand.id ? "bg-primary text-primary-foreground" : ""}`}>
              {b.name}
            </Link>
          ))}
        </nav>
      )}
      <BuilderClient
        key={sf.id}
        storefrontId={sf.id}
        brand={{ id: brand.id, name: brand.name, slug: brand.slug, currency: brand.currency, languages: brand.languages }}
        organiserName={ctx.organiser.name}
        isSample={ctx.organiser.is_sample}
        hostnames={ctx.hostnames}
        initial={{ revision: sf.draft_revision, document: sf.document }}
        live={sf.live_number ? { number: sf.live_number, publishedAt: sf.live_published_at!.toISOString() } : null}
        versions={versions.map((v) => ({ number: v.number, state: v.state, restoredFrom: v.restored_from, publishedAt: v.published_at.toISOString(), author: v.author }))}
        links={links.map((l) => ({ id: l.id, createdAt: l.created_at.toISOString(), expiresAt: l.expires_at.toISOString(), revoked: !!l.revoked_at, expired: l.expired, author: l.author }))}
        events={events}
        canDesign={s.active.role === "owner" || s.active.role === "admin"}
      />
    </div>
  );
}
