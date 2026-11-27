// Catalogue validation (design D1.1, D1.4): the one set of zod schemas that Studio's server actions,
// the seed and (step 11) the AI copilot use. Messages are error codes, translated by the screens.
import * as z from "zod";
import { ladderProblems, type PriceStep } from "../pricing/ladder";
import { DATE_PATTERN, isTimeZone, minutesOf, TIME_PATTERN } from "./time";

export const PRODUCT_TYPES = ["pass", "day_ticket", "flex_day", "add_on", "voucher", "table", "time_slot"] as const;
export type ProductType = (typeof PRODUCT_TYPES)[number];
/** Products that are tickets (admit someone), as opposed to add-ons. */
export const TICKET_TYPES: readonly ProductType[] = ["pass", "day_ticket", "flex_day", "voucher", "table", "time_slot"];
export const POOL_SCOPES = ["event", "day", "slot"] as const;
export type PoolScope = (typeof POOL_SCOPES)[number];

const text = (max: number) => z.string().trim().max(max, "too_long").default("");
/** Fan-facing text in each language; which ones are required is the readiness check's job. */
export const localized = (max = 200) => z.object({ ro: text(max), en: text(max) }).default({ ro: "", en: "" });
export type Localized = { ro: string; en: string };

const date = z.string().regex(DATE_PATTERN, "date_invalid");
const time = z.string().regex(TIME_PATTERN, "time_invalid");
const uuid = z.uuid("not_found");
const instant = z.coerce.date({ error: "date_invalid" });
/** An optional field where "" (an empty form input), undefined and null all mean "not set". */
export const blankToNull = <T extends z.ZodType>(s: T) =>
  z.preprocess((v) => (v === "" || v === undefined ? null : v), s.nullable()).optional().transform((v) => (v ?? null) as z.output<T> | null);
const optionalInstant = blankToNull(instant);
const positiveInt = (max: number) => z.coerce.number({ error: "number_invalid" }).int("number_invalid").min(1, "must_be_positive").max(max, "too_large");
const optionalPositiveInt = (max: number) => blankToNull(positiveInt(max));

// ---------------------------------------------------------------- venues

export const venueSchema = z.object({
  name: z.string().trim().min(1, "required").max(120, "too_long"),
  address: text(300),
  lat: blankToNull(z.coerce.number().min(-90, "invalid").max(90, "invalid")),
  lng: blankToNull(z.coerce.number().min(-180, "invalid").max(180, "invalid")),
  stages: z.array(z.string().trim().min(1).max(60)).max(30).default([]),
});
export type VenueInput = z.input<typeof venueSchema>;

// ---------------------------------------------------------------- events, days, line-up

export const eventSlug = z.string().trim().regex(/^[a-z0-9][a-z0-9-]{1,58}[a-z0-9]$/, "slug_characters");

export const eventSchema = z
  .object({
    brandId: uuid,
    venueId: blankToNull(uuid),
    slug: eventSlug,
    name: localized(120),
    description: localized(4000),
    timezone: z.string().default("Europe/Bucharest").refine(isTimeZone, "timezone_invalid"),
    startsAt: instant,
    endsAt: instant,
  })
  .refine((e) => e.endsAt.getTime() > e.startsAt.getTime(), { message: "end_before_start", path: ["endsAt"] });
export type EventInput = z.input<typeof eventSchema>;

export const daySchema = z.object({ day: date, opens: time, closes: time });
export type DayInput = z.infer<typeof daySchema>;
export const daysSchema = z.array(daySchema).max(400).refine((ds) => new Set(ds.map((d) => d.day)).size === ds.length, "day_duplicate");

export const lineupSchema = z.object({
  dayId: uuid,
  artist: z.string().trim().min(1, "required").max(120, "too_long"),
  stage: text(60),
  starts: time,
  sort: z.coerce.number({ error: "number_invalid" }).int("number_invalid").min(-10000, "too_large").max(10000, "too_large").default(0),
});
export type LineupInput = z.input<typeof lineupSchema>;

export const rulesSchema = z
  .object({
    minAge: optionalPositiveInt(99),
    consentUnderAge: optionalPositiveInt(99),
    maxPerAccount: optionalPositiveInt(1000),
  })
  .refine((r) => r.consentUnderAge === null || r.minAge === null || r.consentUnderAge >= r.minAge, {
    message: "consent_below_min_age",
    path: ["consentUnderAge"],
  });
export type RulesInput = z.input<typeof rulesSchema>;

// ---------------------------------------------------------------- products

const passConfig = z.object({ zone: z.string().trim().min(1, "required").max(60, "too_long"), days: z.array(date).min(1, "days_required") });
// day: the one day it admits; null means the buyer picks any event day when buying ("pick a day").
const dayTicketConfig = z.object({ day: z.union([date, z.null()]).default(null) });
const flexDayConfig = z.object({ days: z.array(date).min(1, "days_required") });
const addOnConfig = z.object({
  requires: z.union([z.literal("none"), z.literal("any_ticket"), z.array(uuid).min(1, "required")]).default("any_ticket"),
});
const voucherConfig = z.object({ spendValueMinor: z.coerce.number({ error: "spend_required" }).int("number_invalid").min(1, "spend_required") });
const tableConfig = z.object({
  category: z.string().trim().min(1, "required").max(30, "too_long"),
  seats: positiveInt(50),
});
const timeSlotConfig = z
  .object({
    slotMinutes: positiveInt(240),
    firstStart: time,
    lastStart: time,
    maxConsecutiveSlots: positiveInt(48),
  })
  .refine((c) => slotSpan(c.firstStart, c.lastStart) % c.slotMinutes === 0, { message: "slot_span_not_divisible", path: ["slotMinutes"] });

