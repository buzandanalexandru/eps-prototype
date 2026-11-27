import { notFound } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { loadEventGraph, planReference, stepsOf } from "@/lib/catalogue/domain";
import { priceSides } from "@/lib/catalogue/display";
import { toLocalInput } from "@/lib/catalogue/time";
import { activePriceStep, activePriceStepIndex, formatMoney, instalmentSchedule, maxMonthlyPayments, nextPriceStep, toMajorString } from "@/lib/pricing";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { lt } from "@/lib/studio/text";
import { ActionForm, FieldError } from "@/components/studio/action-form";
import { EventNav } from "@/components/studio/event-nav";
import { ProductFields } from "@/components/studio/product-fields";
import { Card, Check, Hint, Input, Label } from "@/components/ui/field";
import { deleteProductAction, saveProductAction, setLadderAction, setPlanAction, setProductPoolsAction } from "../../../../actions";

export const metadata = { title: "Product" };

export default async function ProductPage({ params }: { params: Promise<{ id: string; pid: string }> }) {
  const { id, pid } = await params;
  const s = await requireStudio();
  const t = await getTranslations("products");
  const locale = await getLocale();
  const g = await studioTx(s, (c) => loadEventGraph(c, id));
  const p = g?.products.find((x) => x.id === pid);
  if (!g || !p) notFound();
  const tz = g.event.timezone;
  const cur = g.brand.currency;
  const steps = stepsOf(g.steps, p.id);
  const mode = steps[0]?.mode ?? "by_date";
  const now = new Date();
  const active = steps.length ? activePriceStepIndex({ steps, at: now }) : -1;
  const next = steps.length ? nextPriceStep({ steps, at: now }) : null;
  const mine = new Set(g.links.filter((l) => l.product_id === p.id).map((l) => l.pool_id));
  const plan = g.plans.find((x) => x.product_id === p.id);
  const from = planReference(p, now);
  const allIn = steps.length ? priceSides(g, activePriceStep({ steps, at: from }).priceMinor).allInMinor : null;
  const schedule =
    plan && allIn !== null
      ? instalmentSchedule({ allInMinor: allIn, depositMinor: plan.deposit_minor, monthlyCount: plan.monthly_count, dayOfMonth: plan.day_of_month, from, eventStartsAt: g.event.starts_at })
      : null;
  const rows = [...steps, ...Array.from({ length: 3 }, () => null)];
  const money = (m: number) => formatMoney(m, cur, locale);
  return (
    <section className="space-y-6">
      <EventNav g={g} current="products" />
      <h2 className="text-xl font-semibold break-words" data-testid="product-title">
        {lt(p.name, locale) || t(`types.${p.type}.name`)}
      </h2>

      <Card>
        <h3 className="font-semibold">{t("settings")}</h3>
        <ActionForm action={saveProductAction.bind(null, id, p.id, p.type)} submit={t("save")} testId="product-form">
          <ProductFields type={p.type} product={p} days={g.days} others={g.products} tz={tz} />
        </ActionForm>
      </Card>

      <Card id="pools">
        <h3 className="font-semibold">{t("pools")}</h3>
        <Hint>{t("poolsHint")}</Hint>
        {g.pools.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("noPoolsInEvent")}</p>
        ) : (
          <ActionForm action={setProductPoolsAction.bind(null, p.id)} submit={t("savePools")} testId="pools-form">
            <div className="grid gap-1 sm:grid-cols-2">
              {g.pools.map((pool) => (
                <Check key={pool.id} name="pools" value={pool.id} defaultChecked={mine.has(pool.id)} label={`${pool.name} (${pool.capacity.toLocaleString(locale)} · ${t(`scope.${pool.scope}`)})`} />
              ))}
            </div>
            <FieldError name="pools" />
          </ActionForm>
        )}
      </Card>

      <Card id="ladder">
        <h3 className="font-semibold">{t("ladder")}</h3>
        {steps.length > 0 && (
          <div className="space-y-1 text-sm" data-testid="ladder-preview">
            <p>
              {t("inForce")}: <strong data-testid="price-in-force">{money(steps[active].priceMinor)}</strong> ·{" "}
              <span data-testid="fan-pays">{t("fanPays", { amount: money(priceSides(g, steps[active].priceMinor).allInMinor) })}</span> ·{" "}
              <span data-testid="you-receive">{t("youReceive", { amount: money(priceSides(g, steps[active].priceMinor).organiserNetMinor) })}</span>
            </p>
            <p data-testid="next-step">
              {next
                ? next.mode === "by_date"
                  ? t("nextByDate", { price: money(next.priceMinor), date: toLocalInput(next.fromAt!, tz).replace("T", " ") })
                  : t("nextBySales", { price: money(next.priceMinor), n: next.afterSold! })
                : t("topOfLadder")}
            </p>
          </div>
        )}
        <p className="rounded-md bg-muted p-2 text-xs" data-testid="public-note">
          {t("publicNote")}
        </p>
        <ActionForm action={setLadderAction.bind(null, p.id, tz)} submit={t("saveLadder")} testId="ladder-form">
          <fieldset className="flex flex-wrap gap-4 text-sm">
            <legend className="sr-only">{t("mode")}</legend>
            <label className="flex items-center gap-2">
              <input type="radio" name="mode" value="by_date" defaultChecked={mode === "by_date"} /> {t("byDate")}
            </label>
            <label className="flex items-center gap-2">
              <input type="radio" name="mode" value="by_sales" defaultChecked={mode === "by_sales"} /> {t("bySales")}
            </label>
          </fieldset>
          <ol className="space-y-2">
            {rows.map((st, i) => (
              <li key={i} className="flex flex-wrap items-end gap-2" data-testid="ladder-step">
                <span className="w-6 text-sm text-muted-foreground">{i + 1}.</span>
                {i === 0 ? (
                  <span className="w-52 text-sm text-muted-foreground">{t("firstStep")}</span>
                ) : (
                  <Label className="w-52">
                    {t("threshold")}
                    <Input
                      name={`steps.${i}.threshold`}
                      placeholder={t("thresholdPlaceholder")}
                      defaultValue={st ? (st.mode === "by_date" ? (st.fromAt ? toLocalInput(st.fromAt, tz) : "") : String(st.afterSold ?? "")) : ""}
                    />
                  </Label>
                )}
                <Label className="w-32">
                  {t("price", { currency: cur })}
                  <Input name={`steps.${i}.price`} inputMode="decimal" defaultValue={st ? toMajorString(st.priceMinor) : ""} />
                </Label>
                {i === active && <span className="text-xs font-semibold text-primary">{t("youAreHere")}</span>}
                <FieldError name={`steps.${i}.price`} />
              </li>
            ))}
          </ol>
          <Hint>{t("ladderHint")}</Hint>
          <FieldError name="steps" />
        </ActionForm>
      </Card>

      <Card id="instalments">
        <h3 className="font-semibold">{t("instalments")}</h3>
        <Hint>{t("instalmentsHint", { max: maxMonthlyPayments(from, plan?.day_of_month ?? 1, g.event.starts_at) })}</Hint>
        {schedule?.ok && (
          <div className="text-sm" data-testid="schedule">
            <p>
              {t("scheduleTotal", { amount: money(schedule.totalMinor) })} · <span data-testid="deposit">{t("depositNow", { amount: money(schedule.depositMinor) })}</span>
            </p>
            <ol className="list-decimal pl-6">
              {schedule.payments.map((x) => (
                <li key={x.n} data-testid="payment">
                  {x.dueOn}: {money(x.amountMinor)}
                </li>
              ))}
            </ol>
          </div>
        )}
        {schedule && !schedule.ok && <p className="text-sm text-destructive">{t("scheduleInvalid")}</p>}
        <ActionForm action={setPlanAction.bind(null, p.id)} submit={t("savePlan")} testId="plan-form">
          <div className="grid gap-3 sm:grid-cols-3">
            <Label>
              {t("deposit", { currency: cur })}
              <Input name="deposit" inputMode="decimal" defaultValue={plan ? toMajorString(plan.deposit_minor) : "10.00"} />
              <FieldError name="depositMinor" />
            </Label>
            <Label>
              {t("monthlyCount")}
              <Input name="monthlyCount" inputMode="numeric" defaultValue={String(plan?.monthly_count ?? 3)} />
              <FieldError name="monthlyCount" />
            </Label>
            <Label>
              {t("dayOfMonth")}
              <Input name="dayOfMonth" inputMode="numeric" defaultValue={String(plan?.day_of_month ?? 1)} />
              <FieldError name="dayOfMonth" />
            </Label>
          </div>
          {plan && <Check name="remove" label={t("removePlan")} />}
        </ActionForm>
      </Card>

      <Card>
        <ActionForm action={deleteProductAction.bind(null, id, p.id)} submit={t("delete")} variant="destructive" />
      </Card>
    </section>
  );
}
