// Storefront spec "Ticket picker hands a valid selection to checkout": limits, add-on requirements,
// day and slot choices, the running all-in total and the hand-off URL (design D3.8).
import { describe, expect, it } from "vitest";
import { shopEventFromSnapshot } from "@/lib/storefront/shop-view";
import { addOne, blockedReason, choiceSoldOut, normaliseSelection, canContinue, checkoutHref, instanceFor, missingRequirements, parseItems, removeOne, total, type Availability, type AvailabilityState, type Line, type PickerCtx } from "@/lib/storefront/picker";
import type { SaleSnapshot } from "@/lib/snapshot/types";
import { galaxiaSnapshot, GA, CAMPING } from "./snapshot-fixture";

const NOW = new Date("2026-12-05T10:00:00Z");
const onSale = (snap: SaleSnapshot): Record<string, Availability> =>
  Object.fromEntries(snap.products.map((p) => [p.productId, { status: "on_sale" as const, allInMinor: p.priceSteps[0].allInMinor, priceStepIndex: 0 }]));

function ctxOf(snap: SaleSnapshot, products: Record<string, Availability> | null = onSale(snap), instances: AvailabilityState["instances"] = {}): PickerCtx {
  return { event: shopEventFromSnapshot(snap, null, NOW), availability: products ? { products, instances } : null, now: NOW };
}

