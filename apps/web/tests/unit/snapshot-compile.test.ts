// Sale-snapshots spec: compiling at publish, design-only publishes, restores, pool instances, the
// precomputed amounts, concurrent publishes, and the shared schema for every seeded event.
import { expect, it, describe } from "vitest";
import { loadEventGraphs, saveEvent, savePool, saveProduct, saveVenue, setDays, setLadder, setProductPools } from "@/lib/catalogue/domain";
import { zonedToUtc, dateRange } from "@/lib/catalogue/time";
import { compileTerms, slotStarts } from "@/lib/snapshot/compile";
import { compileExtras, snapshotHook, noSalesBeforeFirstPublish, type CompiledEvent } from "@/lib/snapshot/publish";
import type { SaleSnapshot } from "@/lib/snapshot/types";
import { getStorefront, publishDraft, restoreVersion, saveDraft } from "@/lib/storefront/domain";
import { withTx } from "@/lib/db/tx";
import { applySeed, organiserId, SAMPLE_ORGANISERS } from "../../db/seed/apply";
import { asOwner, describeDb, makeOrganiser, testPool, tx, uniq } from "./db";
import { clone, shopOrganiser } from "./storefront-fixtures";
import { validateSnapshot } from "./snapshot-schema";

type Shop = Awaited<ReturnType<typeof shopOrganiser>>;

async function publish(o: Shop, revision: number): Promise<CompiledEvent[]> {
  let out: CompiledEvent[] = [];
  await tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision, userId: o.user.id }, [snapshotHook({ usage: noSalesBeforeFirstPublish, userId: o.user.id, onCompiled: (e) => (out = e) })]));
  return out;
}

async function liveSnapshot(o: { scope: Shop["scope"] }, eventId: string): Promise<SaleSnapshot & { _version: number }> {
  return tx(o.scope, async (c) => {
    const r = await c.query("select version, document_text from control.live_snapshot($1, null)", [eventId]);
    return { ...JSON.parse(r.rows[0].document_text), _version: r.rows[0].version };
  });
}

async function snapshotCount(eventId: string): Promise<number> {
  return asOwner(async (c) => (await c.query("select count(*)::int as n from control.sale_snapshots where event_id = $1", [eventId])).rows[0].n);
}

