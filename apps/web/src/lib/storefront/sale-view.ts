// Time-dependent views of a published product (design D3.6, D3.7), worked out in the browser from
// the snapshot facts on the cached page and the core's live availability: the price step in force,
// the sales status, the next date step, instalments for a purchase now and the "from" price. Pure
// and client-safe (no server imports), so the server's first render and the tests use it too.
import { depositFor, instalmentSchedule } from "../pricing/instalments";
import type { ShopEvent, ShopProduct, ShopSaleProduct } from "./catalogue";

export type InstanceAvailability = { instanceId: string; status: "on_sale" | "sold_out"; remaining: number };

/** The core's answer for one product (only used when it is for the page's snapshot version). */
export type Availability = {
  status: "on_sale" | "not_yet" | "ended" | "sold_out";
  allInMinor: number;
  priceStepIndex: number;
  remaining?: number;
  opensAt?: string;
};

/** The step in force at `now` by date (a sales ladder stays on its first step until the core says otherwise). */
export function stepIndexAt(sale: ShopSaleProduct, now: Date): number {
  let idx = 0;
  for (let i = 1; i < sale.steps.length; i++) {
    const s = sale.steps[i];
    if (s.mode === "by_date" && (s.fromAt === null || Date.parse(s.fromAt) <= now.getTime())) idx = i;
  }
  return idx;
}

/** Price now: the core's step when it answered for this version, else the step by date. */
export function priceAt(p: ShopProduct, now: Date, a?: Availability | null): { stepIndex: number; allInMinor: number; fanFeeMinor: number } {
  const sale = p.sale;
  if (!sale || !sale.steps.length) return { stepIndex: 0, allInMinor: p.allInMinor, fanFeeMinor: p.fanFeeMinor };
  const idx = a && a.priceStepIndex >= 0 && a.priceStepIndex < sale.steps.length ? a.priceStepIndex : stepIndexAt(sale, now);
  return { stepIndex: idx, allInMinor: sale.steps[idx].allInMinor, fanFeeMinor: sale.steps[idx].fanFeeMinor };
}

/** The status by the sales window alone (when the core has not answered). */
export function statusByTime(sale: ShopSaleProduct | undefined, now: Date): Availability["status"] {
  if (sale?.salesUntil && Date.parse(sale.salesUntil) <= now.getTime()) return "ended";
  if (sale?.salesFrom && Date.parse(sale.salesFrom) > now.getTime()) return "not_yet";
  return "on_sale";
}

/** A date ladder's next step after `idx` (all-in, and when it starts); sales ladders never show one. */
export function nextDateStep(sale: ShopSaleProduct, idx: number): { allInMinor: number; fromAt: string } | null {
  const n = sale.steps[idx + 1];
  return n && n.mode === "by_date" && n.fromAt ? { allInMinor: n.allInMinor, fromAt: n.fromAt } : null;
}

/** Instalments for a purchase now, capped per buyer: deposit, then `count` payments (the last may differ). */
export function instalmentsAt(sale: ShopSaleProduct, allInMinor: number, now: Date, eventStartsAt: string, timeZone: string) {
  if (!sale.plan) return null;
  const s = instalmentSchedule({
    allInMinor,
    depositMinor: depositFor({ depositMinor: sale.plan.depositMinor, depositBps: sale.plan.depositBps }, allInMinor),
    monthlyCount: sale.plan.monthlyCount,
    dayOfMonth: sale.plan.dayOfMonth,
    from: now,
    eventStartsAt: new Date(eventStartsAt),
    timeZone,
    capToFit: true,
  });
  if (!s.ok || !s.payments.length) return null;
  return { depositMinor: s.depositMinor, count: s.payments.length, eachMinor: s.payments[0].amountMinor, lastMinor: s.payments.at(-1)!.amountMinor, totalMinor: s.totalMinor };
}

/**
 * The event's "from" price (D3.7): the lowest all-in amount among visible tickets on sale now; when
 * none is on sale yet, the lowest upcoming one (labelled, with the earliest opening). Add-ons,
 * which cannot be bought alone, do not count.
 */
export function fromPriceAt(ev: ShopEvent, now: Date, avail?: Record<string, Availability> | null): { allInMinor: number; upcoming: string | null } | null {
  const tickets = ev.products.filter((p) => (p.sale ? p.sale.isTicket : p.type !== "add_on"));
  const status = (p: ShopProduct) => avail?.[p.id]?.status ?? statusByTime(p.sale, now);
  const onSale = tickets.filter((p) => status(p) === "on_sale");
  if (onSale.length) return { allInMinor: Math.min(...onSale.map((p) => priceAt(p, now, avail?.[p.id]).allInMinor)), upcoming: null };
  const upcoming = tickets.filter((p) => status(p) === "not_yet");
  if (!upcoming.length) return null;
  const opens = upcoming.map((p) => avail?.[p.id]?.opensAt ?? p.sale?.salesFrom ?? "").filter(Boolean).sort()[0] ?? null;
  return { allInMinor: Math.min(...upcoming.map((p) => priceAt(p, now, avail?.[p.id]).allInMinor)), upcoming: opens };
}
