// Local dates and times in an event's time zone, with Intl only (no date library).
export const DEFAULT_TIMEZONE = "Europe/Bucharest";
export const TIME_PATTERN = /^([01]\d|2[0-3]):[0-5]\d$/;
export const DATE_PATTERN = /^\d{4}-\d{2}-\d{2}$/;

export function isTimeZone(tz: string): boolean {
  try {
    new Intl.DateTimeFormat("en", { timeZone: tz });
    return true;
  } catch {
    return false;
  }
}

function parts(d: Date, tz: string) {
  const f = new Intl.DateTimeFormat("en-CA", {
    timeZone: tz,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  });
  const o: Record<string, string> = {};
  for (const p of f.formatToParts(d)) o[p.type] = p.value;
  return o;
}

/** The local calendar date (YYYY-MM-DD) of an instant in a zone. */
export function localDate(d: Date, tz: string): string {
  const p = parts(d, tz);
  return `${p.year}-${p.month}-${p.day}`;
}

/** "YYYY-MM-DDTHH:MM" local wall time of an instant (for datetime-local inputs). */
export function toLocalInput(d: Date, tz: string): string {
  const p = parts(d, tz);
  return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}`;
}

function offsetMinutes(d: Date, tz: string): number {
  const p = parts(d, tz);
  const asUtc = Date.UTC(+p.year, +p.month - 1, +p.day, +p.hour, +p.minute, +p.second);
  return Math.round((asUtc - d.getTime()) / 60000);
}

/** A local wall time ("YYYY-MM-DDTHH:MM" or "YYYY-MM-DD HH:MM") in a zone → the instant. */
export function zonedToUtc(local: string, tz: string): Date | null {
  const m = /^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2})$/.exec(local.trim());
  if (!m) return null;
  const guess = Date.UTC(+m[1], +m[2] - 1, +m[3], +m[4], +m[5]);
  let t = guess - offsetMinutes(new Date(guess), tz) * 60000;
  t = guess - offsetMinutes(new Date(t), tz) * 60000; // second pass settles DST edges
  return new Date(t);
}

/** Minutes since midnight of "HH:MM". */
export function minutesOf(time: string): number {
  const [h, m] = time.split(":").map(Number);
  return h * 60 + m;
}

/** Adds days to a calendar date. */
export function addDays(date: string, days: number): string {
  const d = new Date(`${date}T00:00:00Z`);
  d.setUTCDate(d.getUTCDate() + days);
  return d.toISOString().slice(0, 10);
}

/** Calendar dates from a to b inclusive. */
export function dateRange(a: string, b: string): string[] {
  const out: string[] = [];
  for (let d = a; d <= b && out.length < 400; d = addDays(d, 1)) out.push(d);
  return out;
}

/** A night that closes at or before it opens closes the next morning but belongs to its start date. */
export function closesNextDay(opens: string, closes: string): boolean {
  return minutesOf(closes) <= minutesOf(opens);
}

/** Opening window of one event day as instants. */
export function dayWindow(day: string, opens: string, closes: string, tz: string): { opensAt: Date; closesAt: Date } {
  const opensAt = zonedToUtc(`${day}T${opens}`, tz)!;
  const closesAt = zonedToUtc(`${closesNextDay(opens, closes) ? addDays(day, 1) : day}T${closes}`, tz)!;
  return { opensAt, closesAt };
}

/** Sort key for a time within a night that opens at `opens`: 01:00 sorts after 23:00. */
export function nightOrder(time: string, opens: string): number {
  return (minutesOf(time) - minutesOf(opens) + 1440) % 1440;
}

/**
 * The local calendar date a slot starts on (spec map: slot instance ids). An event day's slots
 * that start before the first start of the grid (after midnight) belong to the next local date.
 */
export function slotStartDate(day: string, firstStart: string, slot: string): string {
  return minutesOf(slot) < minutesOf(firstStart) ? addDays(day, 1) : day;
}
