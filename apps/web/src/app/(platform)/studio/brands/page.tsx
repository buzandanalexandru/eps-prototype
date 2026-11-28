import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { listBrands } from "@/lib/organisers/domain";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { ActionForm } from "@/components/studio/action-form";
import { BrandFields } from "@/components/studio/brand-fields";
import { Card } from "@/components/ui/field";
import { saveBrandAction } from "../actions";

export const metadata = { title: "Brands" };

export default async function BrandsPage() {
  const s = await requireStudio();
  const t = await getTranslations("brands");
  const brands = await studioTx(s, (c) => listBrands(c));
  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-bold">{t("title")}</h1>
      <ul className="grid gap-3 sm:grid-cols-2" data-testid="brand-list">
        {brands.map((b) => (
          <li key={b.id}>
            <Card className="space-y-1">
              <Link href={`/studio/brands/${b.id}`} className="font-semibold hover:underline">
                {b.name}
              </Link>
              <p className="text-sm text-muted-foreground" data-testid="brand-address">
                /s/{b.slug}
              </p>
              <p className="text-xs text-muted-foreground">
                {b.languages.map((l) => t(`lang.${l}`)).join(", ")} · {b.currency} · {t(`templates.${b.template}.name`)}
              </p>
            </Card>
          </li>
        ))}
      </ul>
      <Card>
        <h2 className="font-semibold">{t("add")}</h2>
        <ActionForm action={saveBrandAction.bind(null, null)} submit={t("create")} testId="new-brand-form">
          <BrandFields withTemplate />
        </ActionForm>
      </Card>
    </section>
  );
}
