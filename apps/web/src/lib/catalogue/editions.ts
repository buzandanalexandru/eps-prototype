// Copy as a new edition (design D1.4, C1): one transaction copying the event with its days, line-up,
// products, pools, pool links, ladders and instalment plans, every date shifted by the same number of
// days (wall-clock time kept in the event's zone), as a draft that points at the original.
import type { ClientBase } from "pg";
import { randomUUID } from "node:crypto";
import { DomainError } from "../errors";
import { CURRENT_ORG } from "../organisers/domain";
import { loadEventGraph } from "./domain";
import type { Localized } from "./schema";
import { addDays, localDate, toLocalInput, zonedToUtc } from "./time";

/** Days to the same weekday one year later: 364, or 371 when 364 would land more than a week early. */
export function nextYearOffset(start: Date, tz: string): number {
  const d = localDate(start, tz);
  const sameDate = `${Number(d.slice(0, 4)) + 1}${d.slice(4)}`;
  const diff = Math.round((Date.parse(`${sameDate}T00:00:00Z`) - Date.parse(`${d}T00:00:00Z`)) / 86_400_000);
  return diff - (diff % 7) || 364; // 365 → 364, 366 → 364
}

/** Shifts an instant by whole days, keeping its local wall time in tz. */
export function shiftInstant(at: Date, days: number, tz: string): Date {
  const local = toLocalInput(at, tz);
  return zonedToUtc(`${addDays(local.slice(0, 10), days)}T${local.slice(11)}`, tz)!;
}

function bumpYear(s: string, from: number, to: number): string {
  return s.replaceAll(String(from), String(to));
}

export async function copyAsEdition(c: ClientBase, eventId: string, opts: { offsetDays?: number } = {}): Promise<{ id: string; slug: string }> {
  const g = await loadEventGraph(c, eventId);
  if (!g) throw new DomainError("not_found");
  const ev = g.event;
  const tz = ev.timezone;
  const offset = opts.offsetDays ?? nextYearOffset(ev.starts_at, tz);
  if (!Number.isInteger(offset) || offset === 0 || Math.abs(offset) > 3660) throw new DomainError("invalid", "offsetDays");
  const shift = (d: Date | null) => (d ? shiftInstant(d, offset, tz) : null);
  const shiftDay = (d: string) => addDays(d, offset);

  const fromYear = Number(localDate(ev.starts_at, tz).slice(0, 4));
  const toYear = Number(localDate(shift(ev.starts_at)!, tz).slice(0, 4));
  const name: Localized = { ro: bumpYear(ev.name.ro ?? "", fromYear, toYear), en: bumpYear(ev.name.en ?? "", fromYear, toYear) };
  let slug = ev.slug.includes(String(fromYear)) ? bumpYear(ev.slug, fromYear, toYear) : `${ev.slug}-${toYear}`.slice(0, 60);
  for (let i = 2; (await c.query("select 1 from control.events where brand_id = $1 and slug = $2", [ev.brand_id, slug])).rowCount; i++) {
    slug = `${slug.replace(/-\d+$/, "")}-${i}`.slice(0, 60);
  }

  const newId = randomUUID();
  await c.query(
    `insert into control.events (id, organiser_id, brand_id, venue_id, slug, name, description, timezone, starts_at, ends_at, status, edition_of,
       fee_policy, min_age, consent_under_age, max_per_account)
     values ($1, ${CURRENT_ORG}, $2, $3, $4, $5, $6, $7, $8, $9, 'draft', $10, $11, $12, $13, $14)`,
    [newId, ev.brand_id, ev.venue_id, slug, name, ev.description, tz, shift(ev.starts_at), shift(ev.ends_at), ev.id, ev.fee_policy, ev.min_age, ev.consent_under_age, ev.max_per_account],
  );

  const dayIds = new Map<string, string>();
  for (const d of g.days) {
    const id = randomUUID();
    dayIds.set(d.id, id);
    await c.query(`insert into control.event_days (id, organiser_id, event_id, day, opens, closes) values ($1, ${CURRENT_ORG}, $2, $3, $4, $5)`, [
      id,
      newId,
      shiftDay(d.day),
      d.opens,
      d.closes,
    ]);
  }
  for (const l of g.lineup) {
    await c.query(
      `insert into control.lineup_slots (organiser_id, event_id, day_id, artist, stage, starts, sort) values (${CURRENT_ORG}, $1, $2, $3, $4, $5, $6)`,
      [newId, dayIds.get(l.day_id), l.artist, l.stage, l.starts, l.sort],
    );
  }
  const productIds = new Map(g.products.map((p) => [p.id, randomUUID()]));
  for (const p of g.products) {
    const cfg = { ...p.config } as Record<string, unknown>;
    if (Array.isArray(cfg.days)) cfg.days = (cfg.days as string[]).map(shiftDay);
    if (typeof cfg.day === "string") cfg.day = shiftDay(cfg.day);
    if (Array.isArray(cfg.requires)) cfg.requires = (cfg.requires as string[]).map((id) => productIds.get(id) ?? id);
    await c.query(
      `insert into control.products (id, organiser_id, event_id, type, name, perks, config, named, sales_from, sales_until, visible, max_per_order, sort)
       values ($1, ${CURRENT_ORG}, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`,
      [productIds.get(p.id), newId, p.type, p.name, JSON.stringify(p.perks), cfg, p.named, shift(p.sales_from), shift(p.sales_until), p.visible, p.max_per_order, p.sort],
    );
  }
  const poolIds = new Map(g.pools.map((p) => [p.id, randomUUID()]));
  for (const p of g.pools) {
    await c.query(`insert into control.capacity_pools (id, organiser_id, event_id, name, capacity, scope) values ($1, ${CURRENT_ORG}, $2, $3, $4, $5)`, [
      poolIds.get(p.id),
      newId,
      p.name,
      p.capacity,
      p.scope,
    ]);
  }
  for (const l of g.links) {
    await c.query(`insert into control.product_pools (organiser_id, product_id, pool_id) values (${CURRENT_ORG}, $1, $2)`, [productIds.get(l.product_id), poolIds.get(l.pool_id)]);
  }
  for (const s of g.steps) {
    await c.query(
      `insert into control.price_steps (organiser_id, product_id, position, mode, from_at, after_sold, price_minor) values (${CURRENT_ORG}, $1, $2, $3, $4, $5, $6)`,
      [productIds.get(s.product_id), s.position, s.mode, shift(s.from_at), s.after_sold, s.price_minor],
    );
  }
  for (const p of g.plans) {
    await c.query(
      `insert into control.instalment_plans (organiser_id, product_id, deposit_minor, monthly_count, day_of_month) values (${CURRENT_ORG}, $1, $2, $3, $4)`,
      [productIds.get(p.product_id), p.deposit_minor, p.monthly_count, p.day_of_month],
    );
  }
  return { id: newId, slug };
}
