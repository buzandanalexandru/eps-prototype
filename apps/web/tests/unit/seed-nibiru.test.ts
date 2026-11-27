// Sample-data spec: the NIBIRU 2027 sample organiser, Sunwave, the seed's rules and repeatability.
import { describe, expect, it } from "vitest";
import { applySeed, DEMO_ORGANISER_USER_ID, organiserId, SAMPLE_ORGANISERS } from "../../db/seed/apply";
import { nibiru } from "../../db/seed/nibiru";
import { sunwave } from "../../db/seed/sunwave";
import { sid, uuidv5 } from "../../db/seed/uuid";
import { loadEventGraph, stepsOf } from "@/lib/catalogue/domain";
import { priceSides } from "@/lib/catalogue/display";
import { eventReadiness } from "@/lib/catalogue/readiness";
import { getFeePolicy, listBrands, listMembers } from "@/lib/organisers/domain";
import { activePriceStep, instalmentSchedule, nextPriceStep, toMajorString } from "@/lib/pricing";
import { planReference } from "@/lib/catalogue/domain";
import { asOwner, describeDb, makeOrganiser, testPool, tx } from "./db";
import { festival } from "./catalog-fixtures";

const DOMAIN = "demo.eps.invalid";
const NOW = new Date("2026-11-27T12:00:00Z");
const NIBIRU = organiserId(nibiru);
const TABLES = ["brands", "fee_policies", "memberships", "venues", "events", "event_days", "lineup_slots", "products", "capacity_pools", "product_pools", "price_steps", "instalment_plans"];

async function snapshot(orgIds: string[]) {
  return asOwner(async (c) => {
    const out: Record<string, unknown[]> = {};
    for (const t of TABLES) {
      const r = await c.query(`select to_jsonb(x) - 'updated_at' as j from control.${t} x where organiser_id = any($1::uuid[])`, [orgIds]);
      out[t] = r.rows.map((row) => JSON.stringify(row.j)).sort();
    }
    out.organisers = (await c.query("select to_jsonb(o) as j from control.organisers o where id = any($1::uuid[]) order by id", [orgIds])).rows.map((r) => JSON.stringify(r.j));
    return out;
  });
}

describe("seed ids", () => {
  it("are RFC 9562 v5 UUIDs, stable per name", () => {
    expect(uuidv5("www.example.com", "6ba7b810-9dad-11d1-80b4-00c04fd430c8")).toBe("2ed6657d-e927-568b-95e1-2665a8aea6a2");
    expect(sid("nibiru", "brand", "resort")).toBe(sid("nibiru", "brand", "resort"));
  });
  it("NIBIRU appears in the seed only", () => {
    expect(SAMPLE_ORGANISERS.map((o) => o.name)).toEqual(["NIBIRU", "Sunwave"]);
  });
});

