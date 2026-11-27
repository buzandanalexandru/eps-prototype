// Catalogue operations (design D1.4). Every function runs on a client inside withTx, so row-level
// security limits it to the transaction's organiser and composite foreign keys refuse links to
// another organiser's rows. Studio's server actions and the seed both go through here.
// Functions that take an optional id upsert by it (the seed's stable ids); without one they insert.
import type { ClientBase } from "pg";
import * as z from "zod";
import { DomainError } from "../errors";
import { CURRENT_ORG, getFeePolicy, getPlatformFee } from "../organisers/domain";
import { activePriceStep, type PriceStep } from "../pricing/ladder";
import { effectivePolicy, feeBreakdown, type FeePolicy } from "../pricing/fees";
import { instalmentSchedule } from "../pricing/instalments";
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

const isUuid = (v: string) => z.uuid().safeParse(v).success;

function need<T>(row: T | undefined | null): T {
  if (!row) throw new DomainError("not_found");
  return row;
}

// ---------------------------------------------------------------- venues

export type VenueRow = { id: string; name: string; address: string; lat: number | null; lng: number | null; stages: string[] };

export async function saveVenue(c: ClientBase, raw: VenueInput, id?: string): Promise<VenueRow> {
  const v = venueSchema.parse(raw);
  const r = await c.query<VenueRow>(
    `insert into control.venues (id, organiser_id, name, address, lat, lng, stages)
     values (coalesce($1, gen_random_uuid()), ${CURRENT_ORG}, $2, $3, $4, $5, $6)
     on conflict (id) do update set name = excluded.name, address = excluded.address, lat = excluded.lat, lng = excluded.lng, stages = excluded.stages
     returning id, name, address, lat, lng, stages`,
    [id ?? null, v.name, v.address, v.lat, v.lng, v.stages],
  );
  return need(r.rows[0]);
}

