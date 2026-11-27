// Catalogue spec "Instalment settings per product".
import { describe, expect, it } from "vitest";
import vectors from "@/lib/pricing/vectors.json";
import { depositFor, feeBreakdown, instalmentSchedule, maxMonthlyPayments, toMajorString } from "@/lib/pricing";

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
        timeZone: v.timeZone,
        capToFit: "capToFit" in v ? Boolean(v.capToFit) : false,
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
  const event = new Date("2027-07-15T14:00:00Z"); // 17:00 in Bucharest
  const demoDay = new Date("2026-12-05T10:00:00Z");
  it("NIBIRU-style plan at demo time: 10.00 now and 7 payments adding up to 542.48, remainder on the last", () => {
    const allIn = feeBreakdown(53900, "pass_on", vectors.platformFee).allInMinor;
    const s = instalmentSchedule({ allInMinor: allIn, depositMinor: 1000, monthlyCount: 7, dayOfMonth: 1, from: demoDay, eventStartsAt: event });
    if (!s.ok) throw new Error(s.code);
    expect(toMajorString(s.depositMinor)).toBe("10.00");
    expect(s.payments).toHaveLength(7);
    expect(toMajorString(s.payments.reduce((a, p) => a + p.amountMinor, 0))).toBe("542.48");
  });
  it("a plan that runs past the event is refused with the latest possible count", () => {
    const s = instalmentSchedule({ allInMinor: 55248, depositMinor: 1000, monthlyCount: 12, dayOfMonth: 15, from: demoDay, eventStartsAt: event });
    // 15 Jan … 15 Jun fit (last allowed local day 8 Jul): 6 payments.
    expect(s).toEqual({ ok: false, code: "past_event", maxCount: 6 });
  });
  it("capped per buyer: later buyers get fewer payments, never one inside the last 7 days", () => {
    for (const [from, n] of [["2026-12-05", 7], ["2027-03-10", 4], ["2027-06-20", 1], ["2027-07-01", 0]] as const) {
      expect(maxMonthlyPayments(new Date(`${from}T10:00:00Z`), 1, event)).toBe(n);
    }
    const late = instalmentSchedule({ allInMinor: 55248, depositMinor: 1000, monthlyCount: 7, dayOfMonth: 1, from: new Date("2027-07-01T10:00:00Z"), eventStartsAt: event, capToFit: true });
    expect(late).toEqual({ ok: false, code: "past_event", maxCount: 0 });
  });
  it("the last payment may fall exactly 7 local days before the event", () => {
    const s = instalmentSchedule({ allInMinor: 10000, depositMinor: 0, monthlyCount: 1, dayOfMonth: 8, from: new Date("2027-06-10T10:00:00Z"), eventStartsAt: event });
    expect(s.ok && s.payments[0].dueOn).toBe("2027-07-08");
  });
  it("uses the event's local dates: an event at 01:00 local on 16 Jul is 15 Jul in UTC", () => {
    const night = new Date("2027-07-15T22:00:00Z"); // 16 Jul 01:00 in Bucharest
    expect(maxMonthlyPayments(new Date("2027-06-10T10:00:00Z"), 9, night)).toBe(1); // 9 Jul ≤ 9 Jul
    expect(maxMonthlyPayments(new Date("2027-06-10T10:00:00Z"), 9, night, "UTC")).toBe(0); // 9 Jul > 8 Jul
  });
  it("a deposit not below the all-in price is refused", () => {
    expect(instalmentSchedule({ allInMinor: 5000, depositMinor: 6000, monthlyCount: 2, dayOfMonth: 1, from: demoDay, eventStartsAt: event })).toEqual({ ok: false, code: "deposit_too_large" });
  });
  it("a deposit can be a share of the price", () => {
    expect(depositFor({ depositBps: 1000 }, 55248)).toBe(5525);
    expect(depositFor({ depositMinor: 1000, depositBps: null }, 55248)).toBe(1000);
  });
  it("invalid counts and days are refused", () => {
    expect(instalmentSchedule({ allInMinor: 5000, depositMinor: 0, monthlyCount: 0, dayOfMonth: 1, from: demoDay, eventStartsAt: event }).ok).toBe(false);
    expect(instalmentSchedule({ allInMinor: 5000, depositMinor: 0, monthlyCount: 2, dayOfMonth: 31, from: demoDay, eventStartsAt: event }).ok).toBe(false);
  });
});
