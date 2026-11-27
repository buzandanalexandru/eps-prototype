import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { loadEventGraph } from "@/lib/catalogue/domain";
import { PRODUCT_TYPES, type ProductType } from "@/lib/catalogue/schema";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { ActionForm, FieldError } from "@/components/studio/action-form";
import { EventNav } from "@/components/studio/event-nav";
import { ProductFields } from "@/components/studio/product-fields";
import { Card, Hint, Input, Label, Select } from "@/components/ui/field";
import { saveProductAction } from "../../../../actions";

export const metadata = { title: "New product" };

export default async function NewProductPage({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Promise<{ type?: string }> }) {
  const { id } = await params;
  const { type = "pass" } = await searchParams;
  if (!(PRODUCT_TYPES as readonly string[]).includes(type)) notFound();
  const s = await requireStudio();
  const t = await getTranslations("products");
  const g = await studioTx(s, (c) => loadEventGraph(c, id));
  if (!g) notFound();
  return (
    <section className="space-y-6">
      <EventNav g={g} current="products" />
      <Card>
        <h2 className="font-semibold">{t("newTitle", { type: t(`types.${type}.name`) })}</h2>
        <ActionForm action={saveProductAction.bind(null, id, null, type)} submit={t("create")} testId="product-form">
          <ProductFields type={type as ProductType} days={g.days} others={g.products} tz={g.event.timezone} />
          <div className="grid gap-3 sm:grid-cols-2">
            <Label>
              {t("price", { currency: g.brand.currency })}
              <Input name="price" inputMode="decimal" required />
              <Hint>{t("priceHint")}</Hint>
              <FieldError name="price" />
            </Label>
            <Label>
              {t("pool")}
              <Select name="poolId" defaultValue="">
                <option value="">{t("noPoolYet")}</option>
                {g.pools.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </Select>
            </Label>
          </div>
        </ActionForm>
      </Card>
    </section>
  );
}
