// Catalogue spec "Copy an event as a new edition".
import { describe, expect, it } from "vitest";
import { copyAsEdition, nextYearOffset, shiftInstant } from "@/lib/catalogue/editions";
import { loadEventGraph, savePool, saveProduct, setInstalmentPlan, setLadder, setProductPools, addLineupSlot } from "@/lib/catalogue/domain";
import { describeDb, makeOrganiser, tx } from "./db";
import { festival, FESTIVAL_DAYS } from "./catalog-fixtures";

describe("edition dates", () => {
  it("next year's offset keeps the weekday", () => {
    const start = new Date("2027-07-15T14:00:00Z"); // Thursday
    const off = nextYearOffset(start, "Europe/Bucharest");
    expect(off % 7).toBe(0);
    expect(shiftInstant(start, off, "Europe/Bucharest").getUTCDay()).toBe(start.getUTCDay());
  });
  it("shifting keeps the local wall time across daylight saving", () => {
    const winter = new Date("2027-01-10T10:00:00Z"); // 12:00 in Bucharest (UTC+2)
    expect(shiftInstant(winter, 182, "Europe/Bucharest").toISOString()).toBe("2027-07-11T09:00:00.000Z"); // 12:00 in summer (UTC+3)
  });
});

describeDb("copy as new edition", () => {
  it("Galaxia 2027 → draft Galaxia 2028 with products, pools, ladder and plans, same weekdays, linked to the original", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await tx(o.scope, async (c) => {
      await addLineupSlot(c, f.ev.id, { dayId: f.days[0].id, artist: "Headliner", stage: "Main", starts: "23:00" });
      const camping = await savePool(c, f.ev.id, { name: "Camping", capacity: 3000 });
      const tent = await saveProduct(c, f.ev.id, { type: "add_on", name: { ro: "Camping", en: "Camping" }, config: { requires: [f.ga.id] }, salesFrom: "2026-10-01T00:00:00Z" });
      await setProductPools(c, tent.id, [camping.id]);
      await setLadder(c, tent.id, { mode: "by_date", steps: [{ priceMinor: 10000 }, { fromAt: "2027-03-01T00:00:00+02:00", priceMinor: 15000 }] });
      await c.query("update control.products set sales_from = '2026-10-01T00:00:00Z' where id = $1", [f.ga.id]);
      await setInstalmentPlan(c, f.ga.id, { depositMinor: 1000, monthlyCount: 9, dayOfMonth: 1 }, new Date("2026-09-01"));
    });
    const copy = await tx(o.scope, (c) => copyAsEdition(c, f.ev.id));
    const [orig, g] = await tx(o.scope, async (c) => [await loadEventGraph(c, f.ev.id), await loadEventGraph(c, copy.id)] as const);
    expect(g!.event).toMatchObject({ status: "draft", edition_of: f.ev.id, name: { ro: "Galaxia 2028", en: "Galaxia 2028" } });
    expect(g!.event.slug).toContain("2028");
    expect(g!.days.map((d) => new Date(`${d.day}T12:00:00Z`).getUTCDay())).toEqual(FESTIVAL_DAYS.map((d) => new Date(`${d}T12:00:00Z`).getUTCDay()));
    expect(g!.days[0].day.startsWith("2028-07")).toBe(true);
    expect(g!.products).toHaveLength(orig!.products.length);
    expect(g!.pools.map((p) => p.name).sort()).toEqual(orig!.pools.map((p) => p.name).sort());
    expect(g!.links).toHaveLength(orig!.links.length);
    expect(g!.plans).toHaveLength(1);
    expect(g!.lineup).toHaveLength(1);
    // Ids are new; the add-on now requires the copy's GA, not the original's.
    expect(g!.products.some((p) => orig!.products.some((q) => q.id === p.id))).toBe(false);
    const tentCopy = g!.products.find((p) => p.type === "add_on")!;
    const gaCopy = g!.products.find((p) => p.type === "pass")!;
    expect(tentCopy.config.requires).toEqual([gaCopy.id]);
    expect((gaCopy.config.days as string[])[0]).toBe(g!.days[0].day);
    // Ladder dates shifted by the same offset as the event.
    const shift = g!.event.starts_at.getTime() - orig!.event.starts_at.getTime();
    const origStep = orig!.steps.find((s) => s.from_at)!;
    const copyStep = g!.steps.find((s) => s.from_at)!;
    expect(Math.abs(copyStep.from_at!.getTime() - origStep.from_at!.getTime() - shift)).toBeLessThanOrEqual(3600_000);
    // A second copy gets its own slug.
    const again = await tx(o.scope, (c) => copyAsEdition(c, f.ev.id));
    expect(again.slug).not.toBe(copy.slug);
  });
});
