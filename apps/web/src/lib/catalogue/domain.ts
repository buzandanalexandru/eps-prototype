// Catalogue operations (design D1.4). Every function runs on a client inside withTx, so row-level
// security limits it to the transaction's organiser and composite foreign keys refuse links to
// another organiser's rows (or, for pool links and line-up days, another event's rows).
// Edits are UPDATE ... WHERE id = $1 (no row: not found). Inserts use server-generated ids; the
// optional fixed id of create* functions is for the seed's stable ids only and is never taken from
// a request.
import type { ClientBase } from "pg";
import * as z from "zod";
import { DomainError } from "../errors";
import { CURRENT_ORG, getFeePolicy, getPlatformFee } from "../organisers/domain";
import { activePriceStep, type PriceStep } from "../pricing/ladder";
import { effectivePolicy, feeBreakdown, type FeePolicy } from "../pricing/fees";
import { depositFor, instalmentSchedule, maxMonthlyPayments } from "../pricing/instalments";
import type { Currency } from "../pricing/money";
import {
  daysSchema,
  eventSchema,
  instalmentSchema,
  ladderSchema,
  lineupSchema,
  poolSchema,
  productSchema,
  rulesSchema,
  venueSchema,
  type DayInput,
  type EventInput,
  type InstalmentInput,
  type LadderInput,
  type LineupInput,
  type Localized,
  type PoolInput,
  type PoolScope,
  type Product,
  type ProductInput,
  type ProductType,
  type RulesInput,
  type VenueInput,
} from "./schema";
import { localDate } from "./time";

export const isUuid = (v: string) => z.uuid().safeParse(v).success;

/** Refuses a malformed id before it reaches SQL (reported as not found). */
export function requireUuid(v: string, field?: string): string {
  if (!isUuid(v)) throw new DomainError("not_found", field);
  return v;
}

function need<T>(row: T | undefined | null): T {
  if (!row) throw new DomainError("not_found");
  return row;
}

export type Page = { limit?: number; offset?: number };
export const PAGE_SIZE = 50;
const page = (p: Page = {}) => [Math.min(Math.max(p.limit ?? PAGE_SIZE, 1), 500), Math.max(p.offset ?? 0, 0)];

// ---------------------------------------------------------------- venues

export type VenueRow = { id: string; name: string; address: string; lat: number | null; lng: number | null; stages: string[] };
const VENUE_COLS = "id, name, address, lat, lng, stages";

export async function createVenue(c: ClientBase, raw: VenueInput, fixedId?: string): Promise<VenueRow> {
  const v = venueSchema.parse(raw);
  const r = await c.query<VenueRow>(
    `insert into control.venues (id, organiser_id, name, address, lat, lng, stages)
     values (coalesce($1, gen_random_uuid()), ${CURRENT_ORG}, $2, $3, $4, $5, $6) returning ${VENUE_COLS}`,
    [fixedId ?? null, v.name, v.address, v.lat, v.lng, v.stages],
  );
  return need(r.rows[0]);
}

export async function updateVenue(c: ClientBase, id: string, raw: VenueInput): Promise<VenueRow> {
  requireUuid(id);
  const v = venueSchema.parse(raw);
  const r = await c.query<VenueRow>(
    `update control.venues set name = $2, address = $3, lat = $4, lng = $5, stages = $6 where id = $1 returning ${VENUE_COLS}`,
    [id, v.name, v.address, v.lat, v.lng, v.stages],
  );
  return need(r.rows[0]);
}

/** Studio's save: an edit when an id is given (UPDATE only), else a new venue with a server id. */
export function saveVenue(c: ClientBase, raw: VenueInput, id?: string): Promise<VenueRow> {
  return id ? updateVenue(c, id, raw) : createVenue(c, raw);
}

export async function listVenues(c: ClientBase, p?: Page): Promise<(VenueRow & { events: number })[]> {
  const [limit, offset] = page(p);
  return (
    await c.query(
      `select v.id, v.name, v.address, v.lat, v.lng, v.stages, (select count(*)::int from control.events e where e.venue_id = v.id) as events
       from control.venues v order by v.name, v.id limit $1 offset $2`,
      [limit, offset],
    )
  ).rows;
}

