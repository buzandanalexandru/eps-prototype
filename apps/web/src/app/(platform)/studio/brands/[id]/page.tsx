import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { getBrand } from "@/lib/organisers/domain";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { ActionForm } from "@/components/studio/action-form";
import { BrandFields } from "@/components/studio/brand-fields";
import { Card } from "@/components/ui/field";
import { deleteBrandAction, saveBrandAction } from "../../actions";

export const metadata = { title: "Brand" };

export default async function BrandPage({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Promise<{ welcome?: string }> }) {
  const { id } = await params;
  const { welcome } = await searchParams;
  const s = await requireStudio();
  const t = await getTranslations("brands");
  const brand = await studioTx(s, (c) => getBrand(c, id));
  if (!brand) notFound();
  return (
    <section className="max-w-xl space-y-6">
      {welcome && (
        <p role="status" className="rounded-md border border-emerald-500/50 p-3 text-sm" data-testid="welcome">
          {t("welcome")}
        </p>
      )}
      <div>
        <h1 className="text-2xl font-bold" data-testid="brand-title">
          {brand.name}
        </h1>
        <p className="text-sm text-muted-foreground">
          {t("address")}: <span data-testid="brand-address">/s/{brand.slug}</span> · {t("template")}: <span data-testid="brand-template">{t(`templates.${brand.template}.name`)}</span>
        </p>
      </div>
      <Card>
        <h2 className="font-semibold">{t("settings")}</h2>
        <ActionForm action={saveBrandAction.bind(null, brand.id)} submit={t("save")} testId="brand-settings">
          <BrandFields brand={brand} />
        </ActionForm>
      </Card>
      {s.active.role === "owner" && (
        <Card>
          <h2 className="font-semibold">{t("delete")}</h2>
          <p className="text-sm text-muted-foreground">{t("deleteLead")}</p>
          <ActionForm action={deleteBrandAction.bind(null, brand.id)} submit={t("deleteButton")} variant="destructive" />
        </Card>
      )}
    </section>
  );
}