/** Minutes from the first to the last start (0 when they are equal: one slot a day). */
export function slotSpan(firstStart: string, lastStart: string): number {
  const a = minutesOf(firstStart);
  const b = minutesOf(lastStart);
  return b >= a ? b - a : b + 1440 - a;
}

export const productConfigSchemas = {
  pass: passConfig,
  day_ticket: dayTicketConfig,
  flex_day: flexDayConfig,
  add_on: addOnConfig,
  voucher: voucherConfig,
  table: tableConfig,
  time_slot: timeSlotConfig,
} as const;

const productCommon = {
  name: localized(120),
  perks: z.array(localized(120)).max(12).default([]),
  named: z.coerce.boolean().default(false),
  salesFrom: optionalInstant,
  salesUntil: optionalInstant,
  visible: z.coerce.boolean().default(true),
  maxPerOrder: positiveInt(100).default(10),
  // Ticket lifecycle settings (step 7 uses them; stored now).
  personaliseDeadline: optionalInstant,
  checkinDeadline: optionalInstant,
  lateCheckinFeeMinor: blankToNull(z.coerce.number({ error: "price_invalid" }).int("price_invalid").min(0, "price_invalid").max(100_000_000, "too_large")),
  sort: z.coerce.number({ error: "number_invalid" }).int("number_invalid").min(-10000, "too_large").max(10000, "too_large").default(0),
};

export const productSchema = z
  .discriminatedUnion("type", [
    z.object({ type: z.literal("pass"), config: passConfig, ...productCommon }),
    z.object({ type: z.literal("day_ticket"), config: dayTicketConfig, ...productCommon }),
    z.object({ type: z.literal("flex_day"), config: flexDayConfig, ...productCommon }),
    z.object({ type: z.literal("add_on"), config: addOnConfig, ...productCommon }),
    z.object({ type: z.literal("voucher"), config: voucherConfig, ...productCommon }),
    z.object({ type: z.literal("table"), config: tableConfig, ...productCommon }),
    z.object({ type: z.literal("time_slot"), config: timeSlotConfig, ...productCommon }),
  ])
  .refine((p) => p.salesFrom === null || p.salesUntil === null || p.salesUntil.getTime() > p.salesFrom.getTime(), {
    message: "sales_end_before_start",
    path: ["salesUntil"],
  });
export type ProductInput = z.input<typeof productSchema>;
export type Product = z.output<typeof productSchema>;

// ---------------------------------------------------------------- pools

export const poolSchema = z.object({
  name: z.string().trim().min(1, "required").max(120, "too_long"),
  capacity: z.coerce.number({ error: "number_invalid" }).int("number_invalid").min(0, "must_not_be_negative").max(10_000_000, "too_large"),
  scope: z.enum(POOL_SCOPES).default("event"),
});
export type PoolInput = z.input<typeof poolSchema>;

// ---------------------------------------------------------------- price ladders

const stepInput = z.object({
  fromAt: optionalInstant,
  afterSold: optionalPositiveInt(100_000_000),
  priceMinor: z.coerce.number({ error: "price_invalid" }).int("price_invalid").min(0, "price_invalid").max(100_000_000, "too_large"),
});

export const ladderSchema = z
  .object({ mode: z.enum(["by_date", "by_sales"]), steps: z.array(stepInput).min(1, "ladder_empty").max(30, "too_large") })
  .superRefine((l, ctx) => {
    // A step carrying the other mode's threshold mixes the two kinds: refused.
    const other = l.mode === "by_date" ? "afterSold" : "fromAt";
    if (l.steps.some((s) => s[other] !== null && s[other] !== undefined)) {
      ctx.addIssue({ code: "custom", message: "ladder_mixed", path: ["steps"] });
      return;
    }
    for (const p of ladderProblems(toPriceSteps(l))) ctx.addIssue({ code: "custom", message: p, path: ["steps"] });
  });
export type LadderInput = z.input<typeof ladderSchema>;

export function toPriceSteps(l: { mode: "by_date" | "by_sales"; steps: { fromAt?: Date | null; afterSold?: number | null; priceMinor: number }[] }): PriceStep[] {
  return l.steps.map((s) =>
    l.mode === "by_date" ? { mode: "by_date", fromAt: s.fromAt ?? null, priceMinor: s.priceMinor } : { mode: "by_sales", afterSold: s.afterSold ?? null, priceMinor: s.priceMinor },
  );
}

// ---------------------------------------------------------------- instalments

/** A deposit is a fixed amount (depositMinor) or a share of the all-in price (depositBps), never both. */
export const instalmentSchema = z
  .object({
    depositMinor: blankToNull(z.coerce.number({ error: "price_invalid" }).int("price_invalid").min(0, "price_invalid").max(100_000_000, "too_large")),
    depositBps: blankToNull(z.coerce.number({ error: "number_invalid" }).int("number_invalid").min(0, "number_invalid").max(9999, "too_large")),
    monthlyCount: positiveInt(24),
    dayOfMonth: z.coerce.number().int().min(1, "day_of_month_invalid").max(28, "day_of_month_invalid").default(1),
  })
  .refine((p) => (p.depositMinor === null) !== (p.depositBps === null), { message: "deposit_one_kind", path: ["depositMinor"] });
export type InstalmentInput = z.input<typeof instalmentSchema>;

// ---------------------------------------------------------------- helpers

/** zod issues → { "config.zone": "required", ... } (first message per path). */
export function fieldErrors(err: z.ZodError): Record<string, string> {
  const out: Record<string, string> = {};
  for (const i of err.issues) {
    const k = i.path.join(".") || "_";
    if (!out[k]) out[k] = /^[a-z_]+$/.test(i.message) ? i.message : "invalid";
  }
  return out;
}