// ---------------------------------------------------------------- events

export type EventRow = {
  id: string;
  brand_id: string;
  venue_id: string | null;
  slug: string;
  name: Localized;
  description: Localized;
  timezone: string;
  starts_at: Date;
  ends_at: Date;
  status: "draft" | "published" | "archived";
  edition_of: string | null;
  fee_policy: FeePolicy | null;
  min_age: number | null;
  consent_under_age: number | null;
  max_per_account: number | null;
  hold_minutes: number;
};

const EVENT_COLS =
  "id, brand_id, venue_id, slug, name, description, timezone, starts_at, ends_at, status, edition_of, fee_policy, min_age, consent_under_age, max_per_account, hold_minutes";

export async function createEvent(c: ClientBase, raw: EventInput, fixedId?: string): Promise<EventRow> {
  const e = eventSchema.parse(raw);
  const r = await c.query<EventRow>(
    `insert into control.events (id, organiser_id, brand_id, venue_id, slug, name, description, timezone, starts_at, ends_at)
     values (coalesce($1, gen_random_uuid()), ${CURRENT_ORG}, $2, $3, $4, $5, $6, $7, $8, $9) returning ${EVENT_COLS}`,
    [fixedId ?? null, e.brandId, e.venueId, e.slug, e.name, e.description, e.timezone, e.startsAt, e.endsAt],
  );
  return need(r.rows[0]);
}

export async function updateEvent(c: ClientBase, id: string, raw: EventInput): Promise<EventRow> {
  const e = eventSchema.parse(raw);
  const cur = need(await getEvent(c, id));
  if (cur.brand_id !== e.brandId) {
    // Prices are in the brand's currency: an event moves only to a brand with the same currency.
    const r = await c.query<{ currency: string }>("select currency from control.brands where id = any($1::uuid[])", [[cur.brand_id, e.brandId]]);
    if (r.rows.length !== 2) throw new DomainError("not_found", "brandId");
    if (r.rows[0].currency !== r.rows[1].currency) throw new DomainError("currency_mismatch", "brandId");
  }
  // Moving dates must keep existing days inside the event.
  const days = await c.query<{ day: string }>("select day::text from control.event_days where event_id = $1", [id]);
  const lo = localDate(e.startsAt, e.timezone);
  const hi = localDate(e.endsAt, e.timezone);
  if (days.rows.some((d) => d.day < lo || d.day > hi)) throw new DomainError("day_outside_event", "endsAt");
  const r = await c.query<EventRow>(
    `update control.events set brand_id = $2, venue_id = $3, slug = $4, name = $5, description = $6, timezone = $7,
       starts_at = $8, ends_at = $9, updated_at = now() where id = $1 returning ${EVENT_COLS}`,
    [id, e.brandId, e.venueId, e.slug, e.name, e.description, e.timezone, e.startsAt, e.endsAt],
  );
  return need(r.rows[0]);
}

export function saveEvent(c: ClientBase, raw: EventInput, id?: string): Promise<EventRow> {
  return id ? updateEvent(c, id, raw) : createEvent(c, raw);
}

export async function getEvent(c: ClientBase, id: string): Promise<EventRow | null> {
  if (!isUuid(id)) return null;
  return (await c.query<EventRow>(`select ${EVENT_COLS} from control.events where id = $1`, [id])).rows[0] ?? null;
}

export async function listEvents(c: ClientBase, p?: Page): Promise<(EventRow & { brand_name: string; products: number })[]> {
  const [limit, offset] = page(p);
  return (
    await c.query(
      `select ${EVENT_COLS.split(", ").map((x) => "e." + x).join(", ")}, b.name as brand_name,
        (select count(*)::int from control.products p where p.event_id = e.id) as products
       from control.events e join control.brands b on b.id = e.brand_id order by e.starts_at, e.slug limit $1 offset $2`,
      [limit, offset],
    )
  ).rows;
}

