// Catalogue spec "Buying rules".
import { describe, expect, it } from "vitest";
import { fieldErrors, rulesSchema } from "@/lib/catalogue/schema";
import { getEvent, setRules } from "@/lib/catalogue/domain";
import { describeDb, makeOrganiser, tx } from "./db";
import { festival } from "./catalog-fixtures";

describe("buying rules validation", () => {
  it("accepts a minimum age with a higher consent age and a cap per account", () => {
    expect(rulesSchema.parse({ minAge: "16", consentUnderAge: "18", maxPerAccount: "6" })).toEqual({ minAge: 16, consentUnderAge: 18, maxPerAccount: 6 });
    expect(rulesSchema.parse({ minAge: "", consentUnderAge: "", maxPerAccount: "" })).toEqual({ minAge: null, consentUnderAge: null, maxPerAccount: null });
  });
  it("refuses a consent age below the minimum age", () => {
    const r = rulesSchema.safeParse({ minAge: 18, consentUnderAge: 16 });
    expect(fieldErrors(r.error!)).toMatchObject({ consentUnderAge: "consent_below_min_age" });
  });
  it("refuses a maximum of 0 or negative", () => {
    expect(fieldErrors(rulesSchema.safeParse({ maxPerAccount: 0 }).error!)).toMatchObject({ maxPerAccount: "must_be_positive" });
    expect(fieldErrors(rulesSchema.safeParse({ maxPerAccount: -2 }).error!)).toMatchObject({ maxPerAccount: "must_be_positive" });
  });
});

describeDb("buying rules in the database", () => {
  it("saves minimum age 16 on an event", async () => {
    const o = await makeOrganiser();
    const f = await tx(o.scope, (c) => festival(c, o.brandId));
    await tx(o.scope, (c) => setRules(c, f.ev.id, { minAge: 16, maxPerAccount: 4 }));
    expect(await tx(o.scope, (c) => getEvent(c, f.ev.id))).toMatchObject({ min_age: 16, consent_under_age: null, max_per_account: 4 });
  });
});