describe("ticket picker", () => {
  it("stays at the maximum per order and says why", () => {
    const snap = galaxiaSnapshot();
    snap.rules.maxPerAccount = null;
    const ctx = ctxOf(snap);
    const ga = ctx.event.products.find((p) => p.id === GA)!;
    let lines: Line[] = [];
    for (let i = 0; i < 7; i++) lines = addOne(lines, ga, null, ctx);
    expect(lines).toEqual([{ productId: GA, instance: null, qty: 6 }]);
    expect(blockedReason(lines, ga, null, ctx)).toBe("limit");
    // Few left: the cap follows what remains.
    const few = ctxOf(snap, { ...onSale(snap), [GA]: { status: "on_sale", allInMinor: 55248, priceStepIndex: 0, remaining: 2 } });
    let l2: Line[] = [];
    for (let i = 0; i < 5; i++) l2 = addOne(l2, ga, null, few);
    expect(l2[0].qty).toBe(2);
    expect(removeOne(l2, GA, null)).toEqual([{ productId: GA, instance: null, qty: 1 }]);
  });

  it("an add-on alone cannot continue: Camping needs a Galaxia ticket", () => {
    const snap = galaxiaSnapshot();
    const ctx = ctxOf(snap);
    const camping = ctx.event.products.find((p) => p.id === CAMPING)!;
    const ga = ctx.event.products.find((p) => p.id === GA)!;
    let lines = addOne([], camping, null, ctx);
    expect(missingRequirements(lines, ctx.event).map((p) => p.name.en)).toEqual(["Galaxia camping"]);
    expect(canContinue(lines, ctx)).toBe(false);
    lines = addOne(lines, ga, null, ctx);
    expect(canContinue(lines, ctx)).toBe(true);
  });

  it("2 GA and 1 Camping: the sum of the all-in prices, handed to checkout with the snapshot version", () => {
    const snap = galaxiaSnapshot();
    const ctx = ctxOf(snap);
    const [ga, camping] = [GA, CAMPING].map((id) => ctx.event.products.find((p) => p.id === id)!);
    const lines = addOne(addOne(addOne([], ga, null, ctx), ga, null, ctx), camping, null, ctx);
    expect(total(lines, ctx)).toBe(2 * 55248 + 20398);
    const href = checkoutHref("/s/nibiru/en", ctx.event, lines);
    const u = new URL(href, "http://x");
    expect(u.pathname).toBe("/s/nibiru/en/checkout");
    expect(u.searchParams.get("event")).toBe(snap.eventId);
    expect(u.searchParams.get("v")).toBe("1");
    expect(u.searchParams.get("items")).toBe(`${GA}:2,${CAMPING}:1`);
    expect(parseItems(u.searchParams.get("items"))).toEqual(lines);
  });

  it("asks for the day of a pick-a-day ticket and the day and start time of a slot, and carries that instance", () => {
    const snap = galaxiaSnapshot();
    const day = { ...snap.pools[0], poolId: "0c000000-0000-4000-8000-0000000000d1", scope: "day" as const, instances: [] };
    const slot = { ...snap.pools[0], poolId: "0c000000-0000-4000-8000-0000000000d2", scope: "slot" as const, instances: [] };
    snap.pools.push(day, slot);
    snap.products.push(
      { ...snap.products[0], productId: "0d000000-0000-4000-8000-0000000000d1", type: "day_ticket", config: { day: null }, poolIds: [day.poolId], instalments: null },
      { ...snap.products[0], productId: "0d000000-0000-4000-8000-0000000000d2", type: "time_slot", config: { slotMinutes: 30, firstStart: "17:00", lastStart: "02:30", maxConsecutiveSlots: 4 }, poolIds: [slot.poolId], instalments: null },
    );
    const ctx = ctxOf(snap);
    const pick = ctx.event.products.find((p) => p.id.endsWith("d1"))!;
    const padel = ctx.event.products.find((p) => p.id.endsWith("d2"))!;
    expect(pick.sale!.choice).toBe("day");
    expect(padel.sale!.choice).toBe("slot");
    expect(padel.sale!.slots).toHaveLength(20);
    expect(blockedReason([], pick, null, ctx)).toBe("choose");
    expect(instanceFor(pick, "2027-07-16", null)).toBe("2027-07-16");
    expect(instanceFor(pick, "2030-01-01", null)).toBeNull(); // not an event day
    expect(instanceFor(padel, "2027-07-15", "18:30")).toBe("2027-07-15T18:30");
    expect(instanceFor(padel, "2027-07-15", "01:00")).toBe("2027-07-16T01:00"); // after midnight: next local date
    expect(instanceFor(padel, "2027-07-15", "05:00")).toBeNull();
    const lines = addOne(addOne([], pick, "2027-07-16", ctx), padel, "2027-07-16T01:00", ctx);
    expect(checkoutHref("", ctx.event, lines)).toContain(encodeURIComponent(`${padel.id}:1:2027-07-16T01:00`));
    expect(parseItems(`${padel.id}:1:2027-07-16T01:00`)).toEqual([{ productId: padel.id, qty: 1, instance: "2027-07-16T01:00" }]);
  });

  it("buying is paused while availability is unknown, and blocked before sales open", () => {
    const snap = galaxiaSnapshot();
    const paused = ctxOf(snap, null);
    const ga = paused.event.products.find((p) => p.id === GA)!;
    expect(blockedReason([], ga, null, paused)).toBe("paused");
    expect(addOne([], ga, null, paused)).toEqual([]);
    const notYet = ctxOf(snap, { ...onSale(snap), [GA]: { status: "not_yet", allInMinor: 55248, priceStepIndex: 0, opensAt: "2027-02-01T08:00:00Z" } });
    expect(blockedReason([], ga, null, notYet)).toBe("not_yet");
    expect(parseItems("not-a-uuid:1")).toBeNull();
    expect(parseItems(`${GA}:0`)).toBeNull();
    expect(parseItems(`${GA}:1:2027-07-16T25:00`)).toBeNull();
  });

  it("products sharing a pool share what is left in it", () => {
    const snap = galaxiaSnapshot();
    snap.rules.maxPerAccount = null;
    const plus = { ...snap.products[0], productId: "0d000000-0000-4000-8000-0000000000b1", name: { ro: "GA+", en: "GA+" } };
    snap.products.splice(1, 0, plus);
    const arena = snap.pools[0].poolId;
    const ctx = ctxOf(snap, onSale(snap), { [arena]: { instanceId: arena, status: "on_sale", remaining: 3 } });
    const [ga, gaPlus] = [GA, plus.productId].map((id) => ctx.event.products.find((p) => p.id === id)!);
    let lines: Line[] = [];
    lines = addOne(addOne(lines, ga, null, ctx), ga, null, ctx);
    lines = addOne(addOne(lines, gaPlus, null, ctx), gaPlus, null, ctx);
    expect(lines.reduce((n, l) => n + l.qty, 0)).toBe(3);
    expect(blockedReason(lines, gaPlus, null, ctx)).toBe("limit");
  });

  it("a sold-out day or slot cannot be chosen; the others can", () => {
    const snap = galaxiaSnapshot();
    const day = { ...snap.pools[0], poolId: "0c000000-0000-4000-8000-0000000000d1", scope: "day" as const, instances: [{ instanceId: "x", date: "2027-07-15", slotStart: null, capacity: 8000 }] };
    snap.pools.push(day);
    snap.products.push({ ...snap.products[0], productId: "0d000000-0000-4000-8000-0000000000d1", type: "day_ticket", config: { day: null }, poolIds: [day.poolId], instalments: null });
    const full = `${day.poolId}@2027-07-15`;
    const ctx = ctxOf(snap, onSale(snap), { [full]: { instanceId: full, status: "sold_out", remaining: 0 } });
    const pick = ctx.event.products.find((p) => p.id.endsWith("d1"))!;
    expect(choiceSoldOut(pick, "2027-07-15", ctx)).toBe(true);
    expect(choiceSoldOut(pick, "2027-07-16", ctx)).toBe(false);
    expect(blockedReason([], pick, "2027-07-15", ctx)).toBe("sold_out");
    expect(addOne([], pick, "2027-07-16", ctx)).toEqual([{ productId: pick.id, instance: "2027-07-16", qty: 1 }]);
  });

  it("the checkout's reading of a hand-off trusts nothing: unknown products, bad instances, duplicates and limits", () => {
    const snap = galaxiaSnapshot();
    const ev = shopEventFromSnapshot(snap, null, NOW);
    const lines = normaliseSelection(ev, [
      { productId: GA, instance: null, qty: 5 },
      { productId: GA, instance: null, qty: 5 }, // merged, then capped at 6 per order and 6 per account
      { productId: "0d000000-0000-4000-8000-0000000000ff", instance: null, qty: 1 }, // unknown
      { productId: CAMPING, instance: null, qty: 2 },
    ]);
    expect(lines).toEqual([
      { productId: GA, instance: null, qty: 6 },
      { productId: CAMPING, instance: null, qty: 2 },
    ]);
  });
});
