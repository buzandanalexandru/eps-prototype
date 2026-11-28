// Storefront builder spec "Contrast check with a suggested fix".
import { describe, expect, it } from "vitest";
import { acceptSuggestion, checkContrast, contrastFailures, contrastRatio, hexToOklch, oklchToHex, paletteFromPixels } from "@/lib/storefront/contrast";
import { publishProblems } from "@/lib/storefront/publish-checks";
import { clone, doc, EVENT_ID } from "./storefront-fixtures";

describe("contrast", () => {
  it("computes WCAG ratios", () => {
    expect(contrastRatio("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(contrastRatio("#ffffff", "#ffffff")).toBeCloseTo(1, 5);
    expect(contrastRatio("#777777", "#ffffff")).toBeCloseTo(4.48, 2);
  });

  it("round-trips OKLCH", () => {
    for (const hex of ["#6d28d9", "#0e7490", "#fde047", "#111111"]) {
      const [L, C, h] = hexToOklch(hex);
      expect(oklchToHex(L, C, h)).toBe(hex);
    }
  });

  it("white text on a light yellow primary fails; the suggestion is a darker shade of the same hue that passes", () => {
    const tokens = { ...doc().tokens, mode: "light" as const, light: { ...doc().tokens.light, primary: "#fde047", onPrimary: "#ffffff" } };
    const fail = checkContrast(tokens).find((c) => c.fg === "onPrimary" && c.bg === "primary")!;
    expect(fail.ok).toBe(false);
    expect(fail.ratio).toBeLessThan(4.5);
    expect(fail.fix).toBe("primary");
    const s = fail.suggestion!;
    expect(contrastRatio(s, "#ffffff")).toBeGreaterThanOrEqual(4.5);
    const [L0, , h0] = hexToOklch("#fde047");
    const [L1, , h1] = hexToOklch(s);
    expect(L1).toBeLessThan(L0);
    expect(Math.abs(h1 - h0)).toBeLessThan(0.15);
  });

  it("automatic mode checks both palettes: a failing dark pair is reported and blocks publishing", () => {
    const d = clone(doc());
    d.tokens.mode = "auto";
    d.tokens.dark.onSurface = "#333333"; // dark text on a dark page
    const failing = contrastFailures(d.tokens);
    expect(failing.map((c) => `${c.scheme}:${c.fg}`)).toEqual(["dark:onSurface"]);
    expect(publishProblems(d, { knownEvents: new Set([EVENT_ID]) })).toContainEqual(expect.objectContaining({ code: "contrast", path: "tokens.dark.onSurface" }));
    // In light mode the dark palette is not used, so it is not checked.
    d.tokens.mode = "light";
    expect(contrastFailures(d.tokens)).toEqual([]);
  });

  it("accepting the suggestion makes the pair pass and lifts the block on publishing", () => {
    const d = clone(doc());
    d.tokens.mode = "light";
    d.tokens.light.primary = "#fde047";
    const known = { knownEvents: new Set([EVENT_ID]) };
    expect(publishProblems(d, known).some((p) => p.code === "contrast")).toBe(true);
    let tokens = d.tokens;
    for (const c of contrastFailures(tokens)) tokens = acceptSuggestion(tokens, c);
    // Fixing the button colour can make it fail against the page; accept again until clean.
    for (const c of contrastFailures(tokens)) tokens = acceptSuggestion(tokens, c);
    expect(contrastFailures(tokens)).toEqual([]);
    expect(publishProblems({ ...d, tokens }, known)).toEqual([]);
  });

  it("proposes a passing palette from a logo's pixels", () => {
    const px: number[] = [];
    for (let i = 0; i < 400; i++) px.push(i % 4 === 0 ? 230 : 20, i % 4 === 0 ? 30 : 120, i % 4 === 0 ? 60 : 220, 255); // red and blue
    for (let i = 0; i < 400; i++) px.push(255, 255, 255, 255, 0, 0, 0, 0); // white and transparent are ignored
    const p = paletteFromPixels(px)!;
    expect(p).not.toBeNull();
    const [, , h] = hexToOklch(p.light.primary);
    expect(Math.abs(h - hexToOklch("#1478dc")[2])).toBeLessThan(0.3);
    expect(contrastFailures({ mode: "auto", ...p })).toEqual([]);
    expect(paletteFromPixels([255, 255, 255, 255])).toBeNull();
  });
});
