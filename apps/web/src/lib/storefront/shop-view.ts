// The published shop's view of an event, built from its live sale snapshot (design D3.2, D3.6,
// D3.7): the same ShopEvent the shared renderer shows in the builder, with every amount taken from
// the snapshot's precomputed all-in prices. The price step in force is decided by the date here;
// a sales ladder's step comes from the core's availability answer on the page.
import { slotStarts } from "../snapshot/compile";
import { depositFor, instalmentSchedule } from "../pricing/instalments";
import type { SaleSnapshot, SnapshotProduct } from "../snapshot/types";
import type { ShopEvent, ShopProduct, ShopSaleProduct } from "./catalogue";

/** The step in force at `at` (by date; a sales ladder starts at its first step). Same rule as lib/pricing. */
export function stepAt(p: SnapshotProduct, at: Date, sold = 0): number {
  let idx = 0;
  for (let i = 1; i < p.priceSteps.length; i++) {
    const s = p.priceSteps[i];
    const reached = s.mode === "by_date" ? s.fromAt === null || Date.parse(s.fromAt) <= at.getTime() : s.afterSold === null || sold >= s.afterSold;
    if (reached) idx = i;
  }
  return idx;
}

function choiceOf(snap: SaleSnapshot, p: SnapshotProduct): Pick<ShopSaleProduct, "choice" | "days" | "slots" | "firstStart"> {
  const pools = snap.pools.filter((x) => p.poolIds.includes(x.poolId));
  const eventDays = snap.event.days.map((d) => d.date);
  if (pools.some((x) => x.scope === "slot")) {
    const cfg = p.config as { slotMinutes?: number; firstStart?: string; lastStart?: string };
    const slots = cfg.slotMinutes && cfg.firstStart && cfg.lastStart ? slotStarts({ slotMinutes: cfg.slotMinutes, firstStart: cfg.firstStart, lastStart: cfg.lastStart }) : [];
    return { choice: "slot", days: eventDays, slots, firstStart: cfg.firstStart ?? null };
  }
  if (pools.some((x) => x.scope === "day")) {
    const fixed = p.type === "day_ticket" ? (p.config as { day?: string | null }).day : null;
    return { choice: "day", days: fixed ? [fixed] : eventDays, slots: [], firstStart: null };
  }
  return { choice: "none", days: [], slots: [], firstStart: null };
}

export function saleProduct(snap: SaleSnapshot, p: SnapshotProduct, now: Date): ShopSaleProduct {
  const idx = stepAt(p, now);
  const step = p.priceSteps[idx];
  const nxt = p.priceSteps[idx + 1];
  let instalments: ShopSaleProduct["instalments"] = null;
  if (p.instalments) {
    const deposit = depositFor({ depositMinor: p.instalments.depositMinor, depositBps: p.instalments.depositBps }, step.allInMinor);
    const s = instalmentSchedule({
      allInMinor: step.allInMinor,
      depositMinor: deposit,
      monthlyCount: p.instalments.monthlyCount,
      dayOfMonth: p.instalments.dayOfMonth,
      from: now,
      eventStartsAt: new Date(snap.event.startsAt),
      timeZone: snap.timezone,
      capToFit: true,
    });
    if (s.ok && s.payments.length) {
      instalments = { depositMinor: s.depositMinor, count: s.payments.length, eachMinor: s.payments[0].amountMinor, lastMinor: s.payments.at(-1)!.amountMinor, totalMinor: s.totalMinor };
    }
  }
  return {
    stepIndex: idx,
    steps: p.priceSteps.map((s) => ({ allInMinor: s.allInMinor, fanFeeMinor: s.fanFeeMinor })),
    mode: step.mode,
    next: nxt && nxt.mode === "by_date" && nxt.fromAt ? { allInMinor: nxt.allInMinor, fromAt: nxt.fromAt } : null,
    salesFrom: p.salesFrom,
    salesUntil: p.salesUntil,
    maxPerOrder: p.maxPerOrder,
    requires: p.requires,
    isTicket: p.type !== "add_on",
    ...choiceOf(snap, p),
    instalments,
  };
}

export function shopEventFromSnapshot(snap: SaleSnapshot, map: string | null, now: Date): ShopEvent {
  const products: ShopProduct[] = snap.products
    .filter((p) => p.visible && p.priceSteps.length > 0)
    .sort((a, b) => a.sortOrder - b.sortOrder || a.productId.localeCompare(b.productId))
    .map((p) => {
      const sale = saleProduct(snap, p, now);
      const step = p.priceSteps[sale.stepIndex];
      return { id: p.productId, type: p.type, name: p.name, perks: p.perks, allInMinor: step.allInMinor, fanFeeMinor: step.fanFeeMinor, sale };
    });
  const v = snap.event.venue;
  return {
    id: snap.eventId,
    slug: snap.event.slug,
    name: snap.event.name,
    startsAt: snap.event.startsAt,
    endsAt: snap.event.endsAt,
    timezone: snap.timezone,
    currency: snap.currency,
    venue: v ? { name: v.name, address: v.address, lat: v.lat, lng: v.lng, map } : null,
    days: snap.event.days,
    lineup: snap.event.lineup,
    products,
    sale: { snapshotId: snap.snapshotId, version: snap.version, maxPerAccount: snap.rules.maxPerAccount },
  };
}

/**
 * The event's "from" price (D3.7): the lowest all-in amount among visible tickets on sale now; when
 * none is on sale yet, the lowest upcoming one (labelled). Add-ons, which cannot be bought alone, do
 * not count.
 */
export function fromPrice(ev: ShopEvent, now: Date): { allInMinor: number; upcoming: boolean } | null {
  const tickets = ev.products.filter((p) => (p.sale ? p.sale.isTicket : p.type !== "add_on"));
  const open = (p: ShopProduct) => !p.sale || ((!p.sale.salesFrom || Date.parse(p.sale.salesFrom) <= now.getTime()) && (!p.sale.salesUntil || Date.parse(p.sale.salesUntil) > now.getTime()));
  const onSale = tickets.filter(open);
  const upcoming = tickets.filter((p) => p.sale?.salesFrom && Date.parse(p.sale.salesFrom) > now.getTime());
  const pick = onSale.length ? onSale : upcoming;
  if (!pick.length) return null;
  return { allInMinor: Math.min(...pick.map((p) => p.allInMinor)), upcoming: !onSale.length };
}
