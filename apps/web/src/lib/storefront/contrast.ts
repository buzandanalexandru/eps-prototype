// Colour maths for the brand kit (design D2.3): WCAG 2.2 contrast ratios, OKLCH conversion and the
// suggested fix that moves only the failing colour's lightness, keeping its hue. Pure, no deps.
import type { Palette, PaletteKey, Tokens } from "./schema";

export type Rgb = [number, number, number];

export function hexToRgb(hex: string): Rgb {
  const m = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex);
  if (!m) throw new Error("colour must be #rrggbb");
  return [parseInt(m[1], 16), parseInt(m[2], 16), parseInt(m[3], 16)];
}

export function rgbToHex([r, g, b]: Rgb): string {
  return `#${[r, g, b].map((v) => Math.round(Math.min(255, Math.max(0, v))).toString(16).padStart(2, "0")).join("")}`;
}

const toLinear = (c: number) => {
  const s = c / 255;
  return s <= 0.04045 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
};
const fromLinear = (l: number) => 255 * (l <= 0.0031308 ? 12.92 * l : 1.055 * l ** (1 / 2.4) - 0.055);

export function luminance(hex: string): number {
  const [r, g, b] = hexToRgb(hex).map(toLinear);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** WCAG contrast ratio between two colours, 1 to 21. */
export function contrastRatio(a: string, b: string): number {
  const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p);
  return (x + 0.05) / (y + 0.05);
}

// OKLab / OKLCH (Björn Ottosson's matrices).
export function hexToOklch(hex: string): [number, number, number] {
  const [r, g, b] = hexToRgb(hex).map(toLinear);
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  const L = 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s;
  const A = 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s;
  const B = 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s;
  return [L, Math.sqrt(A * A + B * B), Math.atan2(B, A)];
}