describeDb("sale snapshot compiler", () => {
  it("first publish of a festival makes version 1 live with products, pools, ladders, fee policy, rules and the GA amounts", async () => {
    const o = await shopOrganiser();
    const compiled = await publish(o, 1);
    expect(compiled).toEqual([expect.objectContaining({ eventId: o.f.ev.id, version: 1, created: true })]);
    const s = await liveSnapshot(o, o.f.ev.id);
    expect(s._version).toBe(1);
    const { _version: _v, ...doc } = s; // eslint-disable-line @typescript-eslint/no-unused-vars
    expect(validateSnapshot(doc)).toBeNull();
    expect(s).toMatchObject({ schemaVersion: 1, version: 1, organiserId: o.organiserId, brandId: o.brandId, eventId: o.f.ev.id, currency: "RON", isSample: false });
    expect(s.fee).toEqual({ policy: "pass_on", bps: 250, fixedMinor: 0 });
    expect(s.event).toMatchObject({ slug: o.f.ev.slug, venue: { name: "NIBIRU Arena" }, days: expect.arrayContaining([{ date: "2027-07-15", opens: "17:00", closes: "03:00" }]) });
    expect(s.pools).toEqual([{ poolId: o.f.arena.id, name: "Arena", scope: "event", instances: [{ instanceId: o.f.arena.id, date: null, slotStart: null, capacity: 40000 }] }]);
    const ga = s.products.find((p) => p.productId === o.f.ga.id)!;
    expect(ga.priceSteps).toEqual([{ index: 0, mode: "by_date", fromAt: null, afterSold: null, faceMinor: 53900, allInMinor: 55248, fanFeeMinor: 1348, organiserNetMinor: 53900 }]);
    expect(ga.poolIds).toEqual([o.f.arena.id]);
    expect(s.rules).toEqual({ minAge: null, consentUnderAge: null, maxPerAccount: null });
    const ev = await tx(o.scope, async (c) => (await c.query("select status, live_snapshot_version from control.events where id = $1", [o.f.ev.id])).rows[0]);
    expect(ev).toEqual({ status: "published", live_snapshot_version: 1 });
  });

  it("a design-only publish creates a new storefront version and no new snapshot", async () => {
    const o = await shopOrganiser();
    await publish(o, 1);
    const d = clone((await tx(o.scope, (c) => getStorefront(c, o.sf.id)))!.document);
    d.tokens.light.primary = "#123456";
    const rev = (await tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: 1, document: d }))).revision;
    const compiled = await publish(o, rev);
    expect(compiled).toEqual([expect.objectContaining({ version: 1, created: false })]);
    expect(await snapshotCount(o.f.ev.id)).toBe(1);
    expect((await tx(o.scope, (c) => getStorefront(c, o.sf.id)))!.live_number).toBe(2);
  });

  it("a restore makes the old look live with the current price, not the price at the time", async () => {
    const o = await shopOrganiser();
    await publish(o, 1);
    await tx(o.scope, (c) => setLadder(c, o.f.ga.id, { mode: "by_date", steps: [{ priceMinor: 59900 }] }));
    const sf = (await tx(o.scope, (c) => getStorefront(c, o.sf.id)))!;
    let compiled: CompiledEvent[] = [];
    const v = await tx(o.scope, (c) =>
      restoreVersion(c, { storefrontId: o.sf.id, number: 1, revision: sf.draft_revision, userId: o.user.id }, [snapshotHook({ usage: noSalesBeforeFirstPublish, userId: o.user.id, onCompiled: (e) => (compiled = e) })]),
    );
    expect(v).toMatchObject({ number: 2, restored_from: 1 });
    expect(compiled).toEqual([expect.objectContaining({ version: 2, created: true })]);
    const s = await liveSnapshot(o, o.f.ev.id);
    expect(s.products[0].priceSteps[0]).toMatchObject({ faceMinor: 59900, allInMinor: 61398 });
  });

  it("two publishes at the same moment neither duplicate nor skip a version", async () => {
    const o = await shopOrganiser();
    const run = () =>
      withTx(testPool(), o.scope, (c) =>
        publishDraft(c, { storefrontId: o.sf.id, revision: 1, userId: null }, [snapshotHook({ usage: noSalesBeforeFirstPublish, userId: null })]),
      );
    const [a, b] = await Promise.all([run(), run()]);
    expect([a.created, b.created].sort()).toEqual([false, true]);
    await tx(o.scope, (c) => setLadder(c, o.f.ga.id, { mode: "by_date", steps: [{ priceMinor: 54900 }] }));
    const d = clone((await tx(o.scope, (c) => getStorefront(c, o.sf.id)))!.document);
    d.tokens.light.primary = "#224466";
    const rev = (await tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: 1, document: d }))).revision;
    const again = await Promise.allSettled([run(), withTx(testPool(), o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: rev, userId: null }, [snapshotHook({ usage: noSalesBeforeFirstPublish, userId: null })]))]);
    expect(again.filter((r) => r.status === "fulfilled").length).toBeGreaterThanOrEqual(1);
    const versions = await asOwner(async (c) => (await c.query("select version from control.sale_snapshots where event_id = $1 order by version", [o.f.ev.id])).rows.map((r) => r.version));
    expect(versions).toEqual([1, 2]);
  });

  it("a day-scoped pool of a 74-day season lists 74 instances with stable ids", async () => {
    const o = await makeOrganiser();
    const g = await tx(o.scope, async (c) => {
      const venue = await saveVenue(c, { name: "Promenada" });
      const ev = await saveEvent(c, {
        brandId: o.brandId,
        venueId: venue.id,
        slug: uniq("promenada"),
        name: { ro: "Promenada", en: "Promenada" },
        description: { ro: "d", en: "d" },
        startsAt: zonedToUtc("2027-06-24T17:00", "Europe/Bucharest")!,
        endsAt: zonedToUtc("2027-09-06T03:00", "Europe/Bucharest")!,
      });
      const days = dateRange("2027-06-24", "2027-09-05");
      await setDays(c, ev.id, days.map((day) => ({ day, opens: "17:00", closes: "03:00" })));
      const pool = await savePool(c, ev.id, { name: "Per day", capacity: 8000, scope: "day" });
      const p = await saveProduct(c, ev.id, { type: "day_ticket", name: { ro: "Zi", en: "Day" }, config: { day: null } });
      await setProductPools(c, p.id, [pool.id]);
      await setLadder(c, p.id, { mode: "by_date", steps: [{ priceMinor: 1000 }] });
      const graphs = await loadEventGraphs(c, [ev.id]);
      return { terms: compileTerms(graphs[0], (await compileExtras(c, graphs)).get(ev.id)!), pool };
    });
    const inst = g.terms.pools[0].instances;
    expect(inst).toHaveLength(74);
    expect(inst[0]).toEqual({ instanceId: `${g.pool.id}@2027-06-24`, date: "2027-06-24", slotStart: null, capacity: 8000 });
    expect(inst.at(-1)!.instanceId).toBe(`${g.pool.id}@2027-09-05`);
  });

  it("a padel pool of 2 courts, 30-minute slots 17:00 to 02:30, lists 20 slot instances a day", async () => {
    expect(slotStarts({ slotMinutes: 30, firstStart: "17:00", lastStart: "02:30" })).toHaveLength(20);
    const o = await makeOrganiser();
    const g = await tx(o.scope, async (c) => {
      const venue = await saveVenue(c, { name: "Padel" });
      const ev = await saveEvent(c, {
        brandId: o.brandId,
        venueId: venue.id,
        slug: uniq("padel"),
        name: { ro: "Padel", en: "Padel" },
        description: { ro: "d", en: "d" },
        startsAt: zonedToUtc("2027-07-01T17:00", "Europe/Bucharest")!,
        endsAt: zonedToUtc("2027-07-02T03:00", "Europe/Bucharest")!,
      });
      await setDays(c, ev.id, [{ day: "2027-07-01", opens: "17:00", closes: "03:00" }]);
      const pool = await savePool(c, ev.id, { name: "Courts", capacity: 2, scope: "slot" });
      const p = await saveProduct(c, ev.id, { type: "time_slot", name: { ro: "Teren", en: "Court" }, config: { slotMinutes: 30, firstStart: "17:00", lastStart: "02:30", maxConsecutiveSlots: 4 } });
      await setProductPools(c, p.id, [pool.id]);
      await setLadder(c, p.id, { mode: "by_date", steps: [{ priceMinor: 5000 }] });
      const graphs = await loadEventGraphs(c, [ev.id]);
      return { terms: compileTerms(graphs[0], (await compileExtras(c, graphs)).get(ev.id)!), pool };
    });
    const inst = g.terms.pools[0].instances;
    expect(inst).toHaveLength(20);
    expect(inst.every((i) => i.capacity === 2)).toBe(true);
    expect(inst[0]).toEqual({ instanceId: `${g.pool.id}@2027-07-01T17:00`, date: "2027-07-01", slotStart: "17:00", capacity: 2 });
    // A slot after midnight belongs to the local date it starts on.
    expect(inst.find((i) => i.slotStart === "01:00")!.instanceId).toBe(`${g.pool.id}@2027-07-02T01:00`);
  });
});

