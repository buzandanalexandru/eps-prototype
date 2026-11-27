// Catalogue spec "Venues are reusable" and "Events with days and line-up".
import { describe, expect, it } from "vitest";
import { addLineupSlot, listDays, listLineup, listVenues, saveEvent, setDays } from "@/lib/catalogue/domain";
import { eventSchema, fieldErrors } from "@/lib/catalogue/schema";
import { dayWindow, nightOrder, zonedToUtc } from "@/lib/catalogue/time";
import { describeDb, makeOrganiser, tx, uniq } from "./db";
import { festival, FESTIVAL_DAYS } from "./catalog-fixtures";

describe("event validation", () => {
  it("refuses an end before the start, with the field in error", () => {
    const r = eventSchema.safeParse({
      brandId: "00000000-0000-4000-8000-000000000001",
      slug: "abc",
      startsAt: "2027-07-18T10:00:00Z",
      endsAt: "2027-07-15T10:00:00Z",
    });
    expect(r.success).toBe(false);
    expect(fieldErrors(r.error!)).toMatchObject({ endsAt: "end_before_start" });
  });
  it("a night past midnight belongs to its start date and closes the next morning", () => {
    const w = dayWindow("2027-07-15", "17:00", "03:00", "Europe/Bucharest");
    expect(w.opensAt.toISOString()).toBe("2027-07-15T14:00:00.000Z");
    expect(w.closesAt.toISOString()).toBe("2027-07-16T00:00:00.000Z");
    expect(nightOrder("01:00", "17:00")).toBeGreaterThan(nightOrder("23:00", "17:00"));
  });
  it("local wall times convert across daylight saving", () => {
    expect(zonedToUtc("2027-01-15T10:00", "Europe/Bucharest")!.toISOString()).toBe("2027-01-15T08:00:00.000Z");
    expect(zonedToUtc("2027-07-15T10:00", "Europe/Bucharest")!.toISOString()).toBe("2027-07-15T07:00:00.000Z");
  });
});

describeDb("events, days and line-up", () => {
  it("reuses a venue across two events", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await tx(o.scope, (c) =>
      saveEvent(c, { brandId: o.brandId, venueId: f.venue.id, slug: uniq("second"), name: { ro: "Al doilea", en: "Second" }, startsAt: new Date("2027-08-01T15:00:00Z"), endsAt: new Date("2027-08-02T00:00:00Z") }),
    );
    const venues = await tx(o.scope, (c) => listVenues(c));
    expect(venues).toHaveLength(1);
    expect(venues[0]).toMatchObject({ name: "NIBIRU Arena", events: 2 });
  });

  it("Galaxia 2027: four days closing at 03:00 on their start date; line-up per day and stage in time order", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    const days = await tx(o.scope, (c) => listDays(c, f.ev.id));
    expect(days.map((d) => d.day)).toEqual(FESTIVAL_DAYS);
    expect(days.every((d) => d.opens === "17:00" && d.closes === "03:00")).toBe(true);
    const d1 = days[0].id;
    await tx(o.scope, async (c) => {
      await addLineupSlot(c, f.ev.id, { dayId: d1, artist: "Late act", stage: "Main", starts: "01:00" });
      await addLineupSlot(c, f.ev.id, { dayId: d1, artist: "Headliner", stage: "Main", starts: "23:00" });
      await addLineupSlot(c, f.ev.id, { dayId: d1, artist: "Opener", stage: "Galaxy", starts: "18:00" });
    });
    const { groupLineup } = await import("@/lib/catalogue/lineup");
    const grouped = groupLineup(days, await tx(o.scope, (c) => listLineup(c, f.ev.id)));
    expect(grouped[0].stages.map((s) => [s.stage, s.slots.map((x) => x.artist)])).toEqual([
      ["Galaxy", ["Opener"]],
      ["Main", ["Headliner", "Late act"]],
    ]);
  });

  it("refuses a day outside the event dates with the field in error", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await expect(tx(o.scope, (c) => setDays(c, f.ev.id, [{ day: "2027-07-25", opens: "17:00", closes: "03:00" }]))).rejects.toMatchObject({
      code: "day_outside_event",
      field: "days.0.day",
    });
    // Shrinking the event so a day falls outside is refused too.
    await expect(
      tx(o.scope, (c) =>
        saveEvent(c, { brandId: o.brandId, slug: f.ev.slug, startsAt: f.ev.starts_at, endsAt: new Date("2027-07-16T20:00:00Z") }, f.ev.id),
      ),
    ).rejects.toMatchObject({ code: "day_outside_event" });
  });
});
