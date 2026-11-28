// Sale-snapshots spec: publishing is refused when an event is not ready, when a pool's new capacity
// is below what the core reports as sold or held, and when the core cannot be asked; nothing is
// made live in any of these cases.
import { expect, it } from "vitest";
import { savePool, saveProduct, setLadder } from "@/lib/catalogue/domain";
import { snapshotHook, type UsageSource } from "@/lib/snapshot/publish";
import { coreUsage } from "@/lib/snapshot/core";
import { listVersions, publishDraft } from "@/lib/storefront/domain";
import { asOwner, describeDb, tx } from "./db";
import { shopOrganiser } from "./storefront-fixtures";

type Shop = Awaited<ReturnType<typeof shopOrganiser>>;

function attempt(o: Shop, usage: UsageSource) {
  return tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: 1, userId: o.user.id }, [snapshotHook({ usage, userId: o.user.id })])).catch((e) => e);
}

async function nothingLive(o: Shop) {
  expect(await tx(o.scope, (c) => listVersions(c, o.sf.id))).toEqual([]);
  const n = await asOwner(async (c) => (await c.query("select count(*)::int as n from control.sale_snapshots where event_id = $1", [o.f.ev.id])).rows[0].n);
  expect(n).toBe(0);
}

const zero: UsageSource = async (ids) => new Map(ids.map((id) => [id, {}]));

describeDb("publish gate", () => {
  it("an event with a product that draws on no pool refuses the publish, listing the problem with a link", async () => {
    const o = await shopOrganiser();
    await tx(o.scope, async (c) => {
      const p = await saveProduct(c, o.f.ev.id, { type: "pass", name: { ro: "Fără bazin", en: "No pool" }, config: { zone: "Arena", days: ["2027-07-15"] } });
      await setLadder(c, p.id, { mode: "by_date", steps: [{ priceMinor: 1000 }] });
    });
    const err = await attempt(o, zero);
    expect(err.code).toBe("publish_blocked");
    expect(err.problems).toEqual([expect.objectContaining({ code: "event_not_ready", detail: "product_without_pool", reasonDetail: "No pool", href: expect.stringMatching(new RegExp(`^/studio/events/${o.f.ev.id}/products/.+#pools$`)) })]);
    await nothingLive(o);
  });

  it("capacity below what is sold or held refuses the publish with the numbers", async () => {
    const o = await shopOrganiser();
    await tx(o.scope, (c) => savePool(c, o.f.ev.id, { name: "Arena", capacity: 1000, scope: "event" }, o.f.arena.id));
    const err = await attempt(o, async () => new Map([[o.f.ev.id, { [o.f.arena.id]: 1200 }]]));
    expect(err.code).toBe("publish_blocked");
    expect(err.problems).toEqual([expect.objectContaining({ code: "capacity_below_usage", pool: "Arena", used: 1200, capacity: 1000 })]);
    await nothingLive(o);
  });

  it("a removed pool that has usage refuses the publish", async () => {
    const o = await shopOrganiser();
    const err = await attempt(o, async () => new Map([[o.f.ev.id, { "00000000-0000-4000-8000-0000000000aa@2027-07-15": 3 }]]));
    expect(err.problems).toEqual([expect.objectContaining({ code: "removed_with_usage", used: 3 })]);
    await nothingLive(o);
  });

  it("when the usage check cannot reach the core, publish is refused with a retry message", async () => {
    const o = await shopOrganiser();
    const err = await attempt(o, async () => {
      throw new Error("connect ECONNREFUSED");
    });
    expect(err.code).toBe("publish_blocked");
    expect(err.problems).toEqual([{ code: "core_unreachable", path: "" }]);
    await nothingLive(o);
    // The core client itself: a refused connection and an error answer both throw.
    const refused = coreUsage({ token: "t", baseUrl: "http://127.0.0.1:9", timeoutMs: 500 });
    await expect(refused([o.f.ev.id])).rejects.toThrow();
    const failing = coreUsage({ token: "t", baseUrl: "http://core.test", fetch: async () => new Response("{}", { status: 503 }) });
    await expect(failing([o.f.ev.id])).rejects.toThrow();
    const ok = coreUsage({
      token: "t",
      baseUrl: "http://core.test",
      fetch: async (req) => {
        const r = req as Request;
        expect(r.headers.get("authorization")).toBe("Bearer t");
        return Response.json({ eventId: o.f.ev.id, instances: [{ instanceId: o.f.arena.id, used: 7 }] });
      },
    });
    expect((await ok([o.f.ev.id])).get(o.f.ev.id)).toEqual({ [o.f.arena.id]: 7 });
  });
});
