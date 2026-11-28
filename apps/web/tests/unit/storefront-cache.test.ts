// Storefront spec "Shop pages are cached until the next publish": a cache hit makes no query, and a
// publish's tags make the next request load the new version.
import { expect, it } from "vitest";
import type pg from "pg";
import { withTx } from "@/lib/db/tx";
import { loadPublishedShop } from "@/lib/storefront/published";
import { memoryCache, publishTags, shopLoader } from "@/lib/storefront/shop-cache";
import { getStorefront, publishDraft, saveDraft } from "@/lib/storefront/domain";
import { noSalesBeforeFirstPublish, snapshotHook } from "@/lib/snapshot/publish";
import { describeDb, testPool, tx } from "./db";
import { clone, shopOrganiser } from "./storefront-fixtures";

/** A pool that counts every query sent through it. */
function countingPool(pool: pg.Pool) {
  let n = 0;
  return {
    get queries() {
      return n;
    },
    connect: async () => {
      const c = await pool.connect();
      const q = c.query.bind(c);
      (c as { query: unknown }).query = (...args: unknown[]) => {
        n++;
        return (q as (...a: unknown[]) => unknown)(...args);
      };
      return c;
    },
  };
}

describeDb("shop cache", () => {
  it("a second request with no publish in between queries nothing; a publish refreshes it", async () => {
    const o = await shopOrganiser();
    const hook = () => [snapshotHook({ usage: noSalesBeforeFirstPublish, userId: null })];
    await tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: 1, userId: null }, hook()));
    const slug = (await tx(o.scope, (c) => c.query("select slug from control.brands where id = $1", [o.brandId]))).rows[0].slug as string;
    const pool = countingPool(testPool());
    const cache = memoryCache();
    const get = shopLoader(cache, (k) => withTx(pool, {}, (c) => loadPublishedShop(c, k)));
    const first = await get({ slug });
    expect(first?.versionNumber).toBe(1);
    const after = pool.queries;
    expect(after).toBeGreaterThan(0);
    const second = await get({ slug });
    expect(pool.queries).toBe(after); // served from cache: no query at all
    expect(second).toBe(first);

    const d = clone((await tx(o.scope, (c) => getStorefront(c, o.sf.id)))!.document);
    d.pages[0].sections[0].settings.heading = { ro: "Titlu nou", en: "New title" };
    const rev = (await tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: 1, document: d }))).revision;
    await tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: rev, userId: null }, hook()));
    for (const t of publishTags({ brandId: o.brandId, slug, hostnames: [], eventIds: [o.f.ev.id] })) cache.revalidate(t);
    const third = await get({ slug });
    expect(third?.versionNumber).toBe(2);
    expect(third?.document.pages[0].sections[0].settings.heading).toEqual({ ro: "Titlu nou", en: "New title" });
    // Malformed addresses never reach the database.
    const before = pool.queries;
    expect(await get({ slug: "NOT A SLUG" })).toBeNull();
    expect(pool.queries).toBe(before);
  });
});