describeDb("seeded sample organisers", () => {
  it("NIBIRU: two brands, pass on, the demo organiser as owner; Sunwave is separate", async () => {
    await applySeed(testPool(), { demoDomain: DOMAIN, now: NOW });
    const scope = { organiserId: NIBIRU };
    const brands = await tx(scope, (c) => listBrands(c));
    expect(brands.map((b) => [b.name, b.slug, b.template, b.currency, b.languages.join(",")])).toEqual([
      ["NIBIRU Festivals", "nibiru", "festival", "RON", "ro,en"],
      ["NIBIRU Resort", "nibiru-resort", "resort", "RON", "ro,en"],
    ]);
    expect(await tx(scope, (c) => getFeePolicy(c))).toBe("pass_on");
    const members = await tx(scope, (c) => listMembers(c));
    expect(members).toEqual([expect.objectContaining({ email: `demo-organiser@${DOMAIN}`, role: "owner" })]);
    const flags = await asOwner(async (c) => (await c.query("select id, is_sample from control.organisers where id = any($1::uuid[])", [[NIBIRU, organiserId(sunwave)]])).rows);
    expect(flags.every((f) => f.is_sample)).toBe(true);
    // Sunwave: its own brand and address, nothing shared with NIBIRU.
    const sw = await tx({ organiserId: organiserId(sunwave) }, (c) => listBrands(c));
    expect(sw.map((b) => b.slug)).toEqual(["sunwave-festival"]);
    expect(brands.some((b) => sw.some((s) => s.id === b.id))).toBe(false);
  });

  it("Galaxia 2027: GA 539.00, fan pays 552.48, shares Arena with GA Plus, schedule starts with 10.00; camping needs a ticket", async () => {
    await applySeed(testPool(), { demoDomain: DOMAIN, now: NOW });
    const g = (await tx({ organiserId: NIBIRU }, (c) => loadEventGraph(c, sid("nibiru", "event", "galaxia-2027"))))!;
    expect(g.event.name.en).toBe("Galaxia 2027");
    expect(g.days.map((d) => d.day)).toEqual(["2027-07-15", "2027-07-16", "2027-07-17", "2027-07-18"]);
    const byName = (n: string) => g.products.find((p) => p.name.en === n)!;
    const prices = ["General Access", "General Access Plus", "Golden Circle", "VIP", "Ultra VIP"].map((n) => activePriceStep({ steps: stepsOf(g.steps, byName(n).id), at: NOW }).priceMinor);
    expect(prices).toEqual([53900, 65900, 83900, 93900, 123900]);
    const ga = byName("General Access");
    expect(ga.named).toBe(true);
    const sides = priceSides(g, 53900);
    expect(toMajorString(sides.allInMinor)).toBe("552.48");
    const arena = g.pools.find((p) => p.name === "Arena")!;
    const onArena = g.links.filter((l) => l.pool_id === arena.id).map((l) => g.products.find((p) => p.id === l.product_id)!.name.en);
    expect(onArena).toEqual(expect.arrayContaining(["General Access", "General Access Plus"]));
    const plan = g.plans.find((p) => p.product_id === ga.id)!;
    const s = instalmentSchedule({ allInMinor: sides.allInMinor, depositMinor: plan.deposit_minor, monthlyCount: plan.monthly_count, dayOfMonth: plan.day_of_month, from: planReference(ga, NOW), eventStartsAt: g.event.starts_at });
    expect(s.ok && toMajorString(s.depositMinor)).toBe("10.00");
    expect(s.ok && s.payments.length).toBe(9);
    const camping = g.products.find((p) => p.type === "add_on")!;
    expect(camping.config).toEqual({ requires: "any_ticket" });
    expect(g.links.filter((l) => l.product_id === camping.id).map((l) => g.pools.find((p) => p.id === l.pool_id)!.name)).toEqual(["Camping"]);
  });

  it("resort: Promenada date ladder from 10 to 35 RON with a day pool; tables, Any-Day, 25 RON voucher, padel slot pool", async () => {
    await applySeed(testPool(), { demoDomain: DOMAIN, now: NOW });
    const load = (k: string) => tx({ organiserId: NIBIRU }, async (c) => (await loadEventGraph(c, sid("nibiru", "event", k)))!);
    const prom = await load("promenada-2027");
    expect(prom.days).toHaveLength(74);
    const pick = prom.products.find((p) => p.type === "day_ticket")!;
    const steps = stepsOf(prom.steps, pick.id);
    expect(steps[0].priceMinor).toBe(1000);
    expect(steps.at(-1)).toMatchObject({ priceMinor: 3500 });
    expect((steps.at(-1) as { fromAt: Date }).fromAt.toISOString()).toBe("2027-07-23T21:00:00.000Z");
    const at = new Date("2026-12-15T12:00:00+02:00");
    expect(activePriceStep({ steps, at }).priceMinor).toBe(1100);
    expect(nextPriceStep({ steps, at })?.priceMinor).toBe(1200);
    expect(prom.pools).toEqual([expect.objectContaining({ capacity: 8000, scope: "day" })]);
    const anyDay = prom.products.find((p) => p.type === "flex_day")!;
    expect(stepsOf(prom.steps, anyDay.id)[0].priceMinor).toBe(1500);
    expect((anyDay.config.days as string[]).length).toBe(74);

    const bg = await load("beer-garden-bdlp");
    const tables = bg.products.filter((p) => p.type === "table");
    expect(tables.map((t) => [t.config.category, stepsOf(bg.steps, t.id)[0].priceMinor]).sort()).toEqual([["A", 14900], ["B", 9900], ["C", 7900]]);
    expect(bg.plans.every((p) => p.monthly_count === 4 && p.deposit_minor === 0)).toBe(true);

    const cs = await load("center-stage-2027");
    expect(cs.products[0]).toMatchObject({ type: "voucher", config: { spendValueMinor: 2500 } });
    expect(stepsOf(cs.steps, cs.products[0].id)[0].priceMinor).toBe(2500);

    const padel = await load("padel-2027");
    expect(padel.products[0]).toMatchObject({ type: "time_slot", config: { slotMinutes: 30, firstStart: "17:00", lastStart: "02:30", maxConsecutiveSlots: 4 } });
    expect(stepsOf(padel.steps, padel.products[0].id)[0].priceMinor).toBe(5000);
    expect(padel.pools[0].scope).toBe("slot");

    const standup = await load("stand-up-the-fool");
    expect(standup.event.min_age).toBe(16);
  });

  it("every seeded event passes the readiness check", async () => {
    const results = await applySeed(testPool(), { demoDomain: DOMAIN, now: NOW });
    for (const r of results) for (const e of r.events) expect(e.problems, e.key).toEqual([]);
    for (const o of SAMPLE_ORGANISERS) {
      for (const e of o.events) {
        const g = await tx({ organiserId: organiserId(o) }, (c) => loadEventGraph(c, sid(o.key, "event", e.key)));
        expect(eventReadiness(g!, NOW), e.key).toEqual([]);
      }
    }
  });

  it("running twice creates no rows and restores a changed catalogue; a real organiser is untouched", async () => {
    const real = await makeOrganiser();
    await tx(real.scope, (c) => festival(c, real.brandId));
    const realBefore = await snapshot([real.organiserId]);
    const ids = SAMPLE_ORGANISERS.map(organiserId);
    await applySeed(testPool(), { demoDomain: DOMAIN, now: NOW });
    const first = await snapshot(ids);
    // A visitor edits the sample in Studio: a price, a new event.
    await tx({ organiserId: NIBIRU }, async (c) => {
      await c.query("update control.price_steps set price_minor = 1 where organiser_id = $1", [NIBIRU]);
      await c.query("update control.events set fee_policy = 'absorb', max_per_account = 99 where organiser_id = $1", [NIBIRU]);
      await festival(c, sid("nibiru", "brand", "festivals"));
    });
    await applySeed(testPool(), { demoDomain: DOMAIN, now: NOW });
    const second = await snapshot(ids);
    for (const t of Object.keys(first)) expect(second[t], t).toEqual(first[t]);
    expect(await snapshot([real.organiserId])).toEqual(realBefore);
  });

  it("refuses to touch an organiser with a sample id that is not flagged as sample", async () => {
    const fake = { ...sunwave, key: `fake-${Date.now()}`, slug: `fake-${Date.now()}`, brands: [], venues: [], events: [] };
    const id = organiserId(fake);
    await asOwner((c) => c.query("insert into control.organisers (id, slug, name) values ($1, $2, 'Real')", [id, fake.slug]));
    await expect(applySeed(testPool(), { demoDomain: DOMAIN, now: NOW, organisers: [fake] })).rejects.toThrow(/not a sample organiser/);
  });

  it("makes an existing demo organiser user the owner without creating another", async () => {
    await applySeed(testPool(), { demoDomain: DOMAIN, now: NOW });
    const users = await asOwner(async (c) => (await c.query(`select id from control."user" where email = $1`, [`demo-organiser@${DOMAIN}`])).rows);
    expect(users).toHaveLength(1);
    expect(typeof DEMO_ORGANISER_USER_ID).toBe("string");
  });
});
