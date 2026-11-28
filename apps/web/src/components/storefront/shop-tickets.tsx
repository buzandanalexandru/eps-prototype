"use client";
// The published shop's ticket grid (design D3.6 to D3.8). The cached page carries the snapshot's
// facts; this island works out the present (price step, sales status, instalments), asks the core
// for live availability through our own origin and runs the ticket picker. Without an answer the
// page still shows every price, with buying paused. The page HTML never holds live stock.
import { useMemo, useState } from "react";
import type { ShopEvent, ShopProduct } from "@/lib/storefront/catalogue";
import { fill, type ShopLabels } from "@/lib/storefront/labels";
import {
  choiceAllHeld,
  addOne,
  blockedReason,
  canContinue,
  checkoutHref,
  choiceSoldOut,
  instanceFor,
  maxFor,
  missingRequirements,
  removeOne,
  statusOf,
  total,
  type Line,
  type PickerCtx,
} from "@/lib/storefront/picker";
import { availabilityFor } from "@/lib/storefront/availability-answer";
import { fromPriceAt, instalmentsAt, priceAt } from "@/lib/storefront/sale-view";
import type { Locale } from "@/lib/storefront/schema";
import { AllInPrice, Money, PriceNotes, money, shopDate } from "./all-in-price";
import { useAvailability, useNow } from "./availability";

type Props = { event: ShopEvent; labels: ShopLabels; locale: Locale; base: string; showPerks: boolean };

const card = "rounded-[var(--eps-radius)] border border-[color-mix(in_srgb,var(--eps-on-surface)_15%,transparent)] bg-[color-mix(in_srgb,var(--eps-on-surface)_4%,var(--eps-surface))]";
const btn = "inline-flex min-h-11 items-center justify-center rounded-[var(--eps-radius)] px-5 py-2 text-sm font-semibold";
const stepBtn = "inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-[var(--eps-radius)] border-2 border-[var(--eps-primary)] text-lg font-bold text-[var(--eps-primary)] disabled:cursor-not-allowed disabled:opacity-40";
const select = "min-h-11 rounded-[var(--eps-radius)] border border-[color-mix(in_srgb,var(--eps-on-surface)_30%,transparent)] bg-[var(--eps-surface)] px-2";

const tx = (v: { ro: string; en: string }, l: Locale) => (v[l] || v[l === "ro" ? "en" : "ro"] || "").trim();

/** The event's "from" price, worked out in the browser (it depends on the time and on availability). */
export function FromPrice({ event, labels, locale }: { event: ShopEvent; labels: ShopLabels; locale: Locale }) {
  const now = useNow(event.sale?.renderedAt ?? event.startsAt);
  const load = useAvailability(event.id, event.sale?.version ?? 0);
  const from = fromPriceAt(event, now, availabilityFor(load, event, now)?.products);
  if (!from) return null;
  return (
    <span data-testid="from-price">
      {from.upcoming
        ? fill(labels.upcomingFrom, { price: money(from.allInMinor, event.currency, locale), date: shopDate(from.upcoming, locale, event.timezone) })
        : fill(labels.fromPrice, { price: money(from.allInMinor, event.currency, locale) })}
    </span>
  );
}

