// Fixes from the step 1 adversarial reviews (correctness, security and data, spec and scale).
import { randomUUID } from "node:crypto";
import { describe, expect, it } from "vitest";
import { ladderFromForm } from "@/lib/studio/ladder-form";
import { productSchema, slotSpan } from "@/lib/catalogue/schema";
import { slotStartDate } from "@/lib/catalogue/time";
import { poolInstanceId } from "@/lib/catalogue/pools";
import { slugProblem } from "@/lib/organisers/slugs";
import { completeOnboarding, createBrand, createOrganiser, deleteBrand, revokeInvitation, updateBrand, userOrganisers } from "@/lib/organisers/domain";
import { createPool, loadEventGraph, saveEvent, saveProduct, setInstalmentPlan, setLadder, setProductPools } from "@/lib/catalogue/domain";
import { copyAsEdition } from "@/lib/catalogue/editions";
import { eventReadiness } from "@/lib/catalogue/readiness";
import { asOwner, describeDb, makeOrganiser, makeUser, tx, uniq } from "./db";
import { festival, FESTIVAL_DAYS } from "./catalog-fixtures";

function fd(entries: Record<string, string>) {
  const f = new FormData();
  for (const [k, v] of Object.entries(entries)) f.append(k, v);
  return f;
}

describe("price ladder form", () => {
  it("refuses an empty first price instead of promoting a later step", () => {
    expect(() => ladderFromForm(fd({ mode: "by_date", "steps.0.price": "", "steps.1.threshold": "2027-01-01T00:00", "steps.1.price": "20" }), "Europe/Bucharest")).toThrow(
      expect.objectContaining({ code: "price_required", field: "steps.0.price" }),
    );
  });
  it("reads dates in the event's zone and sales thresholds as numbers", () => {
    const l = ladderFromForm(fd({ mode: "by_date", "steps.0.price": "10", "steps.1.threshold": "2027-01-01T00:00", "steps.1.price": "13" }), "Europe/Bucharest");
    expect(l.steps[1]).toMatchObject({ priceMinor: 1300, fromAt: new Date("2026-12-31T22:00:00Z") });
    expect(() => ladderFromForm(fd({ mode: "by_sales", "steps.0.price": "1", "steps.1.price": "2" }), "UTC")).toThrow(expect.objectContaining({ code: "threshold_required" }));
  });
});

describe("time slots", () => {
  it("a first start equal to the last start is one slot a day (span 0), not 24 hours", () => {
    expect(slotSpan("18:00", "18:00")).toBe(0);
    expect(productSchema.safeParse({ type: "time_slot", name: { ro: "x", en: "x" }, config: { slotMinutes: 45, firstStart: "18:00", lastStart: "18:00", maxConsecutiveSlots: 1 } }).success).toBe(true);
    expect(slotSpan("17:00", "02:30")).toBe(570);
  });
  it("slot instance ids use the local date the slot starts on: after midnight is the next date", () => {
    expect(slotStartDate("2027-07-01", "17:00", "23:30")).toBe("2027-07-01");
    expect(slotStartDate("2027-07-01", "17:00", "01:00")).toBe("2027-07-02");
    expect(poolInstanceId({ id: "p", scope: "slot" }, slotStartDate("2027-07-01", "17:00", "01:00"), "01:00")).toBe("p@2027-07-02T01:00");
  });
});

describe("sample slugs", () => {
  it("are reserved for everyone but the seed", () => {
    for (const s of ["nibiru", "nibiru-resort", "sunwave", "sunwave-festival", "beach-please"]) {
      expect(slugProblem(s), s).toBe("slug_reserved");
      expect(slugProblem(s, { sample: true }), s).toBeNull();
    }
  });
});

