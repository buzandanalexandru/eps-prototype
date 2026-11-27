// Money in integer minor units (bani, cents) with a currency (design D1.3). No floats anywhere:
// parsing and formatting go through strings and integers only.
export type Currency = "RON" | "EUR";
export const CURRENCIES: readonly Currency[] = ["RON", "EUR"];

export function isMinor(n: unknown): n is number {
  return typeof n === "number" && Number.isSafeInteger(n);
}

/** "539", "539.5", "539.50" or "539,50" → 53950. Returns null for anything else (or more than 2 decimals). */
export function parseMajor(input: string): number | null {
  const s = input.trim().replace(",", ".");
  const m = /^(\d{1,9})(?:\.(\d{1,2}))?$/.exec(s);
  if (!m) return null;
  return Number(m[1]) * 100 + Number((m[2] ?? "").padEnd(2, "0"));
}

/** 55248 → "552.48" (no currency, dot separator: the stable form used in tests and inputs). */
export function toMajorString(minor: number): string {
  if (!isMinor(minor)) throw new Error("toMajorString: not an integer amount");
  const neg = minor < 0;
  const a = Math.abs(minor);
  return `${neg ? "-" : ""}${Math.floor(a / 100)}.${String(a % 100).padStart(2, "0")}`;
}

/** Display form for a locale: "552,48 RON" in Romanian, "552.48 RON" in English. */
export function formatMoney(minor: number, currency: Currency, locale: string): string {
  const s = toMajorString(minor);
  const [int, dec] = s.replace("-", "").split(".");
  const sep = locale === "ro" ? "." : ",";
  const grouped = int.replace(/\B(?=(\d{3})+(?!\d))/g, sep);
  return `${minor < 0 ? "−" : ""}${grouped}${locale === "ro" ? "," : "."}${dec} ${currency}`;
}

/** round(n / d) with halves rounded up, for non-negative integers. */
export function divRoundHalfUp(n: number, d: number): number {
  if (!isMinor(n) || !isMinor(d) || n < 0 || d <= 0) throw new Error("divRoundHalfUp: non-negative integers only");
  return Math.floor((2 * n + d) / (2 * d));
}
