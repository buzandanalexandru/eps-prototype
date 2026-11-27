// Catalogue spec "Shared capacity pools".
import { describe, expect, it } from "vitest";
import { availableQuantity, instanceCount, poolBars, poolInstanceId, type PoolLite } from "@/lib/catalogue/pools";
import { listPools, listProductPools, listProducts, savePool, saveProduct, setProductPools } from "@/lib/catalogue/domain";
import { describeDb, makeOrganiser, tx } from "./db";
import { festival, FESTIVAL_DAYS } from "./catalog-fixtures";

const arena: PoolLite = { id: "arena", name: "Arena", capacity: 40000, scope: "event" };
const vipPlatform: PoolLite = { id: "vip", name: "VIP platform", capacity: 2000, scope: "event" };
const promenada: PoolLite = { id: "prom", name: "Promenada", capacity: 8000, scope: "day" };

describe("pool arithmetic", () => {
  it("a product in two pools gets the smaller remaining amount", () => {
    const links = [{ product_id: "VIP", pool_id: "vip" }, { product_id: "VIP", pool_id: "arena" }];
    expect(availableQuantity("VIP", [arena, vipPlatform], links)).toBe(2000);
    expect(availableQuantity("VIP", [arena, vipPlatform], links, { arena: 39_500 })).toBe(500);
  });
  it("a day-scoped pool has one counter per day; selling one day never reduces another", () => {
    const links = [{ product_id: "P", pool_id: "prom" }];
    const used = { [poolInstanceId(promenada, "2027-07-01")]: 7999 };
    expect(availableQuantity("P", [promenada], links, used, { day: "2027-07-01" })).toBe(1);
    expect(availableQuantity("P", [promenada], links, used, { day: "2027-07-02" })).toBe(8000);
    expect(instanceCount(promenada, 74, 0)).toBe(74);
    expect(poolInstanceId({ id: "pad", scope: "slot" }, "2027-07-01", "17:30")).toBe("pad@2027-07-01T17:30");
  });
  it("a product without a pool is not available", () => {
    expect(availableQuantity("X", [arena], [])).toBe(0);
  });
});

describeDb("pools in the database", () => {
  it("GA and GA Plus share the Arena pool of 40,000: one bar listing both", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await tx(o.scope, async (c) => {
      const plus = await saveProduct(c, f.ev.id, { type: "pass", name: { ro: "GA Plus", en: "GA Plus" }, config: { zone: "Arena", days: FESTIVAL_DAYS } });
      await setProductPools(c, plus.id, [f.arena.id]);
      const vipPool = await savePool(c, f.ev.id, { name: "VIP platform", capacity: 2000 });
      const vip = await saveProduct(c, f.ev.id, { type: "pass", name: { ro: "VIP", en: "VIP" }, config: { zone: "VIP", days: FESTIVAL_DAYS } });
      await setProductPools(c, vip.id, [vipPool.id, f.arena.id]);
    });
    const [pools, links, products] = await tx(o.scope, async (c) => [await listPools(c, f.ev.id), await listProductPools(c, f.ev.id), await listProducts(c, f.ev.id)] as const);
    const bars = poolBars(pools, links, products);
    const arenaBar = bars.find((b) => b.pool.name === "Arena")!;
    expect(arenaBar.pool.capacity).toBe(40000);
    expect(arenaBar.products.map((p) => p.name.en).sort()).toEqual(["GA", "GA Plus", "VIP"]);
    const vip = products.find((p) => p.name.en === "VIP")!;
    expect(availableQuantity(vip.id, pools, links)).toBe(2000);
  });
  it("re-setting a product's pools replaces the links", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    const other = await tx(o.scope, (c) => savePool(c, f.ev.id, { name: "Other", capacity: 5, scope: "day" }));
    await tx(o.scope, (c) => setProductPools(c, f.ga.id, [other.id]));
    const links = await tx(o.scope, (c) => listProductPools(c, f.ev.id));
    expect(links).toEqual([{ product_id: f.ga.id, pool_id: other.id }]);
  });
});