export async function deleteEvent(c: ClientBase, id: string): Promise<void> {
  requireUuid(id);
  const r = await c.query("delete from control.events where id = $1 and status = 'draft'", [id]);
  if (!r.rowCount) throw new DomainError("not_found");
}

export async function setRules(c: ClientBase, eventId: string, raw: RulesInput): Promise<void> {
  requireUuid(eventId);
  const r = rulesSchema.parse(raw);
  const res = await c.query(
    "update control.events set min_age = $2, consent_under_age = $3, max_per_account = $4, updated_at = now() where id = $1",
    [eventId, r.minAge, r.consentUnderAge, r.maxPerAccount],
  );
  if (!res.rowCount) throw new DomainError("not_found");
}

export type DayRow = DayInput & { id: string };

/** Replaces the event's days (kept by date, so line-up slots of kept days survive). */
export async function setDays(c: ClientBase, eventId: string, raw: DayInput[], ids?: Record<string, string>): Promise<DayRow[]> {
  const days = daysSchema.parse(raw);
  const ev = need(await getEvent(c, eventId));
  const lo = localDate(ev.starts_at, ev.timezone);
  const hi = localDate(ev.ends_at, ev.timezone);
  const bad = days.findIndex((d) => d.day < lo || d.day > hi);
  if (bad >= 0) throw new DomainError("day_outside_event", `days.${bad}.day`);
  await c.query("delete from control.event_days where event_id = $1 and not (day::text = any($2))", [eventId, days.map((d) => d.day)]);
  if (days.length) {
    // One statement for all days (a season has 74).
    await c.query(
      `insert into control.event_days (id, organiser_id, event_id, day, opens, closes)
       select coalesce(x.id, gen_random_uuid()), ${CURRENT_ORG}, $1, x.day, x.opens, x.closes
       from unnest($2::uuid[], $3::date[], $4::time[], $5::time[]) as x(id, day, opens, closes)
       on conflict (event_id, day) do update set opens = excluded.opens, closes = excluded.closes`,
      [eventId, days.map((d) => ids?.[d.day] ?? null), days.map((d) => d.day), days.map((d) => d.opens), days.map((d) => d.closes)],
    );
  }
  return listDays(c, eventId);
}

export async function listDays(c: ClientBase, eventId: string): Promise<DayRow[]> {
  return (await listDaysOf(c, [eventId])).map(({ event_id: _e, ...d }) => d); // eslint-disable-line @typescript-eslint/no-unused-vars
}

async function listDaysOf(c: ClientBase, eventIds: string[]): Promise<(DayRow & { event_id: string })[]> {
  return (
    await c.query(
      `select id, event_id, day::text as day, to_char(opens, 'HH24:MI') as opens, to_char(closes, 'HH24:MI') as closes
       from control.event_days where event_id = any($1::uuid[]) order by day`,
      [eventIds],
    )
  ).rows;
}

export type LineupRow = { id: string; day_id: string; artist: string; stage: string; starts: string; sort: number; sample?: boolean };

export async function addLineupSlot(c: ClientBase, eventId: string, raw: LineupInput, fixedId?: string): Promise<LineupRow> {
  const l = lineupSchema.parse(raw);
  const r = await c.query<LineupRow>(
    `insert into control.lineup_slots (id, organiser_id, event_id, day_id, artist, stage, starts, sort)
     select coalesce($1, gen_random_uuid()), ${CURRENT_ORG}, $2, d.id, $4, $5, $6, $7 from control.event_days d where d.id = $3 and d.event_id = $2
     returning id, day_id, artist, stage, to_char(starts, 'HH24:MI') as starts, sort`,
    [fixedId ?? null, eventId, l.dayId, l.artist, l.stage, l.starts, l.sort],
  );
  return need(r.rows[0]);
}

export async function deleteLineupSlot(c: ClientBase, eventId: string, id: string): Promise<void> {
  requireUuid(eventId);
  requireUuid(id);
  await c.query("delete from control.lineup_slots where id = $1 and event_id = $2", [id, eventId]);
}

