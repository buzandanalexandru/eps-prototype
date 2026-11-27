// Catalogue spec "Instalment settings per product".
import { describe, expect, it } from "vitest";
import vectors from "@/lib/pricing/vectors.json";
import { feeBreakdown, instalmentSchedule, toMajorString } from "@/lib/pricing";

describe("instalment vectors", () => {
  for (const v of vectors.instalments) {
    it(v.name, () => {
      const s = instalmentSchedule({
        allInMinor: v.allInMinor,
        depositMinor: v.depositMinor,
        monthlyCount: v.monthlyCount,
        dayOfMonth: v.dayOfMonth,
        from: new Date(v.from),
        eventStartsAt: new Date(v.eventStartsAt),
      });
      if (v.ok) {
        if (!s.ok) throw new Error(`expected a schedule, got ${s.code}`);
        expect(s.payments.map((p) => p.amountMinor)).toEqual(v.amounts);
        expect(s.payments[0].dueOn).toBe(v.firstDue);
        expect(s.payments.at(-1)?.dueOn).toBe(v.lastDue);
      } else {
        expect(s.ok).toBe(false);
        expect(s).toMatchObject({ code: v.code, ...("maxCount" in v ? { maxCount: v.maxCount } : {}) });
      }
    });
  }
});

describe("instalment scenarios", () => {
  const event = new Date("2027-07-15T14:00:00Z");
  it("NIBIRU-style plan: 10.00 now and 9 payments adding up to 542.48, remainder on the last", () => {
    const allIn = feeBreakdown(53900, "pass_on", vectors.platformFee).allInMinor;
    const s = instalmentSchedule({ allInMinor: allIn, depositMinor: 1000, monthlyCount: 9, dayOfMonth: 1, from: new Date("2026-10-01"), eventStartsAt: event });
    if (!s.ok) throw new Error(s.code);
    expect(toMajorString(s.depositMinor)).toBe("10.00");
    expect(s.payments).toHaveLength(9);
    expect(toMajorString(s.payments.reduce((a, p) => a + p.amountMinor, 0))).toBe("542.48");
    const amounts = s.payments.map((p) => p.amountMinor);
    expect(new Set(amounts.slice(0, 8)).size).toBe(1);
    expect(amounts[8]).toBeGreaterThanOrEqual(amounts[0]);
  });
  it("a plan that runs past the event is refused with the latest possible count", () => {
    const s = instalmentSchedule({ allInMinor: 55248, depositMinor: 1000, monthlyCount: 12, dayOfMonth: 15, from: new Date("2026-10-01"), eventStartsAt: event });
    // 15 Nov … 15 Jun fit (last allowed day 8 Jul): 8 payments.
    expect(s).toEqual({ ok: false, code: "past_event", maxCount: 8 });
  });
  it("the last payment may fall exactly 7 days before the event", () => {
    const s = instalmentSchedule({ allInMinor: 10000, depositMinor: 0, monthlyCount: 1, dayOfMonth: 8, from: new Date("2027-06-10"), eventStartsAt: event });
    expect(s.ok && s.payments[0].dueOn).toBe("2027-07-08");
  });
  it("a deposit not below the all-in price is refused", () => {
    expect(instalmentSchedule({ allInMinor: 5000, depositMinor: 6000, monthlyCount: 2, dayOfMonth: 1, from: new Date("2026-10-01"), eventStartsAt: event })).toEqual({ ok: false, code: "deposit_too_large" });
  });
  it("invalid counts and days are refused", () => {
    expect(instalmentSchedule({ allInMinor: 5000, depositMinor: 0, monthlyCount: 0, dayOfMonth: 1, from: new Date(), eventStartsAt: event }).ok).toBe(false);
    expect(instalmentSchedule({ allInMinor: 5000, depositMinor: 0, monthlyCount: 2, dayOfMonth: 31, from: new Date(), eventStartsAt: event }).ok).toBe(false);
  });
});