describeDb("review fixes in the database", () => {
  it("onboarding twice at once creates one organisation and one brand", async () => {
    const u = await makeUser();
    const [id1, id2] = [randomUUID(), randomUUID()];
    const slug = uniq("dbl");
    const r = await Promise.allSettled([
      tx({ organiserId: id1, userId: u.id }, (c) => createOrganiser(c, { id: id1, userId: u.id, input: { name: "Double", slug } })),
      tx({ organiserId: id2, userId: u.id }, (c) => createOrganiser(c, { id: id2, userId: u.id, input: { name: "Double", slug: `${slug}-b` } })),
    ]);
    expect(r.every((x) => x.status === "fulfilled")).toBe(true);
    const orgs = await tx({ userId: u.id }, (c) => userOrganisers(c, u.id));
    expect(orgs).toHaveLength(1);
    const org = orgs[0].id;
    const brand = { name: "B", slug: uniq("dbl-brand"), languages: ["ro" as const], currency: "RON" as const };
    const [b1, b2] = await Promise.all([
      tx({ organiserId: org, userId: u.id }, (c) => completeOnboarding(c, org, brand)),
      tx({ organiserId: org, userId: u.id }, (c) => completeOnboarding(c, org, { ...brand, slug: `${brand.slug}-x` })),
    ]);
    expect(b1.brandId).toBe(b2.brandId);
  });

  it("copying an edition twice keeps the year in the slug: -2028, then -2028-2", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, async (c) => {
      const x = await festival(c, o.brandId);
      await c.query("update control.events set slug = 'galaxia-2027' where id = $1", [x.ev.id]);
      return x;
    });
    const a = await tx(o.scope, (c) => copyAsEdition(c, f.ev.id));
    const b = await tx(o.scope, (c) => copyAsEdition(c, f.ev.id));
    expect([a.slug, b.slug]).toEqual(["galaxia-2028", "galaxia-2028-2"]);
  });

  it("a brand with published events cannot be deleted; its drafts go with it otherwise", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await asOwner((c) => c.query("update control.events set status = 'published' where id = $1", [f.ev.id]));
    await expect(tx(o.scope, (c) => deleteBrand(c, "owner", o.brandId))).rejects.toMatchObject({ code: "brand_has_published_events" });
    const raw = await tx(o.scope, (c) => c.query("delete from control.brands where id = $1", [o.brandId])).catch((e) => e);
    expect(raw.code).toBe("23001"); // restrict_violation: ON DELETE RESTRICT
    await asOwner((c) => c.query("update control.events set status = 'draft' where id = $1", [f.ev.id]));
    await tx(o.scope, (c) => deleteBrand(c, "owner", o.brandId));
    expect((await tx(o.scope, (c) => c.query("select count(*)::int as n from control.events"))).rows[0].n).toBe(0);
  });

  it("brand currency is locked once it has events, and events move only between same-currency brands", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    const brand = { name: "B", slug: f.ev.slug, languages: ["ro" as const, "en" as const], currency: "EUR" as const };
    await expect(tx(o.scope, async (c) => updateBrand(c, o.organiserId, o.brandId, { ...brand, slug: (await c.query("select slug from control.brands where id = $1", [o.brandId])).rows[0].slug }))).rejects.toMatchObject({ code: "currency_locked" });
    const eur = await tx(o.scope, (c) => createBrand(c, o.organiserId, { ...brand, slug: uniq("eur") }));
    await expect(
      tx(o.scope, (c) => saveEvent(c, { brandId: eur.id, venueId: f.venue.id, slug: f.ev.slug, name: f.ev.name, description: f.ev.description, startsAt: f.ev.starts_at, endsAt: f.ev.ends_at }, f.ev.id)),
    ).rejects.toMatchObject({ code: "currency_mismatch" });
  });

  it("a sample organiser's brand addresses cannot be changed in Studio", async () => {
    const o = await makeOrganiser();
    await asOwner((c) => c.query("update control.organisers set is_sample = true where id = $1", [o.organiserId]));
    await expect(tx(o.scope, (c) => updateBrand(c, o.organiserId, o.brandId, { name: "x", slug: uniq("new"), languages: ["ro"], currency: "RON" }))).rejects.toMatchObject({ code: "sample_locked" });
  });

  it("readiness: a pass, flexible day or add-on on a day pool, and two slot grids on one slot pool, are flagged", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await tx(o.scope, async (c) => {
      const day = await createPool(c, f.ev.id, { name: "Per day", capacity: 10, scope: "day" });
      await setProductPools(c, f.ga.id, [day.id]);
      const slot = await createPool(c, f.ev.id, { name: "Courts", capacity: 2, scope: "slot" });
      for (const [first, last] of [["17:00", "22:00"], ["18:00", "22:00"]]) {
        const p = await saveProduct(c, f.ev.id, { type: "time_slot", name: { ro: "Padel", en: "Padel" }, config: { slotMinutes: 30, firstStart: first, lastStart: last, maxConsecutiveSlots: 2 } });
        await setLadder(c, p.id, { mode: "by_date", steps: [{ priceMinor: 5000 }] });
        await setProductPools(c, p.id, [slot.id]);
      }
    });
    const codes = eventReadiness((await tx(o.scope, (c) => loadEventGraph(c, f.ev.id)))!).map((p) => p.code).sort();
    expect(codes).toEqual(["pool_scope_invalid", "slot_grid_mismatch"]);
  });

  it("a deposit can be a percentage, never both kinds", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    const now = new Date("2026-12-05T10:00:00Z");
    await tx(o.scope, (c) => setInstalmentPlan(c, f.ga.id, { depositBps: 1000, monthlyCount: 3 }, now));
    const g = await tx(o.scope, (c) => loadEventGraph(c, f.ev.id));
    expect(g!.plans[0]).toMatchObject({ deposit_minor: null, deposit_bps: 1000, monthly_count: 3 });
    await expect(tx(o.scope, (c) => setInstalmentPlan(c, f.ga.id, { depositBps: 1000, depositMinor: 100, monthlyCount: 3 }, now))).rejects.toThrow();
    const raw = await tx(o.scope, (c) => c.query("update control.instalment_plans set deposit_minor = 5 where product_id = $1", [f.ga.id])).catch((e) => e);
    expect(raw.code).toBe("23514");
    expect(FESTIVAL_DAYS).toHaveLength(4);
  });

  it("malformed ids on revoke are refused as not found", async () => {
    const o = await makeOrganiser();
    await expect(tx(o.scope, (c) => revokeInvitation(c, "owner", "not-a-uuid"))).rejects.toMatchObject({ code: "not_found" });
  });
});