export async function listLineup(c: ClientBase, eventId: string): Promise<LineupRow[]> {
  return (await listLineupOf(c, [eventId])).map(({ event_id: _e, ...l }) => l); // eslint-disable-line @typescript-eslint/no-unused-vars
}

async function listLineupOf(c: ClientBase, eventIds: string[]): Promise<(LineupRow & { event_id: string })[]> {
  return (
    await c.query(
      `select id, event_id, day_id, artist, stage, to_char(starts, 'HH24:MI') as starts, sort, sample from control.lineup_slots where event_id = any($1::uuid[])`,
      [eventIds],
    )
  ).rows;
}

// ---------------------------------------------------------------- products

export type ProductRow = {
  id: string;
  event_id: string;
  type: ProductType;
  name: Localized;
  perks: Localized[];
  config: Record<string, unknown>;
  named: boolean;
  sales_from: Date | null;
  sales_until: Date | null;
  visible: boolean;
  max_per_order: number;
  sort: number;
  personalise_deadline: Date | null;
  checkin_deadline: Date | null;
  late_checkin_fee_minor: number | null;
};
const PRODUCT_COLS =
  "id, event_id, type, name, perks, config, named, sales_from, sales_until, visible, max_per_order, sort, personalise_deadline, checkin_deadline, late_checkin_fee_minor::int as late_checkin_fee_minor";

/** Checks the type settings against the event: days must be event days, required products must exist. */
async function checkAgainstEvent(c: ClientBase, eventId: string, p: Product, selfId?: string) {
  const days = new Set((await listDays(c, eventId)).map((d) => d.day));
  const daysOf = p.type === "pass" || p.type === "flex_day" ? p.config.days : p.type === "day_ticket" && p.config.day ? [p.config.day] : [];
  const bad = daysOf.findIndex((d) => !days.has(d));
  if (bad >= 0) throw new DomainError("day_not_in_event", p.type === "day_ticket" ? "config.day" : `config.days`);
  if (p.type === "add_on" && Array.isArray(p.config.requires)) {
    const ids = p.config.requires;
    if (selfId && ids.includes(selfId)) throw new DomainError("invalid", "config.requires");
    const r = await c.query("select count(*)::int as n from control.products where event_id = $1 and id = any($2::uuid[])", [eventId, ids]);
    if (r.rows[0].n !== new Set(ids).size) throw new DomainError("not_found", "config.requires");
  }
}

function productParams(p: Product) {
  return [
    p.type,
    p.name,
    JSON.stringify(p.perks),
    p.config,
    p.named,
    p.salesFrom,
    p.salesUntil,
    p.visible,
    p.maxPerOrder,
    p.sort,
    p.personaliseDeadline,
    p.checkinDeadline,
    p.lateCheckinFeeMinor,
  ];
}

export async function createProduct(c: ClientBase, eventId: string, raw: ProductInput, fixedId?: string): Promise<ProductRow> {
  const p = productSchema.parse(raw);
  need(await getEvent(c, eventId));
  await checkAgainstEvent(c, eventId, p);
  const r = await c.query<ProductRow>(
    `insert into control.products (id, organiser_id, event_id, type, name, perks, config, named, sales_from, sales_until, visible, max_per_order, sort,
       personalise_deadline, checkin_deadline, late_checkin_fee_minor)
     values (coalesce($1, gen_random_uuid()), ${CURRENT_ORG}, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
     returning ${PRODUCT_COLS}`,
    [fixedId ?? null, eventId, ...productParams(p)],
  );
  return need(r.rows[0]);
}

export async function updateProduct(c: ClientBase, eventId: string, id: string, raw: ProductInput): Promise<ProductRow> {
  requireUuid(id);
  const p = productSchema.parse(raw);
  need(await getEvent(c, eventId));
  await checkAgainstEvent(c, eventId, p, id);
  const r = await c.query<ProductRow>(
    `update control.products set type = $3, name = $4, perks = $5, config = $6, named = $7, sales_from = $8, sales_until = $9,
       visible = $10, max_per_order = $11, sort = $12, personalise_deadline = $13, checkin_deadline = $14, late_checkin_fee_minor = $15
     where id = $1 and event_id = $2 returning ${PRODUCT_COLS}`,
    [id, eventId, ...productParams(p)],
  );
  return need(r.rows[0]);
}

