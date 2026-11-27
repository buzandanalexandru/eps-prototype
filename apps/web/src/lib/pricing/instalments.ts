// Instalment schedules (design D1.4, C9): a deposit now and equal monthly payments, any rounding
// remainder on the last one. The last payment falls at least 7 days before the event starts.
// Dates are calendar dates (YYYY-MM-DD, UTC arithmetic); amounts are integer minor units.
import { isMinor } from "./money";

export const LAST_PAYMENT_DAYS_BEFORE_EVENT = 7;
export const MAX_MONTHLY_PAYMENTS = 24;

export type InstalmentInput = {
  allInMinor: number;
  depositMinor: number;
  monthlyCount: number;
  /** Day of the month each monthly payment is due (1 to 28). */
  dayOfMonth: number;
  /** The purchase (or, in Studio, the first sales day): monthly payments start the month after. */
  from: Date;
  eventStartsAt: Date;
};

export type InstalmentPayment = { n: number; dueOn: string; amountMinor: number };

export type InstalmentSchedule =
  | { ok: true; depositMinor: number; payments: InstalmentPayment[]; totalMinor: number }
  | { ok: false; code: "deposit_too_large" }
  | { ok: false; code: "invalid_plan" }
  | { ok: false; code: "past_event"; maxCount: number };

function iso(d: Date): string {
  return d.toISOString().slice(0, 10);
}

/** The k-th monthly due date (k ≥ 1): dayOfMonth in the k-th month after `from`'s month. */
export function dueDate(from: Date, k: number, dayOfMonth: number): Date {
  return new Date(Date.UTC(from.getUTCFullYear(), from.getUTCMonth() + k, dayOfMonth));
}

/** The last day a monthly payment may fall on. */
export function lastAllowedDay(eventStartsAt: Date): Date {
  const d = new Date(Date.UTC(eventStartsAt.getUTCFullYear(), eventStartsAt.getUTCMonth(), eventStartsAt.getUTCDate()));
  d.setUTCDate(d.getUTCDate() - LAST_PAYMENT_DAYS_BEFORE_EVENT);
  return d;
}

/** The most monthly payments that still end at least 7 days before the event (0 if none fit). */
export function maxMonthlyPayments(from: Date, dayOfMonth: number, eventStartsAt: Date): number {
  const limit = lastAllowedDay(eventStartsAt).getTime();
  let n = 0;
  while (n < MAX_MONTHLY_PAYMENTS && dueDate(from, n + 1, dayOfMonth).getTime() <= limit) n++;
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
  const max = maxMonthlyPayments(p.from, p.dayOfMonth, p.eventStartsAt);
  if (p.monthlyCount > max) return { ok: false, code: "past_event", maxCount: max };
  const rest = p.allInMinor - p.depositMinor;
  const each = Math.floor(rest / p.monthlyCount);
  const payments: InstalmentPayment[] = [];
  for (let k = 1; k <= p.monthlyCount; k++) {
    const amount = k === p.monthlyCount ? rest - each * (p.monthlyCount - 1) : each;
    payments.push({ n: k, dueOn: iso(dueDate(p.from, k, p.dayOfMonth)), amountMinor: amount });
  }
  return { ok: true, depositMinor: p.depositMinor, payments, totalMinor: p.allInMinor };
}
