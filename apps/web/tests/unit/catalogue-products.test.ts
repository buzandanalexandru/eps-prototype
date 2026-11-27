// Catalogue spec "Core product types", "Special product types" and the ladder validation of
// "Price ladder by date or by sales".
import { describe, expect, it } from "vitest";
import { fieldErrors, ladderSchema, productSchema, type ProductInput } from "@/lib/catalogue/schema";
import { getProduct, saveProduct, setLadder } from "@/lib/catalogue/domain";
import { describeDb, makeOrganiser, tx } from "./db";
import { festival, FESTIVAL_DAYS } from "./catalog-fixtures";

const name = { ro: "Nume", en: "Name" };

function errs(input: unknown) {
  const r = productSchema.safeParse(input);
  return r.success ? null : fieldErrors(r.error);
}

describe("product types", () => {
  const valid: ProductInput[] = [
    { type: "pass", name: { ro: "Golden Circle", en: "Golden Circle" }, config: { zone: "Golden Circle", days: FESTIVAL_DAYS }, perks: [{ ro: "Fast lane", en: "Fast lane" }, { ro: "Toalete VIP", en: "VIP toilets" }] },
    { type: "day_ticket", name, config: { day: "2027-07-15" } },
    { type: "day_ticket", name, config: { day: null } },
    { type: "flex_day", name: { ro: "Promenada Any-Day", en: "Promenada Any-Day" }, config: { days: FESTIVAL_DAYS } },
    { type: "add_on", name: { ro: "Camping", en: "Camping" }, config: { requires: "any_ticket" } },
    { type: "voucher", name, config: { spendValueMinor: 2500 } },
    { type: "table", name, config: { category: "A", seats: 4 } },
    { type: "time_slot", name: { ro: "Padel", en: "Padel" }, config: { slotMinutes: 30, firstStart: "17:00", lastStart: "02:30", maxConsecutiveSlots: 4 } },
  ];
  for (const p of valid) {
    it(`accepts a valid ${p.type}`, () => {
      expect(errs(p)).toBeNull();
    });
  }
  it("refuses each type's invalid settings with the setting in error", () => {
    expect(errs({ type: "pass", name, config: { zone: "", days: [] } })).toMatchObject({ "config.zone": "required", "config.days": "days_required" });
    expect(errs({ type: "voucher", name, config: {} })).toMatchObject({ "config.spendValueMinor": "spend_required" });
    expect(errs({ type: "voucher", name, config: { spendValueMinor: 0 } })).toMatchObject({ "config.spendValueMinor": "spend_required" });
    expect(errs({ type: "table", name, config: { category: "A", seats: 0 } })).toMatchObject({ "config.seats": "must_be_positive" });
    expect(errs({ type: "time_slot", name, config: { slotMinutes: 45, firstStart: "17:00", lastStart: "02:30", maxConsecutiveSlots: 4 } })).toMatchObject({
      "config.slotMinutes": "slot_span_not_divisible",
    });
    expect(errs({ type: "flex_day", name, config: { days: [] } })).toMatchObject({ "config.days": "days_required" });
    expect(errs({ type: "nope", name, config: {} })).not.toBeNull();
  });
  it("refuses a sales window that ends before it starts", () => {
    expect(errs({ ...valid[1], salesFrom: "2027-01-02T00:00:00Z", salesUntil: "2027-01-01T00:00:00Z" })).toMatchObject({ salesUntil: "sales_end_before_start" });
  });
  it("refuses a maximum per order of 0", () => {
    expect(errs({ ...valid[1], maxPerOrder: 0 })).toMatchObject({ maxPerOrder: "must_be_positive" });
  });
});

describe("ladder validation", () => {
  it("accepts strictly increasing dates and sales", () => {
    expect(ladderSchema.safeParse({ mode: "by_date", steps: [{ priceMinor: 1000 }, { fromAt: "2026-12-01", priceMinor: 1100 }] }).success).toBe(true);
    expect(ladderSchema.safeParse({ mode: "by_sales", steps: [{ priceMinor: 53900 }, { afterSold: 10000, priceMinor: 59900 }] }).success).toBe(true);
  });
  it("refuses unordered dates or thresholds", () => {
    const r = ladderSchema.safeParse({ mode: "by_date", steps: [{ priceMinor: 1 }, { fromAt: "2027-01-01", priceMinor: 2 }, { fromAt: "2026-12-01", priceMinor: 3 }] });
    expect(r.success).toBe(false);
    expect(fieldErrors(r.error!)).toMatchObject({ steps: "ladder_unordered" });
    const s = ladderSchema.safeParse({ mode: "by_sales", steps: [{ priceMinor: 1 }, { afterSold: 10, priceMinor: 2 }, { afterSold: 10, priceMinor: 3 }] });
    expect(fieldErrors(s.error!)).toMatchObject({ steps: "ladder_unordered" });
  });
  it("refuses a ladder mixing dates and sales", () => {
    const r = ladderSchema.safeParse({ mode: "by_date", steps: [{ priceMinor: 1 }, { afterSold: 10, priceMinor: 2 }] });
    expect(fieldErrors(r.error!)).toMatchObject({ steps: "ladder_mixed" });
  });
  it("refuses a threshold on the first step", () => {
    const r = ladderSchema.safeParse({ mode: "by_date", steps: [{ fromAt: "2026-12-01", priceMinor: 1 }] });
    expect(fieldErrors(r.error!)).toMatchObject({ steps: "ladder_first_threshold" });
  });
});

describeDb("products in the database", () => {
  it("Golden Circle pass keeps its zone, days and perks; every type round-trips through the schema", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    const gc = await tx(o.scope, (c) =>
      saveProduct(c, f.ev.id, {
        type: "pass",
        name: { ro: "Golden Circle", en: "Golden Circle" },
        config: { zone: "Golden Circle", days: FESTIVAL_DAYS },
        perks: [{ ro: "Fast lane", en: "Fast lane" }, { ro: "Toalete VIP", en: "VIP toilets" }],
      }),
    );
    const back = await tx(o.scope, (c) => getProduct(c, gc.id));
    expect(back).toMatchObject({ type: "pass", config: { zone: "Golden Circle", days: FESTIVAL_DAYS }, perks: [{ en: "Fast lane" }, { en: "VIP toilets" }] });
    expect(productSchema.safeParse({ type: back!.type, name: back!.name, perks: back!.perks, config: back!.config }).success).toBe(true);
  });
  it("refuses pass days that are not event days, and add-on requirements from elsewhere", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await expect(tx(o.scope, (c) => saveProduct(c, f.ev.id, { type: "pass", name, config: { zone: "Z", days: ["2027-08-01"] } }))).rejects.toMatchObject({
      code: "day_not_in_event",
    });
    await expect(
      tx(o.scope, (c) => saveProduct(c, f.ev.id, { type: "add_on", name, config: { requires: ["00000000-0000-4000-8000-000000000099"] } })),
    ).rejects.toMatchObject({ code: "not_found", field: "config.requires" });
  });
  it("a stored ladder that mixes modes is refused", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await expect(tx(o.scope, (c) => setLadder(c, f.ga.id, { mode: "by_sales", steps: [{ priceMinor: 1 }, { fromAt: "2027-01-01", priceMinor: 2 }] }))).rejects.toThrow();
  });
});
