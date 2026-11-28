// The published shop's view of an event, built from its live sale snapshot (design D3.2, D3.6,
// D3.7): the same ShopEvent the shared renderer shows in the builder, with every amount taken from
// the snapshot's precomputed all-in prices. Only facts go into the (cached) page: what depends on
// the time is worked out in the browser (sale-view.ts). Hidden products, their ids included, and
// the organiser's side of each price never reach the page.
import { slotStarts } from "../snapshot/slots";
import type { PublicSnapshot } from "./published";
type SaleSnapshot = PublicSnapshot;
type SnapshotProduct = PublicSnapshot["products"][number];
import type { ShopEvent, ShopProduct, ShopSaleProduct } from "./catalogue";
import { stepIndexAt } from "./sale-view";

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

export function saleProduct(snap: SaleSnapshot, p: SnapshotProduct, visible: Set<string>): ShopSaleProduct {
  return {
    steps: p.priceSteps.map((s) => ({ mode: s.mode, fromAt: s.fromAt, afterSold: null, allInMinor: s.allInMinor, fanFeeMinor: s.fanFeeMinor })),
    salesFrom: p.salesFrom,
    salesUntil: p.salesUntil,
    maxPerOrder: p.maxPerOrder,
    // Only visible products: a hidden product's id is never published (security review).
    requires: p.requires.filter((id) => visible.has(id)),
    isTicket: p.type !== "add_on",
    poolIds: p.poolIds,
    ...choiceOf(snap, p),
    plan: p.instalments
      ? { depositMinor: p.instalments.depositMinor, depositBps: p.instalments.depositBps, monthlyCount: p.instalments.monthlyCount, dayOfMonth: p.instalments.dayOfMonth }
      : null,
  };
}

export function shopEventFromSnapshot(snap: SaleSnapshot, map: string | null, renderedAt: Date): ShopEvent {
  const shown = snap.products.filter((p) => p.visible && p.priceSteps.length > 0);
  const visible = new Set(shown.map((p) => p.productId));
  const products: ShopProduct[] = shown
    .sort((a, b) => a.sortOrder - b.sortOrder || a.productId.localeCompare(b.productId))
    .map((p) => {
      const sale = saleProduct(snap, p, visible);
      const step = p.priceSteps[stepIndexAt(sale, renderedAt)];
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
    sale: {
      snapshotId: snap.snapshotId,
      version: snap.version,
      maxPerAccount: snap.rules.maxPerAccount,
      pools: snap.pools.map((p) => ({ poolId: p.poolId, scope: p.scope, capacity: p.instances[0]?.capacity ?? 0 })),
      renderedAt: renderedAt.toISOString(),
    },
  };
}
