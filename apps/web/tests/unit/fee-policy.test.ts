// Organisers spec "Fee policy switch" and "The platform fee is published".
import { describe, expect, it } from "vitest";
import { effectivePolicy, feeBreakdown, formatMoney } from "@/lib/pricing";
import { getFeePolicy, getPlatformFee, setEventFeePolicy, setFeePolicy } from "@/lib/organisers/domain";
import { loadEventGraph } from "@/lib/catalogue/domain";
import { priceSides } from "@/lib/catalogue/display";
import { describeDb, makeOrganiser, tx } from "./db";
import { festival } from "./catalog-fixtures";

describe("policy in force", () => {
  it("an event override wins over the organiser policy; no setting means pass on", () => {
    expect(effectivePolicy("pass_on", "absorb")).toBe("absorb");
    expect(effectivePolicy("pass_on", null)).toBe("pass_on");
    expect(effectivePolicy(null, null)).toBe("pass_on");
  });
  it("Studio shows both sides for 539.00 under pass on", () => {
    const b = feeBreakdown(53900, "pass_on", { percentBps: 250, fixedMinor: 0 });
    expect(formatMoney(b.allInMinor, "RON", "en")).toBe("552.48 RON");
    expect(formatMoney(b.organiserNetMinor, "RON", "en")).toBe("539.00 RON");
  });
});

describeDb("fee policy in the database", () => {
  it("the published fee is 2.5% and no fixed part, for RON and EUR", async () => {
    const o = await makeOrganiser();
    expect(await tx(o.scope, (c) => getPlatformFee(c, "RON"))).toEqual({ percentBps: 250, fixedMinor: 0 });
    expect(await tx(o.scope, (c) => getPlatformFee(c, "EUR"))).toEqual({ percentBps: 250, fixedMinor: 0 });
    // Nobody writes it from the app.
    await expect(tx(o.scope, (c) => c.query("update control.platform_fee set percent_bps = 0"))).rejects.toMatchObject({ code: "42501" });
  });

  it("organiser pass on with one event set to absorb: that event absorbs, the others pass on", async () => {
    const o = await makeOrganiser();
    const [one, two] = await tx(o.scope, async (c) => [await festival(c, o.brandId), await festival(c, o.brandId)]);
    await tx(o.scope, async (c) => {
      await setFeePolicy(c, "owner", "pass_on");
      await setEventFeePolicy(c, "owner", one.ev.id, "absorb");
    });
    expect(await tx(o.scope, (c) => getFeePolicy(c))).toBe("pass_on");
    const [g1, g2] = await tx(o.scope, async (c) => [await loadEventGraph(c, one.ev.id), await loadEventGraph(c, two.ev.id)]);
    expect(effectivePolicy(g1!.organiserPolicy, g1!.event.fee_policy)).toBe("absorb");
    expect(effectivePolicy(g2!.organiserPolicy, g2!.event.fee_policy)).toBe("pass_on");
    expect(priceSides(g1!, 53900)).toMatchObject({ allInMinor: 53900, organiserNetMinor: 52552 });
    expect(priceSides(g2!, 53900)).toMatchObject({ allInMinor: 55248, organiserNetMinor: 53900 });
    // Clearing the override falls back to the organiser policy.
    await tx(o.scope, (c) => setEventFeePolicy(c, "owner", one.ev.id, null));
    const g3 = await tx(o.scope, (c) => loadEventGraph(c, one.ev.id));
    expect(effectivePolicy(g3!.organiserPolicy, g3!.event.fee_policy)).toBe("pass_on");
  });

  it("organisers' policies stay apart", async () => {
    const a = await makeOrganiser();
    const b = await makeOrganiser();
    await tx(a.scope, (c) => setFeePolicy(c, "owner", "split"));
    expect(await tx(b.scope, (c) => getFeePolicy(c))).toBe("pass_on");
  });
});
