// Instalment schedules (design D1.4, C9): a deposit now and equal monthly payments, any rounding
// remainder on the last one. Dates are calendar dates in the event's time zone: payment k falls on
// `dayOfMonth` of the k-th month after the purchase's local month, and the last payment falls at
// least 7 days before the event's local start date. A plan's monthly count is a maximum: each buyer
// gets as many payments as still fit from their purchase date (capped per buyer).
import { addDays, DEFAULT_TIMEZONE, localDate } from "../catalogue/time";
import { divRoundHalfUp, isMinor } from "./money";

export const LAST_PAYMENT_DAYS_BEFORE_EVENT = 7;
export const MAX_MONTHLY_PAYMENTS = 24;

export type InstalmentInput = {
  allInMinor: number;
  depositMinor: number;
  /** The most monthly payments (the plan's setting). */
  monthlyCount: number;
  /** Day of the month each monthly payment is due (1 to 28). */
  dayOfMonth: number;
  /** The purchase: monthly payments start the month after its local month. */
  from: Date;
  eventStartsAt: Date;
  /** The event's time zone (default Europe/Bucharest). */
  timeZone?: string;
  /** true: give the buyer as many payments as still fit (up to monthlyCount) instead of refusing. */
  capToFit?: boolean;
};

export type InstalmentPayment = { n: number; dueOn: string; amountMinor: number };

export type InstalmentSchedule =
  | { ok: true; depositMinor: number; payments: InstalmentPayment[]; totalMinor: number }
  | { ok: false; code: "deposit_too_large" }
  | { ok: false; code: "invalid_plan" }
  | { ok: false; code: "past_event"; maxCount: number };

/** The deposit for an all-in price: a fixed amount, or a share in basis points rounded half up. */
export function depositFor(plan: { depositMinor?: number | null; depositBps?: number | null }, allInMinor: number): number {
  if (plan.depositMinor !== null && plan.depositMinor !== undefined) return plan.depositMinor;
  return divRoundHalfUp(allInMinor * (plan.depositBps ?? 0), 10_000);
}

/** The k-th monthly due date (k ≥ 1) as YYYY-MM-DD, from a local purchase date. */
export function dueDate(fromLocal: string, k: number, dayOfMonth: number): string {
  const y = Number(fromLocal.slice(0, 4));
  const m = Number(fromLocal.slice(5, 7)) - 1 + k;
  return new Date(Date.UTC(y, m, dayOfMonth)).toISOString().slice(0, 10);
}

/** The last local date a monthly payment may fall on. */
export function lastAllowedDay(eventStartsAt: Date, timeZone = DEFAULT_TIMEZONE): string {
  return addDays(localDate(eventStartsAt, timeZone), -LAST_PAYMENT_DAYS_BEFORE_EVENT);
}

/** The most monthly payments a buyer purchasing at `from` can get (0 if none fit). */
export function maxMonthlyPayments(from: Date, dayOfMonth: number, eventStartsAt: Date, timeZone = DEFAULT_TIMEZONE): number {
  const fromLocal = localDate(from, timeZone);
  const limit = lastAllowedDay(eventStartsAt, timeZone);
  let n = 0;
  while (n < MAX_MONTHLY_PAYMENTS && dueDate(fromLocal, n + 1, dayOfMonth) <= limit) n++;
  return n;
}

export function instalmentSchedule(p: InstalmentInput): InstalmentSchedule {
  if (
    !isMinor(p.allInMinor) || !isMinor(p.depositMinor) || p.depositMinor < 0 ||
    !Number.isInteger(p.monthlyCount) || p.monthlyCount < 1 || p.monthlyCount > MAX_MONTHLY_PAYMENTS ||
    !Number.isInteger(p.dayOfMonth) || p.dayOfMonth < 1 || p.dayOfMonth > 28
  ) {
    return { ok: false, code: "invalid_plan" };
  }
  if (p.depositMinor >= p.allInMinor) return { ok: false, code: "deposit_too_large" };
  const tz = p.timeZone ?? DEFAULT_TIMEZONE;
  const max = maxMonthlyPayments(p.from, p.dayOfMonth, p.eventStartsAt, tz);
  const count = p.capToFit ? Math.min(p.monthlyCount, max) : p.monthlyCount;
  if (count < 1 || count > max) return { ok: false, code: "past_event", maxCount: max };
  const fromLocal = localDate(p.from, tz);
  const rest = p.allInMinor - p.depositMinor;
  const each = Math.floor(rest / count);
  const payments: InstalmentPayment[] = [];
  for (let k = 1; k <= count; k++) {
    payments.push({ n: k, dueOn: dueDate(fromLocal, k, p.dayOfMonth), amountMinor: k === count ? rest - each * (count - 1) : each });
  }
  return { ok: true, depositMinor: p.depositMinor, payments, totalMinor: p.allInMinor };
}
