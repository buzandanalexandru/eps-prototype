// The ticket picker's rules (design D3.8), pure so the client component and the tests share them:
// quantities within each product's maximum per order, the event's maximum per account and what the
// core reports as left, per product and per pool instance (products sharing a pool share its
// room); add-ons only with a ticket they require; a day or a day and start time for products on
// day or slot pools (sold-out days and slots cannot be chosen); the running all-in total; and the
// hand-off to checkout.
import { slotStartDate } from "../catalogue/time";
import type { ShopEvent, ShopProduct } from "./catalogue";
import { priceAt, statusByTime, type Availability, type InstanceAvailability } from "./sale-view";

export type { Availability, InstanceAvailability };

/** A line: a product, the pool instance qualifier it is for (day or day and start time) and a quantity. */
export type Line = { productId: string; instance: string | null; qty: number };

/** What the core said, for the page's snapshot version (null: unknown or unreachable, buying paused). */
export type AvailabilityState = {
  products: Record<string, Availability>;
  /** Pool instances that are sold out or have few left (others have room). */
  instances: Record<string, InstanceAvailability>;
};

export type PickerCtx = { event: ShopEvent; availability: AvailabilityState | null; now: Date };

/** The instance qualifier of a choice: `YYYY-MM-DD` for a day, the local start date and time for a slot. */
export function instanceFor(p: ShopProduct, day: string | null, time: string | null): string | null {
  const s = p.sale;
  if (!s || s.choice === "none") return null;
  if (!day || !s.days.includes(day)) return null;
  if (s.choice === "day") return day;
  if (!time || !s.slots.includes(time)) return null;
  return `${slotStartDate(day, s.firstStart ?? time, time)}T${time}`;
}

/** The pool instances one unit of the product draws on for a choice (scoped pools need the choice). */
export function poolInstancesOf(p: ShopProduct, ev: ShopEvent, instance: string | null): string[] {
  const out: string[] = [];
  for (const id of p.sale?.poolIds ?? []) {
    const pool = ev.sale?.pools.find((x) => x.poolId === id);
    if (!pool) continue;
    if (pool.scope === "event") out.push(id);
    else if (instance) out.push(pool.scope === "day" ? `${id}@${instance.slice(0, 10)}` : `${id}@${instance}`);
  }
  return out;
}

/** Room left in a pool instance: the core's count when it reported one (few left), else its capacity. */
export function roomIn(instanceId: string, ctx: PickerCtx): number {
  const known = ctx.availability?.instances[instanceId];
  if (known) return known.remaining;
  const pool = ctx.event.sale?.pools.find((x) => x.poolId === instanceId.split("@")[0]);
  return pool?.capacity ?? 0;
}

/** Whether a day or slot can no longer be chosen (one of its pool instances is full). */
export function choiceSoldOut(p: ShopProduct, instance: string, ctx: PickerCtx): boolean {
  return poolInstancesOf(p, ctx.event, instance).some((i) => ctx.availability?.instances[i]?.status === "sold_out" || roomIn(i, ctx) <= 0);
}

export type Blocked = "paused" | "not_yet" | "ended" | "sold_out" | "choose" | "limit" | "account_limit" | null;

const qtyOf = (lines: Line[], productId: string) => lines.filter((l) => l.productId === productId).reduce((n, l) => n + l.qty, 0);
const ticketQty = (lines: Line[], ev: ShopEvent) => lines.filter((l) => ev.products.find((p) => p.id === l.productId)?.sale?.isTicket).reduce((n, l) => n + l.qty, 0);

/** The status of a product: from the core when known, else from its sales window. */
export function statusOf(p: ShopProduct, ctx: PickerCtx): Availability["status"] {
  return ctx.availability?.products[p.id]?.status ?? statusByTime(p.sale, ctx.now);
}

/** The most of a product one order may hold now: its maximum per order, capped by what is left. */
export function maxFor(p: ShopProduct, ctx: PickerCtx): number {
  const max = p.sale?.maxPerOrder ?? 0;
  const left = ctx.availability?.products[p.id]?.remaining;
  return typeof left === "number" ? Math.min(max, left) : max;
}

/** Units the selection already draws from a pool instance, across every product sharing it. */
function usedIn(lines: Line[], instanceId: string, ev: ShopEvent): number {
  return lines.reduce((n, l) => {
    const p = ev.products.find((x) => x.id === l.productId);
    return p && poolInstancesOf(p, ev, l.instance).includes(instanceId) ? n + l.qty : n;
  }, 0);
}

/** Why one more of the product cannot be added (null: it can). */
export function blockedReason(lines: Line[], p: ShopProduct, instance: string | null, ctx: PickerCtx): Blocked {
  if (!ctx.availability) return "paused";
  const st = statusOf(p, ctx);
  if (st !== "on_sale") return st;
  if (p.sale && p.sale.choice !== "none" && !instance) return "choose";
  if (instance && choiceSoldOut(p, instance, ctx)) return "sold_out";
  if (qtyOf(lines, p.id) >= maxFor(p, ctx)) return "limit";
  if (poolInstancesOf(p, ctx.event, instance).some((i) => usedIn(lines, i, ctx.event) >= roomIn(i, ctx))) return "limit";
  const perAccount = ctx.event.sale?.maxPerAccount;
  if (p.sale?.isTicket && perAccount && ticketQty(lines, ctx.event) >= perAccount) return "account_limit";
  return null;
}

