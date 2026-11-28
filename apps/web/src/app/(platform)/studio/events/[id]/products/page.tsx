import Link from "next/link";
import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { loadEventGraph, stepsOf } from "@/lib/catalogue/domain";
import { priceSides } from "@/lib/catalogue/display";
import { availableQuantity } from "@/lib/catalogue/pools";
import { PRODUCT_TYPES } from "@/lib/catalogue/schema";
import { activePriceStep, formatMoney } from "@/lib/pricing";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { lt } from "@/lib/studio/text";
import { EventNav } from "@/components/studio/event-nav";
import { Button } from "@/components/ui/button";
import { Card, Label, Select } from "@/components/ui/field";

export const metadata = { title: "Products" };

export default async function ProductsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const s = await requireStudio();
  const t = await getTranslations("products");
  const locale = await getLocale();
  const g = await studioTx(s, (c) => loadEventGraph(c, id));
  if (!g) notFound();
  const cur = g.brand.currency;
  const now = new Date();
  return (
    <section className="space-y-6">
      <EventNav g={g} current="products" />
      <Card>
        <form action={`/studio/events/${id}/products/new`} method="get" className="flex flex-wrap items-end gap-2" data-testid="add-product">
          <Label className="min-w-48">
            {t("addType")}
            <Select name="type" defaultValue="pass">
              {PRODUCT_TYPES.map((p) => (
                <option key={p} value={p}>
                  {t(`types.${p}.name`)}
                </option>
              ))}
            </Select>
          </Label>
          <Button type="submit">{t("add")}</Button>
        </form>
      </Card>
      {g.products.length === 0 && <p className="text-muted-foreground">{t("none")}</p>}
      <ul className="space-y-3" data-testid="product-list">
        {g.products.map((p) => {
          const steps = stepsOf(g.steps, p.id);
          const face = steps.length ? activePriceStep({ steps, at: now }).priceMinor : null;
          const sides = face === null ? null : priceSides(g, face);
          const pools = g.links.filter((l) => l.product_id === p.id).map((l) => g.pools.find((x) => x.id === l.pool_id)?.name);
          return (
            <li key={p.id} data-testid="product" data-type={p.type}>
              <Card className="space-y-1">
                <div className="flex flex-wrap items-baseline justify-between gap-2">
                  <Link href={`/studio/events/${id}/products/${p.id}`} className="font-semibold break-words hover:underline">
                    {lt(p.name, locale) || t(`types.${p.type}.name`)}
                  </Link>
                  <span className="text-xs text-muted-foreground">{t(`types.${p.type}.name`)}</span>
                </div>
                <p className="text-xs text-muted-foreground">{t(`types.${p.type}.how`)}</p>
                {sides && (
                  <p className="text-sm">
                    <span data-testid="fan-pays">{t("fanPays", { amount: formatMoney(sides.allInMinor, cur, locale) })}</span> ·{" "}
                    <span data-testid="you-receive">{t("youReceive", { amount: formatMoney(sides.organiserNetMinor, cur, locale) })}</span>
                  </p>
                )}
                <p className="text-xs text-muted-foreground">
                  {pools.length ? t("drawsOn", { pools: pools.join(", ") }) : t("noPool")} · {t("available", { n: availableQuantity(p.id, g.pools, g.links) })}
                </p>
              </Card>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