export async function listVenues(c: ClientBase): Promise<(VenueRow & { events: number })[]> {
  return (
    await c.query(
      `select v.id, v.name, v.address, v.lat, v.lng, v.stages, (select count(*)::int from control.events e where e.venue_id = v.id) as events
       from control.venues v order by v.name`,
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
};

const EVENT_COLS =
  "id, brand_id, venue_id, slug, name, description, timezone, starts_at, ends_at, status, edition_of, fee_policy, min_age, consent_under_age, max_per_account";

export async function saveEvent(c: ClientBase, raw: EventInput, id?: string): Promise<EventRow> {
  const e = eventSchema.parse(raw);
  if (id) {
    // Moving dates must keep existing days inside the event.
    const days = await c.query<{ day: string }>("select day::text from control.event_days where event_id = $1", [id]);
    const lo = localDate(e.startsAt, e.timezone);
    const hi = localDate(e.endsAt, e.timezone);
    if (days.rows.some((d) => d.day < lo || d.day > hi)) throw new DomainError("day_outside_event", "endsAt");
  }
  const r = await c.query<EventRow>(
    `insert into control.events (id, organiser_id, brand_id, venue_id, slug, name, description, timezone, starts_at, ends_at)
     values (coalesce($1, gen_random_uuid()), ${CURRENT_ORG}, $2, $3, $4, $5, $6, $7, $8, $9)
     on conflict (id) do update set brand_id = excluded.brand_id, venue_id = excluded.venue_id, slug = excluded.slug,
       name = excluded.name, description = excluded.description, timezone = excluded.timezone,
       starts_at = excluded.starts_at, ends_at = excluded.ends_at, updated_at = now()
     returning ${EVENT_COLS}`,
    [id ?? null, e.brandId, e.venueId, e.slug, e.name, e.description, e.timezone, e.startsAt, e.endsAt],
  );
  return need(r.rows[0]);
}

export async function getEvent(c: ClientBase, id: string): Promise<EventRow | null> {
  if (!isUuid(id)) return null;
  return (await c.query<EventRow>(`select ${EVENT_COLS} from control.events where id = $1`, [id])).rows[0] ?? null;
}

export async function listEvents(c: ClientBase): Promise<(EventRow & { brand_name: string; products: number })[]> {
  return (
    await c.query(
      `select ${EVENT_COLS.split(", ").map((x) => "e." + x).join(", ")}, b.name as brand_name,
        (select count(*)::int from control.products p where p.event_id = e.id) as products
       from control.events e join control.brands b on b.id = e.brand_id order by e.starts_at, e.slug`,
    )
  ).rows;
}

export async function deleteEvent(c: ClientBase, id: string): Promise<void> {
  const r = await c.query("delete from control.events where id = $1 and status = 'draft'", [id]);
  if (!r.rowCount) throw new DomainError("not_found");
}

export async function setRules(c: ClientBase, eventId: string, raw: RulesInput): Promise<void> {
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
  for (const d of days) {
    await c.query(
      `insert into control.event_days (id, organiser_id, event_id, day, opens, closes) values (coalesce($1, gen_random_uuid()), ${CURRENT_ORG}, $2, $3, $4, $5)
       on conflict (event_id, day) do update set opens = excluded.opens, closes = excluded.closes`,
      [ids?.[d.day] ?? null, eventId, d.day, d.opens, d.closes],
    );
  }
  return listDays(c, eventId);
}

export async function listDays(c: ClientBase, eventId: string): Promise<DayRow[]> {
  return (
    await c.query<DayRow>(
      `select id, day::text as day, to_char(opens, 'HH24:MI') as opens, to_char(closes, 'HH24:MI') as closes
       from control.event_days where event_id = $1 order by day`,
      [eventId],
    )
  ).rows;
}

export type LineupRow = { id: string; day_id: string; artist: string; stage: string; starts: string; sort: number };

export async function addLineupSlot(c: ClientBase, eventId: string, raw: LineupInput, id?: string): Promise<LineupRow> {
  const l = lineupSchema.parse(raw);
  const r = await c.query<LineupRow>(
    `insert into control.lineup_slots (id, organiser_id, event_id, day_id, artist, stage, starts, sort)
     select coalesce($1, gen_random_uuid()), ${CURRENT_ORG}, $2, d.id, $4, $5, $6, $7 from control.event_days d where d.id = $3 and d.event_id = $2
     on conflict (id) do update set day_id = excluded.day_id, artist = excluded.artist, stage = excluded.stage, starts = excluded.starts, sort = excluded.sort
     returning id, day_id, artist, stage, to_char(starts, 'HH24:MI') as starts, sort`,
    [id ?? null, eventId, l.dayId, l.artist, l.stage, l.starts, l.sort],
  );
  return need(r.rows[0]);
}

export async function deleteLineupSlot(c: ClientBase, eventId: string, id: string): Promise<void> {
  await c.query("delete from control.lineup_slots where id = $1 and event_id = $2", [id, eventId]);
}

export async function listLineup(c: ClientBase, eventId: string): Promise<LineupRow[]> {
  return (
    await c.query<LineupRow>(
      `select id, day_id, artist, stage, to_char(starts, 'HH24:MI') as starts, sort from control.lineup_slots where event_id = $1`,
      [eventId],
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
};
const PRODUCT_COLS = "id, event_id, type, name, perks, config, named, sales_from, sales_until, visible, max_per_order, sort";

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

export async function saveProduct(c: ClientBase, eventId: string, raw: ProductInput, id?: string): Promise<ProductRow> {
  const p = productSchema.parse(raw);
  need(await getEvent(c, eventId));
  if (id) {
    const cur = await c.query<{ event_id: string; type: string }>("select event_id, type from control.products where id = $1", [id]);
    if (cur.rows[0] && (cur.rows[0].event_id !== eventId)) throw new DomainError("not_found");
  }
  await checkAgainstEvent(c, eventId, p, id);
  const r = await c.query<ProductRow>(
    `insert into control.products (id, organiser_id, event_id, type, name, perks, config, named, sales_from, sales_until, visible, max_per_order, sort)
     values (coalesce($1, gen_random_uuid()), ${CURRENT_ORG}, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
     on conflict (id) do update set type = excluded.type, name = excluded.name, perks = excluded.perks, config = excluded.config,
       named = excluded.named, sales_from = excluded.sales_from, sales_until = excluded.sales_until, visible = excluded.visible,
       max_per_order = excluded.max_per_order, sort = excluded.sort
     returning ${PRODUCT_COLS}`,
    [id ?? null, eventId, p.type, p.name, JSON.stringify(p.perks), p.config, p.named, p.salesFrom, p.salesUntil, p.visible, p.maxPerOrder, p.sort],
  );
  return need(r.rows[0]);
}

export async function getProduct(c: ClientBase, id: string): Promise<ProductRow | null> {
  if (!isUuid(id)) return null;
  return (await c.query<ProductRow>(`select ${PRODUCT_COLS} from control.products where id = $1`, [id])).rows[0] ?? null;
}

export async function listProducts(c: ClientBase, eventId: string): Promise<ProductRow[]> {
  return (await c.query<ProductRow>(`select ${PRODUCT_COLS} from control.products where event_id = $1 order by sort, created_at`, [eventId])).rows;
}

export async function deleteProduct(c: ClientBase, eventId: string, id: string): Promise<void> {
  await c.query("delete from control.products where id = $1 and event_id = $2", [id, eventId]);
}

// ---------------------------------------------------------------- pools

export type PoolRow = { id: string; event_id: string; name: string; capacity: number; scope: PoolScope };

export async function savePool(c: ClientBase, eventId: string, raw: PoolInput, id?: string): Promise<PoolRow> {
  const p = poolSchema.parse(raw);
  const r = await c.query<PoolRow>(
    `insert into control.capacity_pools (id, organiser_id, event_id, name, capacity, scope)
     values (coalesce($1, gen_random_uuid()), ${CURRENT_ORG}, $2, $3, $4, $5)
     on conflict (id) do update set name = excluded.name, capacity = excluded.capacity, scope = excluded.scope
     where control.capacity_pools.event_id = excluded.event_id
     returning id, event_id, name, capacity, scope`,
    [id ?? null, eventId, p.name, p.capacity, p.scope],
  );
  return need(r.rows[0]);
}

export async function deletePool(c: ClientBase, eventId: string, id: string): Promise<void> {
  await c.query("delete from control.capacity_pools where id = $1 and event_id = $2", [id, eventId]);
}

export async function listPools(c: ClientBase, eventId: string): Promise<PoolRow[]> {
  return (await c.query<PoolRow>("select id, event_id, name, capacity, scope from control.capacity_pools where event_id = $1 order by name", [eventId])).rows;
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
  for (const poolId of ids) {
    await c.query(
      `insert into control.product_pools (organiser_id, product_id, pool_id) values (${CURRENT_ORG}, $1, $2) on conflict do nothing`,
      [productId, poolId],
    );
  }
}

/** Links one product to one pool (used by isolation tests and the pools screen). */
export async function linkProductPool(c: ClientBase, productId: string, poolId: string): Promise<void> {
  await c.query(`insert into control.product_pools (organiser_id, product_id, pool_id) values (${CURRENT_ORG}, $1, $2) on conflict do nothing`, [productId, poolId]);
}

export async function listProductPools(c: ClientBase, eventId: string): Promise<{ product_id: string; pool_id: string }[]> {
  return (
    await c.query(
      `select pp.product_id, pp.pool_id from control.product_pools pp join control.products p on p.id = pp.product_id where p.event_id = $1`,
      [eventId],
    )
  ).rows;
}

// ---------------------------------------------------------------- price ladders

export type StepRow = { product_id: string; position: number; mode: "by_date" | "by_sales"; from_at: Date | null; after_sold: number | null; price_minor: number };

export async function setLadder(c: ClientBase, productId: string, raw: LadderInput): Promise<void> {
  const l = ladderSchema.parse(raw);
  need(await getProduct(c, productId));
  await c.query("delete from control.price_steps where product_id = $1", [productId]);
  for (const [i, s] of l.steps.entries()) {
    await c.query(
      `insert into control.price_steps (organiser_id, product_id, position, mode, from_at, after_sold, price_minor)
       values (${CURRENT_ORG}, $1, $2, $3, $4, $5, $6)`,
      [productId, i, l.mode, i === 0 || l.mode !== "by_date" ? null : s.fromAt ?? null, i === 0 || l.mode !== "by_sales" ? null : s.afterSold ?? null, s.priceMinor],
    );
  }
}

export async function listSteps(c: ClientBase, eventId: string): Promise<StepRow[]> {
  return (
    await c.query<StepRow>(
      `select s.product_id, s.position, s.mode, s.from_at, s.after_sold, s.price_minor::int as price_minor
       from control.price_steps s join control.products p on p.id = s.product_id where p.event_id = $1 order by s.product_id, s.position`,
      [eventId],
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

export type PlanRow = { product_id: string; deposit_minor: number; monthly_count: number; day_of_month: number };

/**
 * The day a plan is checked from: the product's first sales day, else now. A buyer on the first sales
 * day gets the full plan; checkout (step 4) offers later buyers at most the payments that still end
 * 7 days before the event (maxMonthlyPayments), so no real schedule breaks the rule.
 */
export function planReference(product: { sales_from: Date | null }, now: Date): Date {
  return product.sales_from ?? now;
}

export async function pricingContext(c: ClientBase, ev: EventRow): Promise<{ currency: Currency; policy: FeePolicy; fee: { percentBps: number; fixedMinor: number } }> {
  const b = need((await c.query<{ currency: Currency }>("select currency from control.brands where id = $1", [ev.brand_id])).rows[0]);
  const policy = effectivePolicy(await getFeePolicy(c), ev.fee_policy);
  return { currency: b.currency, policy, fee: await getPlatformFee(c, b.currency) };
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
  const s = instalmentSchedule({ allInMinor: allIn, depositMinor: plan.depositMinor, monthlyCount: plan.monthlyCount, dayOfMonth: plan.dayOfMonth, from, eventStartsAt: ev.starts_at });
  if (!s.ok) {
    if (s.code === "past_event") throw new DomainError("instalments_end_after_event", "monthlyCount", { max: s.maxCount });
    throw new DomainError(s.code, s.code === "deposit_too_large" ? "depositMinor" : undefined);
  }
  await c.query(
    `insert into control.instalment_plans (organiser_id, product_id, deposit_minor, monthly_count, day_of_month) values (${CURRENT_ORG}, $1, $2, $3, $4)
     on conflict (product_id) do update set deposit_minor = excluded.deposit_minor, monthly_count = excluded.monthly_count, day_of_month = excluded.day_of_month`,
    [productId, plan.depositMinor, plan.monthlyCount, plan.dayOfMonth],
  );
}

export async function listPlans(c: ClientBase, eventId: string): Promise<PlanRow[]> {
  return (
    await c.query<PlanRow>(
      `select i.product_id, i.deposit_minor::int as deposit_minor, i.monthly_count, i.day_of_month
       from control.instalment_plans i join control.products p on p.id = i.product_id where p.event_id = $1`,
      [eventId],
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

export async function loadEventGraph(c: ClientBase, eventId: string): Promise<EventGraph | null> {
  const event = await getEvent(c, eventId);
  if (!event) return null;
  const brand = need(
    (await c.query("select id, name, slug, languages, currency from control.brands where id = $1", [event.brand_id])).rows[0] as EventGraph["brand"] | undefined,
  );
  const venue = event.venue_id ? ((await c.query("select id, name from control.venues where id = $1", [event.venue_id])).rows[0] ?? null) : null;
  const [days, lineup, products, pools, links, steps, plans, organiserPolicy, fee] = [
    await listDays(c, eventId),
    await listLineup(c, eventId),
    await listProducts(c, eventId),
    await listPools(c, eventId),
    await listProductPools(c, eventId),
    await listSteps(c, eventId),
    await listPlans(c, eventId),
    await getFeePolicy(c),
    await getPlatformFee(c, brand.currency),
  ];
  return { event, brand, venue, days, lineup, products, pools, links, steps, plans, organiserPolicy, fee };
}
