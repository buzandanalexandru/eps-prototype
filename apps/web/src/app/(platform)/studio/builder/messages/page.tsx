// Email and ticket previews with the brand kit on a sample order (design D2.7; B12).
import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import Link from "next/link";
import { listBrands } from "@/lib/organisers/domain";
import { getStorefrontByBrand } from "@/lib/storefront/domain";
import { SHOP_LABELS } from "@/lib/storefront/labels";
import type { Locale } from "@/lib/storefront/schema";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { ConfirmationEmail, sampleOrder, TicketPreview } from "@/components/storefront/messages";

export const metadata = { title: "Email and ticket" };

export default async function MessagesPage({ searchParams }: { searchParams: Promise<{ brand?: string; lang?: string }> }) {
  const sp = await searchParams;
  const s = await requireStudio();
  const t = await getTranslations("builder");
  const data = await studioTx(s, async (c) => {
    const brands = await listBrands(c);
    const brand = sp.brand ? brands.find((b) => b.id === sp.brand) : brands[0];
    if (!brand) return null;
    const sf = await getStorefrontByBrand(c, brand.id);
    return sf ? { brand, sf } : null;
  });
  if (!data) notFound();
  const ui = await getLocale();
  const wanted = (sp.lang === "ro" || sp.lang === "en" ? sp.lang : ui) as Locale;
  const doc = data.sf.document;
  const locale: Locale = doc.locales.enabled.includes(wanted) ? wanted : doc.locales.default;
  const order = sampleOrder(data.brand.name, data.brand.currency, locale);
  return (
    <div className="mx-auto max-w-4xl space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-2xl font-bold">{t("messagesPageTitle", { brand: data.brand.name })}</h1>
        <div className="flex gap-3 text-sm">
          {doc.locales.enabled.map((l) => (
            <Link key={l} href={`/studio/builder/messages?brand=${data.brand.id}&lang=${l}`} aria-current={l === locale ? "true" : undefined} className={l === locale ? "font-semibold underline" : "underline"}>
              {l.toUpperCase()}
            </Link>
          ))}
          <Link href={`/studio/builder?brand=${data.brand.id}`} className="underline">
            {t("backToBuilder")}
          </Link>
        </div>
      </div>
      <p className="text-sm text-muted-foreground">{t("messagesHint")}</p>
      <section className="space-y-2">
        <h2 className="font-semibold">{t("emailPreview")}</h2>
        <div className="overflow-hidden rounded-md border">
          <ConfirmationEmail doc={doc} labels={SHOP_LABELS[locale]} locale={locale} brandName={data.brand.name} order={order} />
        </div>
      </section>
      <section className="space-y-2">
        <h2 className="font-semibold">{t("ticketPreview")}</h2>
        <TicketPreview doc={doc} labels={SHOP_LABELS[locale]} brandName={data.brand.name} order={order} />
      </section>
    </div>
  );
}