/** Adds one (when allowed); returns the same lines when blocked. */
export function addOne(lines: Line[], p: ShopProduct, instance: string | null, ctx: PickerCtx): Line[] {
  if (blockedReason(lines, p, instance, ctx)) return lines;
  const i = lines.findIndex((l) => l.productId === p.id && l.instance === instance);
  if (i < 0) return [...lines, { productId: p.id, instance, qty: 1 }];
  return lines.map((l, j) => (j === i ? { ...l, qty: l.qty + 1 } : l));
}

export function removeOne(lines: Line[], productId: string, instance: string | null): Line[] {
  return lines.flatMap((l) => (l.productId === productId && l.instance === instance ? (l.qty > 1 ? [{ ...l, qty: l.qty - 1 }] : []) : [l]));
}

/** The unit all-in price now: the core's current step when known (sales ladders), else by date. */
export function unitPrice(p: ShopProduct, ctx: PickerCtx): number {
  return priceAt(p, ctx.now, ctx.availability?.products[p.id]).allInMinor;
}

export function total(lines: Line[], ctx: PickerCtx): number {
  return lines.reduce((sum, l) => {
    const p = ctx.event.products.find((x) => x.id === l.productId);
    return p ? sum + l.qty * unitPrice(p, ctx) : sum;
  }, 0);
}

/** Add-ons in the selection whose required tickets are missing. */
export function missingRequirements(lines: Line[], ev: ShopEvent): ShopProduct[] {
  const picked = new Set(lines.filter((l) => l.qty > 0).map((l) => l.productId));
  return ev.products.filter((p) => picked.has(p.id) && p.sale && p.sale.requires.length > 0 && !p.sale.requires.some((r) => picked.has(r)));
}

/** Whether "Continue" can be pressed: something selected, buying not paused, requirements met. */
export function canContinue(lines: Line[], ctx: PickerCtx): boolean {
  return !!ctx.availability && lines.some((l) => l.qty > 0) && missingRequirements(lines, ctx.event).length === 0;
}

/** The checkout route (owned by step 4) with the event, snapshot version and items. */
export function checkoutHref(base: string, ev: ShopEvent, lines: Line[]): string {
  const items = lines
    .filter((l) => l.qty > 0)
    .map((l) => [l.productId, String(l.qty), ...(l.instance ? [l.instance] : [])].join(":"))
    .join(",");
  const q = new URLSearchParams({ event: ev.id, v: String(ev.sale?.version ?? 0), items });
  return `${base}/checkout?${q.toString()}`;
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const INSTANCE = /^\d{4}-\d{2}-\d{2}(T([01]\d|2[0-3]):[0-5]\d)?$/;

/** Parses `items` from the checkout URL; null when malformed (at most 50 lines, 1 to 100 each). */
export function parseItems(raw: string | null | undefined): Line[] | null {
  if (!raw || raw.length > 4000) return null;
  const out: Line[] = [];
  for (const part of raw.split(",")) {
    // productId:qty[:instance]; a slot instance holds a colon of its own (2027-07-02T01:00).
    const a = part.indexOf(":");
    const b = a < 0 ? -1 : part.indexOf(":", a + 1);
    const productId = a < 0 ? part : part.slice(0, a);
    const q = a < 0 ? "" : b < 0 ? part.slice(a + 1) : part.slice(a + 1, b);
    const instance = b < 0 ? undefined : part.slice(b + 1);
    const qty = Number(q);
    if (!UUID.test(productId) || !/^\d{1,3}$/.test(q) || qty < 1 || qty > 100) return null;
    if (instance !== undefined && !INSTANCE.test(instance)) return null;
    out.push({ productId, qty, instance: instance ?? null });
  }
  return out.length && out.length <= 50 ? out : null;
}

/**
 * The checkout's own reading of a hand-off (server side, never trusting the URL): only the event's
 * visible products, each instance valid for its product, duplicate lines merged, and every limit
 * applied again (maximum per order, maximum per account). Step 4 re-prices everything in the core
 * from snapshot `v`; this only shows the selection.
 */
export function normaliseSelection(ev: ShopEvent, lines: Line[]): Line[] {
  const byKey = new Map<string, Line>();
  for (const l of lines) {
    const p = ev.products.find((x) => x.id === l.productId);
    if (!p?.sale) continue;
    let inst: string | null = null;
    if (p.sale.choice === "day") inst = l.instance && p.sale.days.includes(l.instance) ? l.instance : null;
    if (p.sale.choice === "slot") {
      const m = l.instance ? /^(\d{4}-\d{2}-\d{2})T(\d{2}:\d{2})$/.exec(l.instance) : null;
      inst = m && p.sale.slots.includes(m[2]) && p.sale.days.some((d) => instanceFor(p, d, m[2]) === l.instance) ? l.instance : null;
    }
    if (p.sale.choice !== "none" && !inst) continue;
    const key = `${p.id}|${inst ?? ""}`;
    const cur = byKey.get(key);
    byKey.set(key, { productId: p.id, instance: inst, qty: (cur?.qty ?? 0) + l.qty });
  }
  const out: Line[] = [];
  const perProduct = new Map<string, number>();
  let tickets = 0;
  const perAccount = ev.sale?.maxPerAccount ?? Infinity;
  for (const l of byKey.values()) {
    const p = ev.products.find((x) => x.id === l.productId)!;
    const room = Math.min(p.sale!.maxPerOrder - (perProduct.get(p.id) ?? 0), p.sale!.isTicket ? perAccount - tickets : Infinity);
    const qty = Math.max(0, Math.min(l.qty, room));
    if (!qty) continue;
    perProduct.set(p.id, (perProduct.get(p.id) ?? 0) + qty);
    if (p.sale!.isTicket) tickets += qty;
    out.push({ ...l, qty });
  }
  return out;
}
