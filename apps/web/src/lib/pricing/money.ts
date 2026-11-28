// Money in integer minor units (bani, cents) with a currency (design D1.3). No floats anywhere:
// parsing and formatting go through strings and integers only.
export type Currency = "RON" | "EUR";
export const CURRENCIES: readonly Currency[] = ["RON", "EUR"];

export function isMinor(n: unknown): n is number {
  return typeof n === "number" && Number.isSafeInteger(n);
}

/**
 * "539", "539.5", "539.50" or "539,50" → 53950. Returns null for anything else: signs, spaces inside,
 * thousands separators, more than 2 decimals, or more than 1,000,000.00.
 */
export const MAX_MAJOR = 1_000_000;
export function parseMajor(input: string): number | null {
  const s = input.trim().replace(",", ".");
  const m = /^(0|[1-9]\d{0,6})(?:\.(\d{1,2}))?$/.exec(s);
  if (!m) return null;
  const v = Number(m[1]) * 100 + Number((m[2] ?? "").padEnd(2, "0"));
  return v <= MAX_MAJOR * 100 ? v : null;
}

/** 55248 → "552.48" (no currency, dot separator: the stable form used in tests and inputs). */
export function toMajorString(minor: number): string {
  if (!isMinor(minor)) throw new Error("toMajorString: not an integer amount");
  const neg = minor < 0;
  const a = Math.abs(minor);
  return `${neg ? "-" : ""}${Math.floor(a / 100)}.${String(a % 100).padStart(2, "0")}`;
}

const MONEY_LOCALES: Record<string, string> = { ro: "ro-RO", en: "en-GB" };
const moneyFormats = new Map<string, Intl.NumberFormat>();

/**
 * Display form for a locale through Intl.NumberFormat with the currency: "552,48 RON" in Romanian,
 * "RON 552.48" in English (a no-break space between amount and currency). Amounts are integer minor
 * units; the division is exact for display at two decimals.
 */
export function formatMoney(minor: number, currency: Currency, locale: string): string {
  if (!isMinor(minor)) throw new Error("formatMoney: not an integer amount");
  const tag = MONEY_LOCALES[locale] ?? "en-GB";
  const key = `${tag}|${currency}`;
  let f = moneyFormats.get(key);
  if (!f) {
    f = new Intl.NumberFormat(tag, { style: "currency", currency, currencyDisplay: "code", minimumFractionDigits: 2, maximumFractionDigits: 2 });
    moneyFormats.set(key, f);
  }
  return f.format(minor / 100);
}

/** round(n / d) with halves rounded up, for non-negative integers. */
export function divRoundHalfUp(n: number, d: number): number {
  if (!isMinor(n) || !isMinor(d) || n < 0 || d <= 0) throw new Error("divRoundHalfUp: non-negative integers only");
  return Math.floor((2 * n + d) / (2 * d));
}