export function saveProduct(c: ClientBase, eventId: string, raw: ProductInput, id?: string): Promise<ProductRow> {
  return id ? updateProduct(c, eventId, id, raw) : createProduct(c, eventId, raw);
}

export async function getProduct(c: ClientBase, id: string): Promise<ProductRow | null> {
  if (!isUuid(id)) return null;
  return (await c.query<ProductRow>(`select ${PRODUCT_COLS} from control.products where id = $1`, [id])).rows[0] ?? null;
}

export async function listProducts(c: ClientBase, eventId: string): Promise<ProductRow[]> {
  return listProductsOf(c, [eventId]);
}

async function listProductsOf(c: ClientBase, eventIds: string[]): Promise<ProductRow[]> {
  return (await c.query<ProductRow>(`select ${PRODUCT_COLS} from control.products where event_id = any($1::uuid[]) order by sort, created_at`, [eventIds])).rows;
}

export async function deleteProduct(c: ClientBase, eventId: string, id: string): Promise<void> {
  requireUuid(eventId);
  requireUuid(id);
  await c.query("delete from control.products where id = $1 and event_id = $2", [id, eventId]);
}

// ---------------------------------------------------------------- pools

export type PoolRow = { id: string; event_id: string; name: string; capacity: number; scope: PoolScope };

export async function createPool(c: ClientBase, eventId: string, raw: PoolInput, fixedId?: string): Promise<PoolRow> {
  const p = poolSchema.parse(raw);
  need(await getEvent(c, eventId));
  const r = await c.query<PoolRow>(
    `insert into control.capacity_pools (id, organiser_id, event_id, name, capacity, scope)
     values (coalesce($1, gen_random_uuid()), ${CURRENT_ORG}, $2, $3, $4, $5) returning id, event_id, name, capacity, scope`,
    [fixedId ?? null, eventId, p.name, p.capacity, p.scope],
  );
  return need(r.rows[0]);
}

export async function updatePool(c: ClientBase, eventId: string, id: string, raw: PoolInput): Promise<PoolRow> {
  requireUuid(id);
  const p = poolSchema.parse(raw);
  const r = await c.query<PoolRow>(
    `update control.capacity_pools set name = $3, capacity = $4, scope = $5 where id = $1 and event_id = $2 returning id, event_id, name, capacity, scope`,
    [id, eventId, p.name, p.capacity, p.scope],
  );
  return need(r.rows[0]);
}

export function savePool(c: ClientBase, eventId: string, raw: PoolInput, id?: string): Promise<PoolRow> {
  return id ? updatePool(c, eventId, id, raw) : createPool(c, eventId, raw);
}

export async function deletePool(c: ClientBase, eventId: string, id: string): Promise<void> {
  requireUuid(eventId);
  requireUuid(id);
  await c.query("delete from control.capacity_pools where id = $1 and event_id = $2", [id, eventId]);
}

export async function listPools(c: ClientBase, eventId: string): Promise<PoolRow[]> {
  return listPoolsOf(c, [eventId]);
}

async function listPoolsOf(c: ClientBase, eventIds: string[]): Promise<PoolRow[]> {
  return (await c.query<PoolRow>("select id, event_id, name, capacity, scope from control.capacity_pools where event_id = any($1::uuid[]) order by name, id", [eventIds])).rows;
}

/** Sets which pools a product draws on (many-to-many). Pools must belong to the product's event. */
export async function setProductPools(c: ClientBase, productId: string, poolIds: string[]): Promise<void> {
  const prod = need(await getProduct(c, productId));
  const ids = [...new Set(poolIds)];
  if (ids.some((i) => !isUuid(i))) throw new DomainError("not_found", "pools");
  if (ids.length) {
    const r = await c.query("select count(*)::int as n from control.capacity_pools where event_id = $1 and id = any($2::uuid[])", [prod.event_id, ids]);
    if (r.rows[0].n !== ids.length) throw new DomainError("not_found", "pools");
  }
  await c.query("delete from control.product_pools where product_id = $1 and not (pool_id = any($2::uuid[]))", [productId, ids]);
  if (ids.length) {
    await c.query(
      `insert into control.product_pools (organiser_id, event_id, product_id, pool_id)
       select ${CURRENT_ORG}, $1, $2, x from unnest($3::uuid[]) as x on conflict do nothing`,
      [prod.event_id, productId, ids],
    );
  }
}

