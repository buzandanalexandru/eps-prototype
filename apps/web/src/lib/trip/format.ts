// Dates on the fan's ticket screens: in the event's time zone, formatted for the chosen language
// (Romanian: "1 iulie 2027"; English: "1 July 2027").
export function tag(locale: string): string {
  return locale === "ro" ? "ro-RO" : "en-GB";
}

export function fmtDate(iso: string | null | undefined, locale: string, tz = "Europe/Bucharest", time = false): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const opts: Intl.DateTimeFormatOptions = { day: "numeric", month: "long", year: "numeric", timeZone: tz };
  if (time) Object.assign(opts, { hour: "2-digit", minute: "2-digit", hour12: false });
  try {
    return new Intl.DateTimeFormat(tag(locale), opts).format(d);
  } catch {
    return new Intl.DateTimeFormat(tag(locale), { ...opts, timeZone: "UTC" }).format(d);
  }
}

/** A calendar date (YYYY-MM-DD) as words, without a time zone shift. */
export function fmtDay(day: string | null | undefined, locale: string): string {
  if (!day || !/^\d{4}-\d{2}-\d{2}$/.test(day)) return "";
  return new Intl.DateTimeFormat(tag(locale), { weekday: "long", day: "numeric", month: "long", timeZone: "UTC" }).format(new Date(`${day}T12:00:00Z`));
}

export function fmtTime(ms: number, locale: string): string {
  return new Intl.DateTimeFormat(tag(locale), { hour: "2-digit", minute: "2-digit", hour12: false }).format(new Date(ms));
}