export function ShopTickets({ event, labels, locale, base, showPerks }: Props) {
  const now = useNow(event.sale?.renderedAt ?? event.startsAt);
  const load = useAvailability(event.id, event.sale?.version ?? 0);
  const [lines, setLines] = useState<Line[]>([]);
  const [choice, setChoice] = useState<Record<string, { day: string; time: string }>>({});
  const [limitHit, setLimitHit] = useState<string | null>(null);
  const ctx: PickerCtx = useMemo(() => ({ event, availability: availabilityFor(load, event, now), now }), [event, load, now]);
  const sum = total(lines, ctx);
  const missing = missingRequirements(lines, event);
  const paused = !ctx.availability;

  const instanceOf = (p: ShopProduct) => {
    const c = choice[p.id];
    return instanceFor(p, c?.day ?? (p.sale?.days.length === 1 ? p.sale.days[0] : null), c?.time ?? null);
  };

  return (
    <div className="space-y-4" data-testid="shop-tickets" data-availability={load.state}>
      {load.state === "failed" && (
        <p role="status" className={`${card} p-3 text-sm font-semibold`} data-testid="availability-failed">
          {labels.unavailable}
        </p>
      )}
      <ul className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3" data-testid="ticket-grid">
        {event.products.map((p) => {
          const sale = p.sale!;
          const status = statusOf(p, ctx);
          const a = ctx.availability?.products[p.id];
          const price = priceAt(p, now, a);
          const inst = instanceOf(p);
          const qty = lines.filter((l) => l.productId === p.id && l.instance === inst).reduce((n, l) => n + l.qty, 0);
          const allQty = lines.filter((l) => l.productId === p.id).reduce((n, l) => n + l.qty, 0);
          const blocked = blockedReason(lines, p, inst, ctx);
          const opens = a?.opensAt ?? sale.salesFrom;
          const name = tx(p.name, locale);
          const chosenDay = choice[p.id]?.day ?? (sale.days.length === 1 ? sale.days[0] : "");
          return (
            <li key={p.id} className={`${card} flex min-w-0 flex-col gap-3 p-5`} data-testid="ticket-card" data-product={p.id} data-status={status}>
              <h3 className="font-[family-name:var(--eps-font-heading)] text-xl font-semibold break-words">{name}</h3>
              {showPerks && p.perks.length > 0 && (
                <ul className="ml-5 list-disc space-y-1 text-sm">
                  {p.perks.map((k, i) => (
                    <li key={i}>{tx(k, locale)}</li>
                  ))}
                </ul>
              )}
              <div className="mt-auto space-y-2">
                <AllInPrice allInMinor={price.allInMinor} fanFeeMinor={price.fanFeeMinor} currency={event.currency} locale={locale} labels={labels} showFee />
                <PriceNotes
                  sale={sale}
                  stepIndex={price.stepIndex}
                  instalments={instalmentsAt(sale, price.allInMinor, now, event.startsAt, event.timezone)}
                  currency={event.currency}
                  locale={locale}
                  labels={labels}
                  timeZone={event.timezone}
                />
                <p className="text-sm font-semibold" data-testid="ticket-status">
                  {status === "not_yet" && opens
                    ? fill(labels.onSaleFrom, { date: shopDate(opens, locale, event.timezone) })
                    : status === "ended"
                      ? labels.salesEnded
                      : status === "sold_out"
                        ? a?.allHeld
                          ? labels.allHeld
                          : labels.soldOut
                        : a?.remaining !== undefined
                          ? fill(labels.fewLeft, { n: a.remaining })
                          : ""}
                </p>
              </div>
              {sale.choice !== "none" && sale.days.length > 1 && (
                <label className="flex flex-col gap-1 text-sm">
                  <span>{labels.chooseDay}</span>
                  <select
                    className={select}
                    value={choice[p.id]?.day ?? ""}
                    onChange={(e) => setChoice({ ...choice, [p.id]: { day: e.target.value, time: choice[p.id]?.time ?? "" } })}
                    data-testid="choose-day"
                  >
                    <option value="">{labels.choose}</option>
                    {sale.days.map((d) => {
                      const full = sale.choice === "day" && choiceSoldOut(p, d, ctx);
                      return (
                        <option key={d} value={d} disabled={full} data-sold-out={full || undefined}>
                          {shopDate(`${d}T12:00:00Z`, locale, "UTC")}
                          {full ? ` · ${choiceAllHeld(p, d, ctx) ? labels.allHeldChoice : labels.soldOut}` : ""}
                        </option>
                      );
                    })}
                  </select>
                </label>
              )}
              {sale.choice === "slot" && (
                <label className="flex flex-col gap-1 text-sm">
                  <span>{labels.chooseTime}</span>
                  <select
                    className={select}
                    value={choice[p.id]?.time ?? ""}
                    onChange={(e) => setChoice({ ...choice, [p.id]: { day: choice[p.id]?.day ?? "", time: e.target.value } })}
                    data-testid="choose-time"
                  >
                    <option value="">{labels.choose}</option>
                    {sale.slots.map((t) => {
                      const q = chosenDay ? instanceFor(p, chosenDay, t) : null;
                      const full = !!q && choiceSoldOut(p, q, ctx);
                      return (
                        <option key={t} value={t} disabled={full} data-sold-out={full || undefined}>
                          {t}
                          {full ? ` · ${q && choiceAllHeld(p, q, ctx) ? labels.allHeldChoice : labels.soldOut}` : ""}
                        </option>
                      );
                    })}
                  </select>
                </label>
              )}
              <div className="flex items-center gap-3">
                <button type="button" className={stepBtn} aria-label={fill(labels.removeOne, { product: name })} disabled={qty === 0} onClick={() => setLines(removeOne(lines, p.id, inst))} data-testid="minus">
                  −
                </button>
                <output className="min-w-8 text-center text-lg font-semibold" aria-label={labels.quantity} data-testid="qty">
                  {qty}
                </output>
                <button
                  type="button"
                  className={stepBtn}
                  aria-label={fill(labels.addOne, { product: name })}
                  disabled={paused || (blocked !== null && blocked !== "limit" && blocked !== "account_limit")}
                  onClick={() => {
                    if (blocked === "limit" || blocked === "account_limit") setLimitHit(p.id);
                    else {
                      setLimitHit(null);
                      setLines(addOne(lines, p, inst, ctx));
                    }
                  }}
                  data-testid="plus"
                >
                  +
                </button>
              </div>
              {limitHit === p.id && (
                <p role="status" className="text-sm font-semibold" data-testid="limit">
                  {fill(labels.limitReached, { n: blocked === "account_limit" && event.sale?.maxPerAccount ? event.sale.maxPerAccount : Math.min(maxFor(p, ctx), Math.max(allQty, 0) || maxFor(p, ctx)) })}
                </p>
              )}
            </li>
          );
        })}
      </ul>
      <div className={`${card} sticky bottom-2 flex flex-wrap items-center justify-between gap-3 p-4`} data-testid="picker-summary">
        <div className="min-w-0">
          <p className="text-sm text-[var(--eps-muted)]">{labels.total}</p>
          <p className="text-xl font-bold">
            <Money minor={sum} currency={event.currency} locale={locale} testId="picker-total" />
          </p>
          {missing.map((m) => (
            <p key={m.id} className="text-sm font-semibold" data-testid="needs-ticket">
              {fill(labels.needsTicket, { product: tx(m.name, locale), event: tx(event.name, locale) })}
            </p>
          ))}
          {load.state === "loading" && <p className="text-xs text-[var(--eps-muted)]">{labels.checking}</p>}
        </div>
        {canContinue(lines, ctx) ? (
          <a href={checkoutHref(base, event, lines)} className={`${btn} bg-[var(--eps-primary)] text-[var(--eps-on-primary)]`} data-testid="continue">
            {labels.continue}
          </a>
        ) : (
          <button type="button" disabled className={`${btn} cursor-not-allowed bg-[var(--eps-primary)] text-[var(--eps-on-primary)] opacity-50`} data-testid="continue">
            {labels.continue}
          </button>
        )}
      </div>
    </div>
  );
}