function oklchToLinear(L: number, C: number, h: number): [number, number, number] {
  const a = C * Math.cos(h);
  const b = C * Math.sin(h);
  const l = (L + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m = (L - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s = (L - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return [
    4.0767416621 * l - 3.3077115913 * m + 0.2309699292 * s,
    -1.2684380046 * l + 2.6097574011 * m - 0.3413193965 * s,
    -0.0041960863 * l - 0.7034186147 * m + 1.707614701 * s,
  ];
}

/** OKLCH to hex, reducing chroma until the colour fits sRGB (hue and lightness kept). */
export function oklchToHex(L: number, C: number, h: number): string {
  let c = C;
  for (let i = 0; i < 40; i++) {
    const lin = oklchToLinear(L, c, h);
    if (lin.every((v) => v >= -1e-4 && v <= 1 + 1e-4)) return rgbToHex(lin.map((v) => fromLinear(Math.min(1, Math.max(0, v)))) as Rgb);
    c *= 0.9;
  }
  return rgbToHex(oklchToLinear(L, 0, h).map((v) => fromLinear(Math.min(1, Math.max(0, v)))) as Rgb);
}

/**
 * The closest colour of the same hue (only OKLCH lightness moves, in steps of 0.005) that reaches
 * `min` against `other`; null if none does.
 */
export function suggestLightness(colour: string, other: string, min: number): string | null {
  if (contrastRatio(colour, other) >= min) return colour;
  const [L, C, h] = hexToOklch(colour);
  for (let d = 0.005; d <= 1; d += 0.005) {
    for (const cand of [L - d, L + d]) {
      if (cand < 0 || cand > 1) continue;
      const hex = oklchToHex(cand, C, h);
      if (contrastRatio(hex, other) >= min) return hex;
    }
  }
  return null;
}

export type PairCheck = {
  scheme: "light" | "dark";
  /** The colour to change and the one it is measured against. */
  fg: PaletteKey;
  bg: PaletteKey;
  /** Which of the two the suggestion changes. */
  fix: PaletteKey;
  min: number;
  ratio: number;
  ok: boolean;
  suggestion: string | null;
};

// Text pairs need 4.5:1, accents and controls against the page 3:1 (WCAG 2.2 AA, 1.4.3 and 1.4.11).
// The fix changes the colour the organiser is most likely to have picked for the brand: the button
// colour for button text, the text colour for page text.
const PAIRS: { fg: PaletteKey; bg: PaletteKey; fix: PaletteKey; min: number }[] = [
  { fg: "onSurface", bg: "surface", fix: "onSurface", min: 4.5 },
  { fg: "muted", bg: "surface", fix: "muted", min: 4.5 },
  { fg: "onPrimary", bg: "primary", fix: "primary", min: 4.5 },
  { fg: "primary", bg: "surface", fix: "primary", min: 3 },
  { fg: "accent", bg: "surface", fix: "accent", min: 3 },
];

function checkPalette(p: Palette, scheme: "light" | "dark"): PairCheck[] {
  return PAIRS.map(({ fg, bg, fix, min }) => {
    const ratio = contrastRatio(p[fg], p[bg]);
    const ok = ratio >= min;
    let suggestion: string | null = null;
    if (!ok) {
      const other = fix === fg ? p[bg] : p[fg];
      suggestion = suggestLightness(p[fix], other, min);
    }
    return { scheme, fg, bg, fix, min, ratio: Math.round(ratio * 100) / 100, ok, suggestion };
  });
}

/** Every pair in every scheme the brand uses (dark too when the mode is dark or automatic). */
export function checkContrast(tokens: Pick<Tokens, "mode" | "light" | "dark">): PairCheck[] {
  const out: PairCheck[] = [];
  if (tokens.mode !== "dark") out.push(...checkPalette(tokens.light, "light"));
  if (tokens.mode !== "light") out.push(...checkPalette(tokens.dark, "dark"));
  return out;
}

export function contrastFailures(tokens: Pick<Tokens, "mode" | "light" | "dark">): PairCheck[] {
  return checkContrast(tokens).filter((c) => !c.ok);
}

/** Applies a pair's suggestion to the tokens (the organiser's "Accept"). */
export function acceptSuggestion<T extends Pick<Tokens, "light" | "dark">>(tokens: T, check: PairCheck): T {
  if (!check.suggestion) return tokens;
  return { ...tokens, [check.scheme]: { ...tokens[check.scheme], [check.fix]: check.suggestion } };
}

/**
 * Proposes a palette from a logo's pixels (RGBA bytes, as from a canvas): the most common saturated
 * colour becomes primary, the next distinct one accent; transparent, near-white and near-black
 * pixels are ignored. The result is then made to pass the contrast check.
 */
export function paletteFromPixels(rgba: ArrayLike<number>): Pick<Tokens, "light" | "dark"> | null {
  const buckets = new Map<number, { n: number; r: number; g: number; b: number }>();
  for (let i = 0; i + 3 < rgba.length; i += 4 * Math.max(1, Math.floor(rgba.length / 4 / 20000))) {
    const [r, g, b, a] = [rgba[i], rgba[i + 1], rgba[i + 2], rgba[i + 3]];
    if (a < 128) continue;
    const max = Math.max(r, g, b);
    const min = Math.min(r, g, b);
    if (max > 240 && min > 240) continue;
    if (max < 20) continue;
    if (max - min < 25) continue; // greys say little about a brand
    const key = ((r >> 4) << 8) | ((g >> 4) << 4) | (b >> 4);
    const e = buckets.get(key) ?? { n: 0, r: 0, g: 0, b: 0 };
    e.n++;
    e.r += r;
    e.g += g;
    e.b += b;
    buckets.set(key, e);
  }
  const ranked = [...buckets.values()].sort((x, y) => y.n - x.n).map((e) => rgbToHex([e.r / e.n, e.g / e.n, e.b / e.n]));
  if (!ranked.length) return null;
  const primary = ranked[0];
  const hue = (h: string) => hexToOklch(h)[2];
  const accent = ranked.find((c) => Math.abs(((hue(c) - hue(primary) + 3 * Math.PI) % (2 * Math.PI)) - Math.PI) > 0.5) ?? ranked[1] ?? primary;
  const [, , ph] = hexToOklch(primary);
  const light: Palette = {
    primary,
    onPrimary: contrastRatio(primary, "#ffffff") >= contrastRatio(primary, "#111111") ? "#ffffff" : "#111111",
    surface: oklchToHex(0.985, 0.01, ph),
    onSurface: oklchToHex(0.22, 0.02, ph),
    accent,
    muted: oklchToHex(0.45, 0.02, ph),
  };
  const dark: Palette = {
    primary: oklchToHex(Math.max(hexToOklch(primary)[0], 0.7), hexToOklch(primary)[1], ph),
    onPrimary: "#111111",
    surface: oklchToHex(0.17, 0.02, ph),
    onSurface: oklchToHex(0.96, 0.01, ph),
    accent: oklchToHex(Math.max(hexToOklch(accent)[0], 0.72), hexToOklch(accent)[1], hexToOklch(accent)[2]),
    muted: oklchToHex(0.75, 0.02, ph),
  };
  let out = { light, dark };
  for (let pass = 0; pass < 4; pass++) {
    for (const scheme of ["light", "dark"] as const) {
      for (const c of checkContrast({ mode: scheme, ...out })) if (!c.ok) out = acceptSuggestion(out, c);
    }
  }
  return out;
}
