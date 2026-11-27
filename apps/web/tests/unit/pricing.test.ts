// Organisers spec "One all-in price calculation" and catalogue spec "Price ladder by date or by
// sales": the shared vectors, plus the scenarios spelled out.
import { describe, expect, it } from "vitest";
import vectors from "@/lib/pricing/vectors.json";
import {
  activePriceStep,
  activePriceStepIndex,
  feeBreakdown,
  formatMoney,
  ladderProblems,
  nextPriceStep,
  parseMajor,
  toMajorString,
  type FeePolicy,
  type PriceStep,
} from "@/lib/pricing";

const FEE = vectors.platformFee;

function steps(raw: (typeof vectors.ladders)[number]["steps"]): PriceStep[] {
  return raw.map((s) =>
    s.mode === "by_date"
      ? { mode: "by_date", fromAt: "fromAt" in s && s.fromAt ? new Date(s.fromAt) : null, priceMinor: s.priceMinor }
      : { mode: "by_sales", afterSold: "afterSold" in s ? (s.afterSold as number | null) : null, priceMinor: s.priceMinor },
  );
}

describe("fee vectors", () => {
  for (const v of vectors.fees) {
    it(v.name, () => {
      const b = feeBreakdown(v.faceMinor, v.policy as FeePolicy, FEE);
      expect(b).toEqual({
        faceMinor: v.faceMinor,
        feeMinor: v.feeMinor,
        fanFeeMinor: v.fanFeeMinor,
        allInMinor: v.allInMinor,
        organiserNetMinor: v.organiserNetMinor,
      });
      // Fan and organiser always differ by exactly the fee.
      expect(b.allInMinor - b.organiserNetMinor).toBe(b.feeMinor);
    });
  }
});

describe("all-in price scenarios", () => {
  it("pass on: 539.00 RON → fee 13.48, fan pays 552.48", () => {
    const b = feeBreakdown(53900, "pass_on", FEE);
    expect(toMajorString(b.feeMinor)).toBe("13.48");
    expect(toMajorString(b.allInMinor)).toBe("552.48");
  });
  it("split with an odd fee of 0.25: fan +0.12, organiser −0.13", () => {
    const b = feeBreakdown(1000, "split", FEE);
    expect(b.feeMinor).toBe(25);
    expect(b.allInMinor - 1000).toBe(12);
    expect(1000 - b.organiserNetMinor).toBe(13);
  });
  it("absorb 25.00 → fan pays 25.00, organiser receives 24.37", () => {
    const b = feeBreakdown(2500, "absorb", FEE);
    expect(toMajorString(b.allInMinor)).toBe("25.00");
    expect(toMajorString(b.organiserNetMinor)).toBe("24.37");
  });
  it("free product pays no fee under any policy, even with a fixed part", () => {
    for (const p of ["absorb", "pass_on", "split"] as const) {
      expect(feeBreakdown(0, p, { percentBps: 250, fixedMinor: 50 }).feeMinor).toBe(0);
    }
  });
  it("a fixed part is added per unit", () => {
    expect(feeBreakdown(1000, "pass_on", { percentBps: 250, fixedMinor: 50 }).feeMinor).toBe(75);
  });
  it("refuses non-integer or negative prices", () => {
    expect(() => feeBreakdown(10.5, "pass_on", FEE)).toThrow();
    expect(() => feeBreakdown(-1, "pass_on", FEE)).toThrow();
  });
  it("sums of units never round at cart level", () => {
    const unit = feeBreakdown(1000, "split", FEE).allInMinor;
    expect(unit * 3).toBe(3036);
  });
});

describe("ladder vectors", () => {
  for (const v of vectors.ladders) {
    it(v.name, () => {
      const input = { steps: steps(v.steps), at: new Date(v.at), sold: v.sold };
      expect(activePriceStepIndex(input)).toBe(v.activeIndex);
      const next = nextPriceStep(input);
      expect(next ? input.steps.indexOf(next) : null).toBe(v.nextIndex);
    });
  }
});

describe("ladder scenarios", () => {
  const promenada = steps(vectors.ladders[0].steps);
  it("date ladder: on 15 Dec 2026 the price is 11 RON and next is 13 RON from 1 Jan 2027", () => {
    const at = new Date("2026-12-15T12:00:00+02:00");
    expect(activePriceStep({ steps: promenada, at }).priceMinor).toBe(1100);
    const next = nextPriceStep({ steps: promenada, at });
    expect(next?.priceMinor).toBe(1300);
    expect(next?.mode === "by_date" && next.fromAt?.toISOString()).toBe("2026-12-31T22:00:00.000Z");
  });
  it("sales ladder: 9,999 sold → 539, 10,000 sold → 599", () => {
    const ga: PriceStep[] = [
      { mode: "by_sales", afterSold: null, priceMinor: 53900 },
      { mode: "by_sales", afterSold: 10000, priceMinor: 59900 },
    ];
    expect(activePriceStep({ steps: ga, at: new Date(), sold: 9999 }).priceMinor).toBe(53900);
    expect(activePriceStep({ steps: ga, at: new Date(), sold: 10000 }).priceMinor).toBe(59900);
  });
  it("before the first threshold the first step is in force", () => {
    expect(activePriceStep({ steps: promenada, at: new Date("2020-01-01") }).priceMinor).toBe(1000);
  });
  it("ladder problems: unordered, mixed, threshold on the first step", () => {
    expect(ladderProblems(promenada)).toEqual([]);
    expect(ladderProblems([promenada[0], promenada[2], promenada[1]])).toEqual(["ladder_unordered"]);
    expect(ladderProblems([promenada[0], { mode: "by_sales", afterSold: 5, priceMinor: 1 }])).toEqual(["ladder_mixed"]);
    expect(ladderProblems([promenada[1]])).toEqual(["ladder_first_threshold"]);
    expect(ladderProblems([])).toEqual(["ladder_empty"]);
  });
});

describe("money helpers", () => {
  it("parses and prints major units without floats", () => {
    expect(parseMajor("539")).toBe(53900);
    expect(parseMajor("552,48")).toBe(55248);
    expect(parseMajor("0.1")).toBe(10);
    expect(parseMajor("1.234")).toBeNull();
    expect(parseMajor("-1")).toBeNull();
    for (const bad of ["+1", "1 000", "1.000,00", "01", "", "10000000", "1e3"]) expect(parseMajor(bad), bad).toBeNull();
    expect(parseMajor("1000000")).toBe(100_000_000);
    expect(formatMoney(123900, "RON", "ro")).toBe("1.239,00 RON");
    expect(formatMoney(55248, "RON", "en")).toBe("552.48 RON");
  });
});
