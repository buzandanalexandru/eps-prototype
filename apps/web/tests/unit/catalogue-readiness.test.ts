// Catalogue spec "Readiness check before publishing", "Missing translation", "Add-on requiring a
// ticket" and "Product without a pool or a zero pool".
import { expect, it } from "vitest";
import { loadEventGraph, saveEvent, savePool, saveProduct, setDays, setLadder, setProductPools } from "@/lib/catalogue/domain";
import { eventReadiness } from "@/lib/catalogue/readiness";
import { describeDb, makeOrganiser, tx, uniq } from "./db";
import { festival } from "./catalog-fixtures";

const codes = (ps: { code: string }[]) => ps.map((p) => p.code).sort();

describeDb("readiness check", () => {
  it("a complete festival is ready", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    const g = await tx(o.scope, (c) => loadEventGraph(c, f.ev.id));
    expect(eventReadiness(g!)).toEqual([]);
  });

  it("missing English name saves as a draft and is listed", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await tx(o.scope, (c) => saveEvent(c, { brandId: o.brandId, venueId: f.venue.id, slug: f.ev.slug, name: { ro: "Galaxia 2027", en: "" }, description: f.ev.description, startsAt: f.ev.starts_at, endsAt: f.ev.ends_at }, f.ev.id));
    const ps = eventReadiness((await tx(o.scope, (c) => loadEventGraph(c, f.ev.id)))!);
    expect(ps).toEqual([expect.objectContaining({ code: "missing_translation", field: "name.en", href: `/studio/events/${f.ev.id}#name-en` })]);
  });

  it("a Romanian-only brand does not require English", async () => {
    const o = await makeOrganiser({ languages: ["ro"] });
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await tx(o.scope, (c) => saveEvent(c, { brandId: o.brandId, venueId: f.venue.id, slug: f.ev.slug, name: { ro: "Galaxia", en: "" }, description: { ro: "x", en: "" }, startsAt: f.ev.starts_at, endsAt: f.ev.ends_at }, f.ev.id));
    expect(eventReadiness((await tx(o.scope, (c) => loadEventGraph(c, f.ev.id)))!)).toEqual([]);
  });

  it("product without a pool and a missing English description: both listed with links; fixing them makes it ready", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    const plus = await tx(o.scope, async (c) => {
      await saveEvent(c, { brandId: o.brandId, venueId: f.venue.id, slug: f.ev.slug, name: f.ev.name, description: { ro: "Descriere", en: "" }, startsAt: f.ev.starts_at, endsAt: f.ev.ends_at }, f.ev.id);
      const p = await saveProduct(c, f.ev.id, { type: "day_ticket", name: { ro: "Zi", en: "Day" }, config: { day: "2027-07-15" } });
      await setLadder(c, p.id, { mode: "by_date", steps: [{ priceMinor: 19900 }] });
      return p;
    });
    const ps = eventReadiness((await tx(o.scope, (c) => loadEventGraph(c, f.ev.id)))!);
    expect(codes(ps)).toEqual(["missing_translation", "product_without_pool"]);
    expect(ps.find((p) => p.code === "product_without_pool")!.href).toBe(`/studio/events/${f.ev.id}/products/${plus.id}#pools`);
    await tx(o.scope, async (c) => {
      await setProductPools(c, plus.id, [f.arena.id]);
      await saveEvent(c, { brandId: o.brandId, venueId: f.venue.id, slug: f.ev.slug, name: f.ev.name, description: { ro: "Descriere", en: "Description" }, startsAt: f.ev.starts_at, endsAt: f.ev.ends_at }, f.ev.id);
    });
    expect(eventReadiness((await tx(o.scope, (c) => loadEventGraph(c, f.ev.id)))!)).toEqual([]);
  });

  it("a pool of capacity 0 is listed", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await tx(o.scope, (c) => savePool(c, f.ev.id, { name: "Arena", capacity: 0 }, f.arena.id));
    expect(codes(eventReadiness((await tx(o.scope, (c) => loadEventGraph(c, f.ev.id)))!))).toEqual(["pool_capacity_zero"]);
  });

  it("an add-on requiring any ticket, in an event with no ticket product, is flagged", async () => {
    const o = await makeOrganiser();
    const ev = await tx(o.scope, async (c) => {
      const e = await saveEvent(c, { brandId: o.brandId, slug: uniq("camp"), name: { ro: "Camping", en: "Camping" }, description: { ro: "x", en: "x" }, startsAt: new Date("2027-07-15T14:00:00Z"), endsAt: new Date("2027-07-16T00:00:00Z") });
      await setDays(c, e.id, [{ day: "2027-07-15", opens: "17:00", closes: "23:00" }]);
      const pool = await savePool(c, e.id, { name: "Camping", capacity: 100 });
      const camping = await saveProduct(c, e.id, { type: "add_on", name: { ro: "Camping", en: "Camping" }, config: { requires: "any_ticket" } });
      await setProductPools(c, camping.id, [pool.id]);
      await setLadder(c, camping.id, { mode: "by_date", steps: [{ priceMinor: 5000 }] });
      return e;
    });
    const g = await tx(o.scope, (c) => loadEventGraph(c, ev.id));
    expect(g!.products[0].config).toEqual({ requires: "any_ticket" });
    expect(codes(eventReadiness(g!))).toEqual(["addon_requires_missing", "no_venue"]);
  });

  it("an empty event lists its missing parts", async () => {
    const o = await makeOrganiser();
    const ev = await tx(o.scope, (c) => saveEvent(c, { brandId: o.brandId, slug: uniq("empty"), startsAt: new Date("2027-07-15T14:00:00Z"), endsAt: new Date("2027-07-16T00:00:00Z") }));
    const ps = eventReadiness((await tx(o.scope, (c) => loadEventGraph(c, ev.id)))!);
    expect(new Set(codes(ps))).toEqual(new Set(["missing_translation", "no_venue", "no_days", "no_products"]));
  });
});
