import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { loadEventGraph } from "@/lib/catalogue/domain";
import { instanceCount, poolBars } from "@/lib/catalogue/pools";
import { POOL_SCOPES, slotSpan } from "@/lib/catalogue/schema";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { lt } from "@/lib/studio/text";
import { ActionForm, FieldError } from "@/components/studio/action-form";
import { EventNav } from "@/components/studio/event-nav";
import { Card, Hint, Input, Label, Select } from "@/components/ui/field";
import { deletePoolAction, savePoolAction } from "../../../actions";

export const metadata = { title: "Pools" };

export default async function PoolsPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const s = await requireStudio();
  const t = await getTranslations("pools");
  const tp = await getTranslations("products");
  const locale = await getLocale();
  const g = await studioTx(s, (c) => loadEventGraph(c, id));
  if (!g) notFound();
  const bars = poolBars(g.pools, g.links, g.products);
  const slot = g.products.find((p) => p.type === "time_slot")?.config as { slotMinutes?: number; firstStart?: string; lastStart?: string } | undefined;
  const slotsPerDay = slot?.slotMinutes && slot.firstStart && slot.lastStart ? slotSpan(slot.firstStart, slot.lastStart) / slot.slotMinutes + 1 : 0;
  return (
    <section className="space-y-6">
      <EventNav g={g} current="pools" />
      <p className="text-sm text-muted-foreground">{t("lead")}</p>
      <ul className="space-y-3" data-testid="pool-list">
        {bars.map(({ pool, products }) => (
          <li key={pool.id} id={`pool-${pool.id}`} data-testid="pool">
            <Card>
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <span className="font-semibold" data-testid="pool-name">
                  {pool.name}
                </span>
                <span className="text-sm" data-testid="pool-capacity">
                  {t("capacityOf", { n: pool.capacity.toLocaleString(locale) })} · {tp(`scope.${pool.scope}`)}
                </span>
              </div>
              {/* The pool bar: nothing sold yet in Studio, so remaining = capacity (sales come from the core). */}
              <div className="h-2 w-full overflow-hidden rounded bg-muted" role="img" aria-label={t("barLabel", { n: pool.capacity.toLocaleString(locale) })}>
                <div className="h-full bg-primary" style={{ width: pool.capacity > 0 ? "100%" : "0%" }} />
              </div>
              <p className="text-xs text-muted-foreground">
                {pool.scope !== "event" && t("instances", { n: instanceCount(pool, g.days.length, slotsPerDay) }) + " · "}
                {products.length ? t("drawnBy", { products: products.map((p) => lt(p.name, locale)).join(", ") }) : t("noProducts")}
              </p>
              <details>
                <summary className="cursor-pointer text-sm">{t("edit")}</summary>
                <div className="flex flex-wrap items-end gap-3 pt-2">
                  <ActionForm action={savePoolAction.bind(null, id, pool.id)} submit={t("save")}>
                    <PoolFields name={pool.name} capacity={pool.capacity} scope={pool.scope} />
                  </ActionForm>
                  <ActionForm action={deletePoolAction.bind(null, id, pool.id)} submit={t("delete")} variant="destructive" />
                </div>
              </details>
            </Card>
          </li>
        ))}
      </ul>
      <Card>
        <h2 className="font-semibold">{t("add")}</h2>
        <ActionForm action={savePoolAction.bind(null, id, null)} submit={t("create")} testId="new-pool-form">
          <PoolFields />
        </ActionForm>
        <Hint>{t("linkHint")}</Hint>
      </Card>
    </section>
  );
}

async function PoolFields({ name, capacity, scope }: { name?: string; capacity?: number; scope?: string }) {
  const t = await getTranslations("pools");
  const tp = await getTranslations("products");
  return (
    <div className="grid gap-3 sm:grid-cols-3">
      <Label>
        {t("name")}
        <Input name="name" defaultValue={name} />
        <FieldError name="name" />
      </Label>
      <Label>
        {t("capacity")}
        <Input name="capacity" inputMode="numeric" defaultValue={capacity ?? ""} />
        <FieldError name="capacity" />
      </Label>
      <Label>
        {t("scope")}
        <Select name="scope" defaultValue={scope ?? "event"}>
          {POOL_SCOPES.map((x) => (
            <option key={x} value={x}>
              {tp(`scope.${x}`)}
            </option>
          ))}
        </Select>
      </Label>
    </div>
  );
}