/**
 * Links one product to one pool. The link carries the product's event, and the database refuses a
 * pool of another event or organiser (composite foreign keys).
 */
export async function linkProductPool(c: ClientBase, productId: string, poolId: string): Promise<void> {
  const r = await c.query(
    `insert into control.product_pools (organiser_id, event_id, product_id, pool_id)
     select ${CURRENT_ORG}, p.event_id, p.id, $2 from control.products p where p.id = $1 on conflict do nothing`,
    [productId, poolId],
  );
  if (r.rowCount === 0) {
    const exists = await c.query("select 1 from control.product_pools where product_id = $1 and pool_id = $2", [productId, poolId]);
    if (!exists.rowCount) throw new DomainError("not_found");
  }
}

export async function listProductPools(c: ClientBase, eventId: string): Promise<{ product_id: string; pool_id: string }[]> {
  return (await listLinksOf(c, [eventId])).map(({ product_id, pool_id }) => ({ product_id, pool_id }));
}

async function listLinksOf(c: ClientBase, eventIds: string[]): Promise<{ event_id: string; product_id: string; pool_id: string }[]> {
  return (await c.query("select event_id, product_id, pool_id from control.product_pools where event_id = any($1::uuid[])", [eventIds])).rows;
}

// ---------------------------------------------------------------- price ladders

export type StepRow = { product_id: string; position: number; mode: "by_date" | "by_sales"; from_at: Date | null; after_sold: number | null; price_minor: number };

export async function setLadder(c: ClientBase, productId: string, raw: LadderInput): Promise<void> {
  const l = ladderSchema.parse(raw);
  need(await getProduct(c, productId));
  await c.query("delete from control.price_steps where product_id = $1", [productId]);
  await c.query(
    `insert into control.price_steps (organiser_id, product_id, position, mode, from_at, after_sold, price_minor)
     select ${CURRENT_ORG}, $1, x.position, $2, x.from_at, x.after_sold, x.price
     from unnest($3::int[], $4::timestamptz[], $5::int[], $6::bigint[]) as x(position, from_at, after_sold, price)`,
    [
      productId,
      l.mode,
      l.steps.map((_, i) => i),
      l.steps.map((s, i) => (i === 0 || l.mode !== "by_date" ? null : s.fromAt ?? null)),
      l.steps.map((s, i) => (i === 0 || l.mode !== "by_sales" ? null : s.afterSold ?? null)),
      l.steps.map((s) => s.priceMinor),
    ],
  );
}

export async function listSteps(c: ClientBase, eventId: string): Promise<StepRow[]> {
  return listStepsOf(c, [eventId]);
}

async function listStepsOf(c: ClientBase, eventIds: string[]): Promise<StepRow[]> {
  return (
    await c.query<StepRow>(
      `select s.product_id, s.position, s.mode, s.from_at, s.after_sold, s.price_minor::int as price_minor
       from control.price_steps s join control.products p on p.id = s.product_id where p.event_id = any($1::uuid[]) order by s.product_id, s.position`,
      [eventIds],
    )
  ).rows;
}

export function stepsOf(rows: StepRow[], productId: string): PriceStep[] {
  return rows
    .filter((r) => r.product_id === productId)
    .sort((a, b) => a.position - b.position)
    .map((r) => (r.mode === "by_date" ? { mode: "by_date", fromAt: r.from_at, priceMinor: r.price_minor } : { mode: "by_sales", afterSold: r.after_sold, priceMinor: r.price_minor }));
}

// ---------------------------------------------------------------- instalments

export type PlanRow = { product_id: string; deposit_minor: number | null; deposit_bps: number | null; monthly_count: number; day_of_month: number };

