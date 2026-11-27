// Catalogue spec "Catalogue data stays with its organiser".
import { expect, it } from "vitest";
import { getEvent, linkProductPool, listEvents, saveEvent, savePool, saveProduct, setProductPools } from "@/lib/catalogue/domain";
import { fromPgError } from "@/lib/errors";
import { describeDb, makeOrganiser, tx, uniq } from "./db";
import { festival } from "./catalog-fixtures";

describeDb("catalogue isolation", () => {
  it("linking organiser A's product to organiser B's pool is refused by the database and reported as not found", async () => {
    const a = await makeOrganiser();
    const b = await makeOrganiser();
    const fa = await tx(a.scope, (c) => festival(c, a.brandId));
    const fb = await tx(b.scope, (c) => festival(c, b.brandId));
    // Raw link in A's scope to B's pool: the composite foreign key (organiser_id, pool_id) fails.
    const err = await tx(a.scope, (c) => linkProductPool(c, fa.ga.id, fb.arena.id)).catch((e) => e);
    expect(err.code).toBe("23503");
    expect(fromPgError(err)?.code).toBe("not_found");
    // Through the domain function: B's pool is not visible, so not found before any write.
    await expect(tx(a.scope, (c) => setProductPools(c, fa.ga.id, [fb.arena.id]))).rejects.toMatchObject({ code: "not_found" });
  });

  it("an event cannot use another organiser's brand or venue, and A never sees B's events", async () => {
    const a = await makeOrganiser();
    const b = await makeOrganiser();
    const fb = await tx(b.scope, (c) => festival(c, b.brandId));
    const brandErr = await tx(a.scope, (c) =>
      saveEvent(c, { brandId: b.brandId, slug: uniq("x"), startsAt: new Date("2027-01-01"), endsAt: new Date("2027-01-02") }),
    ).catch((e) => e);
    expect(fromPgError(brandErr)?.code).toBe("not_found");
    const venueErr = await tx(a.scope, (c) =>
      saveEvent(c, { brandId: a.brandId, venueId: fb.venue.id, slug: uniq("x"), startsAt: new Date("2027-01-01"), endsAt: new Date("2027-01-02") }),
    ).catch((e) => e);
    expect(fromPgError(venueErr)?.code).toBe("not_found");
    expect(await tx(a.scope, (c) => getEvent(c, fb.ev.id))).toBeNull();
    expect((await tx(a.scope, (c) => listEvents(c))).map((e) => e.id)).not.toContain(fb.ev.id);
  });

  it("writing a row with another organiser's id is refused by row-level security", async () => {
    const a = await makeOrganiser();
    const b = await makeOrganiser();
    const fb = await tx(b.scope, (c) => festival(c, b.brandId));
    const err = await tx(a.scope, (c) =>
      c.query("insert into control.capacity_pools (organiser_id, event_id, name, capacity) values ($1, $2, 'sneaky', 1)", [b.organiserId, fb.ev.id]),
    ).catch((e) => e);
    expect(err.code).toBe("42501");
    // Upserting B's product id from A's scope cannot take it over.
    const fa = await tx(a.scope, (c) => festival(c, a.brandId));
    const up = await tx(a.scope, (c) => saveProduct(c, fa.ev.id, { type: "add_on", name: { ro: "x", en: "x" }, config: { requires: "none" } }, fb.ga.id)).catch((e) => e);
    expect(fromPgError(up)?.code).toBe("not_found");
    const up2 = await tx(a.scope, (c) => savePool(c, fa.ev.id, { name: "x", capacity: 1 }, fb.arena.id)).catch((e) => e);
    expect(up2).toBeInstanceOf(Error);
  });
});