describeDb("seeded events against the shared schema", () => {
  it("every seeded event compiles to a document that validates, with all-in = face + fan fee", async () => {
    await applySeed(testPool(), { demoDomain: "demo.eps.invalid", now: new Date("2026-12-05T10:00:00Z") });
    for (const o of SAMPLE_ORGANISERS) {
      const id = organiserId(o);
      await tx({ organiserId: id }, async (c) => {
        const ids = (await c.query<{ id: string }>("select id::text from control.events")).rows.map((r) => r.id);
        const graphs = await loadEventGraphs(c, ids);
        const extras = await compileExtras(c, graphs);
        for (const g of graphs) {
          const t = compileTerms(g, extras.get(g.event.id)!);
          const doc = { ...t, snapshotId: "5e1a7f00-0000-4000-8000-000000000001", version: 1, publishedAt: "2026-12-05T10:00:00.000Z" };
          expect(validateSnapshot(doc), g.event.slug).toBeNull();
          for (const p of t.products) for (const s of p.priceSteps) expect(s.allInMinor).toBe(s.faceMinor + s.fanFeeMinor);
        }
      });
    }
    // The sample shops were published: their events have live snapshots that validate.
    const galaxia = await asOwner(async (c) => (await c.query("select document_text from control.sale_snapshots s join control.events e on e.id = s.event_id where e.slug = 'galaxia-2027' and e.organiser_id = $1", [organiserId(SAMPLE_ORGANISERS[0])])).rows);
    expect(galaxia.length).toBeGreaterThanOrEqual(1);
    const doc = JSON.parse(galaxia[0].document_text) as SaleSnapshot;
    expect(validateSnapshot(doc)).toBeNull();
    expect(doc.isSample).toBe(true);
    expect(doc.products.find((p) => p.name.en === "General Access")!.priceSteps[0]).toMatchObject({ faceMinor: 53900, allInMinor: 55248, fanFeeMinor: 1348, organiserNetMinor: 53900 });
  });
});

describe("snapshot schema validator", () => {
  it("rejects a document with a missing field", () => {
    expect(validateSnapshot({ schemaVersion: 1 })).toMatch(/missing/);
  });
});