/**
 * The earliest purchase a plan must serve: now, or the first sales day when sales open later. The
 * plan's count is a maximum; each buyer gets as many payments as still fit from their own purchase.
 */
export function planReference(product: { sales_from: Date | null }, now: Date): Date {
  return product.sales_from && product.sales_from.getTime() > now.getTime() ? product.sales_from : now;
}

export async function pricingContext(c: ClientBase, ev: EventRow): Promise<{ currency: Currency; policy: FeePolicy; fee: { percentBps: number; fixedMinor: number } }> {
  const b = need((await c.query<{ currency: Currency }>("select currency from control.brands where id = $1", [ev.brand_id])).rows[0]);
  const policy = effectivePolicy(await getFeePolicy(c), ev.fee_policy);
  return { currency: b.currency, policy, fee: await getPlatformFee(c, b.currency) };
}

/** The deposit of a stored plan for an all-in price. */
export function planDeposit(plan: Pick<PlanRow, "deposit_minor" | "deposit_bps">, allInMinor: number): number {
  return depositFor({ depositMinor: plan.deposit_minor, depositBps: plan.deposit_bps }, allInMinor);
}

export async function setInstalmentPlan(c: ClientBase, productId: string, raw: InstalmentInput | null, now = new Date()): Promise<void> {
  const prod = need(await getProduct(c, productId));
  if (raw === null) {
    await c.query("delete from control.instalment_plans where product_id = $1", [productId]);
    return;
  }
  const plan = instalmentSchema.parse(raw);
  const ev = need(await getEvent(c, prod.event_id));
  const steps = stepsOf(await listSteps(c, ev.id), productId);
  if (!steps.length) throw new DomainError("price_required", "depositMinor");
  const ctx = await pricingContext(c, ev);
  const from = planReference(prod, now);
  const allIn = feeBreakdown(activePriceStep({ steps, at: from }).priceMinor, ctx.policy, ctx.fee).allInMinor;
  const s = instalmentSchedule({
    allInMinor: allIn,
    depositMinor: depositFor(plan, allIn),
    monthlyCount: plan.monthlyCount,
    dayOfMonth: plan.dayOfMonth,
    from,
    eventStartsAt: ev.starts_at,
    timeZone: ev.timezone,
  });
  if (!s.ok) {
    if (s.code === "past_event") throw new DomainError("instalments_end_after_event", "monthlyCount", { max: s.maxCount });
    throw new DomainError(s.code, s.code === "deposit_too_large" ? (plan.depositBps !== null ? "depositBps" : "depositMinor") : undefined);
  }
  await c.query(
    `insert into control.instalment_plans (organiser_id, product_id, deposit_minor, deposit_bps, monthly_count, day_of_month) values (${CURRENT_ORG}, $1, $2, $3, $4, $5)
     on conflict (product_id) do update set deposit_minor = excluded.deposit_minor, deposit_bps = excluded.deposit_bps,
       monthly_count = excluded.monthly_count, day_of_month = excluded.day_of_month`,
    [productId, plan.depositMinor, plan.depositBps, plan.monthlyCount, plan.dayOfMonth],
  );
}

/** For Studio: from which local date buyers get how many payments (one row per count). */
export function planBreakpoints(plan: Pick<PlanRow, "monthly_count" | "day_of_month">, from: Date, eventStartsAt: Date, timeZone: string) {
  const out: { from: string; count: number }[] = [];
  let at = localDate(from, timeZone);
  for (let i = 0; i < 30; i++) {
    const count = Math.min(plan.monthly_count, maxMonthlyPayments(new Date(`${at}T12:00:00Z`), plan.day_of_month, eventStartsAt, timeZone));
    if (out.at(-1)?.count !== count) out.push({ from: at, count });
    if (count === 0) break;
    const y = Number(at.slice(0, 4));
    const m = Number(at.slice(5, 7));
    at = new Date(Date.UTC(y, m, 1)).toISOString().slice(0, 10); // first day of next month
  }
  return out;
}

export async function listPlans(c: ClientBase, eventId: string): Promise<PlanRow[]> {
  return listPlansOf(c, [eventId]);
}

