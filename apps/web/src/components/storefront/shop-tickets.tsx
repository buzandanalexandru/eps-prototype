"use client";
// The published shop's ticket grid (design D3.6 to D3.8): the cards render on the server with the
// snapshot's all-in prices, then this island asks the core for live availability (through our own
// origin, short-cached) and runs the ticket picker. Without an answer the page still shows every
// price, with buying paused. The page HTML never holds live stock, so it can be cached.
import { useEffect, useMemo, useState } from "react";
import type { ShopEvent, ShopProduct } from "@/lib/storefront/catalogue";
import { fill, type ShopLabels } from "@/lib/storefront/labels";
import {
  addOne,
  blockedReason,
  canContinue,
  checkoutHref,
  instanceFor,
  maxFor,
  missingRequirements,
  removeOne,
  statusOf,
  total,
  unitPrice,
  type Availability,
  type Line,
  type PickerCtx,
} from "@/lib/storefront/picker";
import type { Locale } from "@/lib/storefront/schema";
import { AllInPrice, Money, PriceNotes, shopDate } from "./all-in-price";

type Props = { event: ShopEvent; labels: ShopLabels; locale: Locale; base: string; showPerks: boolean; now: string };

const card = "rounded-[var(--eps-radius)] border border-[color-mix(in_srgb,var(--eps-on-surface)_15%,transparent)] bg-[color-mix(in_srgb,var(--eps-on-surface)_4%,var(--eps-surface))]";
const btn = "inline-flex min-h-11 items-center justify-center rounded-[var(--eps-radius)] px-5 py-2 text-sm font-semibold";
const stepBtn = "inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-[var(--eps-radius)] border-2 border-[var(--eps-primary)] text-lg font-bold text-[var(--eps-primary)] disabled:cursor-not-allowed disabled:opacity-40";

const tx = (v: { ro: string; en: string }, l: Locale) => (v[l] || v[l === "ro" ? "en" : "ro"] || "").trim();

type Load = { state: "loading" } | { state: "ok"; data: Record<string, Availability> } | { state: "failed" };

function useAvailability(eventId: string): Load {
  const [load, setLoad] = useState<Load>({ state: "loading" });
  useEffect(() => {
    let stop = false;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const run = async () => {
      let next: Load = { state: "failed" };
      try {
        const res = await fetch(`/api/public/events/${eventId}/availability`, { headers: { accept: "application/json" } });
        if (res.ok) {
          const body = (await res.json()) as { products?: (Availability & { productId: string })[] };
          if (Array.isArray(body.products)) next = { state: "ok", data: Object.fromEntries(body.products.map((p) => [p.productId, p])) };
        }
      } catch {
        // network failure: buying stays paused, the page keeps its prices
      }
      if (stop) return;
      setLoad(next);
      // Refresh while the page is open; retry sooner after a failure.
      timer = setTimeout(run, next.state === "ok" ? 30_000 : 10_000);
    };
    void run();
    return () => {
      stop = true;
      if (timer) clearTimeout(timer);
    };
  }, [eventId]);
  return load;
}

export function ShopTickets({ event, labels, locale, base, showPerks, now }: Props) {
  const load = useAvailability(event.id);
  const [lines, setLines] = useState<Line[]>([]);
  const [choice, setChoice] = useState<Record<string, { day: string; time: string }>>({});
  const [limitHit, setLimitHit] = useState<string | null>(null);
  const ctx: PickerCtx = useMemo(() => ({ event, availability: load.state === "ok" ? load.data : null, now: new Date(now) }), [event, load, now]);
  const sum = total(lines, ctx);
  const missing = missingRequirements(lines, event);
  const paused = load.state !== "ok";

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
          const a = ctx.availability?.[p.id];
          const inst = instanceOf(p);
          const qty = lines.filter((l) => l.productId === p.id && l.instance === inst).reduce((n, l) => n + l.qty, 0);
          const allQty = lines.filter((l) => l.productId === p.id).reduce((n, l) => n + l.qty, 0);
          const blocked = blockedReason(lines, p, inst, ctx);
          const opens = a?.opensAt ?? sale.salesFrom;
          const name = tx(p.name, locale);
          const priceNow = unitPrice(p, ctx);
          const step = a ? sale.steps[a.priceStepIndex] : undefined;
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
                <AllInPrice allInMinor={priceNow} fanFeeMinor={step?.fanFeeMinor ?? p.fanFeeMinor} currency={event.currency} locale={locale} labels={labels} showFee />
                <PriceNotes sale={sale} currency={event.currency} locale={locale} labels={labels} timeZone={event.timezone} />
                <p className="text-sm font-semibold" data-testid="ticket-status">
                  {status === "not_yet" && opens
                    ? fill(labels.onSaleFrom, { date: shopDate(opens, locale, event.timezone) })
                    : status === "ended"
                      ? labels.salesEnded
                      : status === "sold_out"
                        ? labels.soldOut
                        : a?.remaining !== undefined
                          ? fill(labels.fewLeft, { n: a.remaining })
                          : ""}
                </p>
              </div>
              {sale.choice !== "none" && sale.days.length > 1 && (
                <label className="flex flex-col gap-1 text-sm">
                  <span>{labels.chooseDay}</span>
                  <select
                    className="min-h-11 rounded-[var(--eps-radius)] border border-[color-mix(in_srgb,var(--eps-on-surface)_30%,transparent)] bg-[var(--eps-surface)] px-2"
                    value={choice[p.id]?.day ?? ""}
                    onChange={(e) => setChoice({ ...choice, [p.id]: { day: e.target.value, time: choice[p.id]?.time ?? "" } })}
                    data-testid="choose-day"
                  >
                    <option value="">{labels.choose}</option>
                    {sale.days.map((d) => (
                      <option key={d} value={d}>
                        {shopDate(`${d}T12:00:00Z`, locale, "UTC")}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              {sale.choice === "slot" && (
                <label className="flex flex-col gap-1 text-sm">
                  <span>{labels.chooseTime}</span>
                  <select
                    className="min-h-11 rounded-[var(--eps-radius)] border border-[color-mix(in_srgb,var(--eps-on-surface)_30%,transparent)] bg-[var(--eps-surface)] px-2"
                    value={choice[p.id]?.time ?? ""}
                    onChange={(e) => setChoice({ ...choice, [p.id]: { day: choice[p.id]?.day ?? "", time: e.target.value } })}
                    data-testid="choose-time"
                  >
                    <option value="">{labels.choose}</option>
                    {sale.slots.map((t) => (
                      <option key={t} value={t}>
                        {t}
                      </option>
                    ))}
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
              {limitHit === p.id && allQty >= maxFor(p, ctx) && (
                <p role="status" className="text-sm font-semibold" data-testid="limit">
                  {fill(labels.limitReached, { n: maxFor(p, ctx) })}
                </p>
              )}
              {limitHit === p.id && allQty < maxFor(p, ctx) && event.sale?.maxPerAccount && (
                <p role="status" className="text-sm font-semibold" data-testid="limit">
                  {fill(labels.limitReached, { n: event.sale.maxPerAccount })}
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
          {paused && load.state === "loading" && <p className="text-xs text-[var(--eps-muted)]">{labels.checking}</p>}
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