async function listPlansOf(c: ClientBase, eventIds: string[]): Promise<PlanRow[]> {
  return (
    await c.query<PlanRow>(
      `select i.product_id, i.deposit_minor::int as deposit_minor, i.deposit_bps, i.monthly_count, i.day_of_month
       from control.instalment_plans i join control.products p on p.id = i.product_id where p.event_id = any($1::uuid[])`,
      [eventIds],
    )
  ).rows;
}

// ---------------------------------------------------------------- the whole event

export type EventGraph = {
  event: EventRow;
  brand: { id: string; name: string; slug: string; languages: ("ro" | "en")[]; currency: Currency };
  venue: { id: string; name: string } | null;
  days: DayRow[];
  lineup: LineupRow[];
  products: ProductRow[];
  pools: PoolRow[];
  links: { product_id: string; pool_id: string }[];
  steps: StepRow[];
  plans: PlanRow[];
  organiserPolicy: FeePolicy;
  fee: { percentBps: number; fixedMinor: number };
};

/** Loads many events with a fixed number of queries (Studio overview, readiness of every event). */
export async function loadEventGraphs(c: ClientBase, eventIds: string[]): Promise<EventGraph[]> {
  const ids = eventIds.filter(isUuid);
  if (!ids.length) return [];
  const events = (await c.query<EventRow>(`select ${EVENT_COLS} from control.events where id = any($1::uuid[])`, [ids])).rows;
  if (!events.length) return [];
  const evIds = events.map((e) => e.id);
  const brands = (await c.query<EventGraph["brand"]>("select id, name, slug, languages, currency from control.brands where id = any($1::uuid[])", [[...new Set(events.map((e) => e.brand_id))]])).rows;
  const venueIds = [...new Set(events.map((e) => e.venue_id).filter((v): v is string => !!v))];
  const venues = venueIds.length ? (await c.query<{ id: string; name: string }>("select id, name from control.venues where id = any($1::uuid[])", [venueIds])).rows : [];
  const [days, lineup, products, pools, links, steps, plans, organiserPolicy] = [
    await listDaysOf(c, evIds),
    await listLineupOf(c, evIds),
    await listProductsOf(c, evIds),
    await listPoolsOf(c, evIds),
    await listLinksOf(c, evIds),
    await listStepsOf(c, evIds),
    await listPlansOf(c, evIds),
    await getFeePolicy(c),
  ];
  const fees = new Map<Currency, { percentBps: number; fixedMinor: number }>();
  for (const cur of new Set(brands.map((b) => b.currency))) fees.set(cur, await getPlatformFee(c, cur));
  const byId = new Map(events.map((e) => [e.id, e]));
  return eventIds
    .map((id) => byId.get(id))
    .filter((e): e is EventRow => !!e)
    .map((event) => {
      const brand = need(brands.find((b) => b.id === event.brand_id));
      const productIds = new Set(products.filter((p) => p.event_id === event.id).map((p) => p.id));
      return {
        event,
        brand,
        venue: venues.find((v) => v.id === event.venue_id) ?? null,
        days: days.filter((d) => d.event_id === event.id).map(({ event_id: _e, ...d }) => d), // eslint-disable-line @typescript-eslint/no-unused-vars
        lineup: lineup.filter((l) => l.event_id === event.id).map(({ event_id: _e, ...l }) => l), // eslint-disable-line @typescript-eslint/no-unused-vars
        products: products.filter((p) => p.event_id === event.id),
        pools: pools.filter((p) => p.event_id === event.id),
        links: links.filter((l) => l.event_id === event.id).map(({ product_id, pool_id }) => ({ product_id, pool_id })),
        steps: steps.filter((s) => productIds.has(s.product_id)),
        plans: plans.filter((p) => productIds.has(p.product_id)),
        organiserPolicy,
        fee: fees.get(brand.currency)!,
      };
    });
}

export async function loadEventGraph(c: ClientBase, eventId: string): Promise<EventGraph | null> {
  return (await loadEventGraphs(c, [eventId]))[0] ?? null;
}
