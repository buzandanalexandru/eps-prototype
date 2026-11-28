// The snapshot compiler (design D3.1, D3.2): an event's loaded catalogue graph in, its sale terms
// out. Pure over the EventGraph plus the venue and the organiser's sample flag, so the publish hook,
// the tests and the seed check compile the same way. All amounts come from lib/pricing's
// feeBreakdown: no consumer of a snapshot ever computes a fee.
import { createHash } from "node:crypto";
import { stepsOf, type EventGraph } from "../catalogue/domain";
import { TICKET_TYPES } from "../catalogue/schema";
import { slotStarts } from "./slots";
import { slotStartDate } from "../catalogue/time";
import { effectivePolicy, feeBreakdown } from "../pricing/fees";
import { lastAllowedDay } from "../pricing/instalments";
import {
  SNAPSHOT_SCHEMA_VERSION,
  type L10n,
  type SaleSnapshot,
  type SaleTerms,
  type SnapshotPool,
  type SnapshotPoolInstance,
  type SnapshotProduct,
} from "./types";

export type CompileExtras = {
  venue: { name: string; address: string; lat: number | null; lng: number | null } | null;
  isSample: boolean;
  organiserId: string;
  /** The selling shop's checkout slots (its storefront document's checkout.slots). */
  checkoutQuestions: { afterContact: string[]; consents: string[] };
};

const l10n = (v: Partial<L10n> | null | undefined): L10n => ({ ro: v?.ro ?? "", en: v?.en ?? "" });
const iso = (d: Date | null): string | null => (d ? d.toISOString() : null);

function poolInstances(g: EventGraph, pool: EventGraph["pools"][number]): SnapshotPoolInstance[] {
  if (pool.scope === "event") return [{ instanceId: pool.id, date: null, slotStart: null, capacity: pool.capacity }];
  const days = [...g.days].sort((a, b) => a.day.localeCompare(b.day));
  if (pool.scope === "day") return days.map((d) => ({ instanceId: `${pool.id}@${d.day}`, date: d.day, slotStart: null, capacity: pool.capacity }));
  // Slot pools: the grid of the time slots drawing on it, on every event day. A slot that starts
  // after midnight belongs to the next local date (spec map).
  const slot = g.links
    .filter((l) => l.pool_id === pool.id)
    .map((l) => g.products.find((p) => p.id === l.product_id))
    .find((p) => p?.type === "time_slot");
  if (!slot) return [];
  const cfg = slot.config as { slotMinutes: number; firstStart: string; lastStart: string };
  const starts = slotStarts(cfg);
  const out: SnapshotPoolInstance[] = [];
  for (const d of days) {
    for (const s of starts) {
      const date = slotStartDate(d.day, cfg.firstStart, s);
      out.push({ instanceId: `${pool.id}@${date}T${s}`, date, slotStart: s, capacity: pool.capacity });
    }
  }
  return out;
}

function requiresOf(g: EventGraph, p: EventGraph["products"][number]): string[] {
  if (p.type !== "add_on") return [];
  const req = (p.config as { requires?: unknown }).requires;
  if (Array.isArray(req)) return req.filter((x): x is string => typeof x === "string");
  if (req === "none") return [];
  // "any_ticket" (the default): any ticket product of the event.
  return g.products.filter((q) => TICKET_TYPES.includes(q.type)).sort((a, b) => a.sort - b.sort || a.id.localeCompare(b.id)).map((q) => q.id);
}

/** The sale terms of one event: everything except the snapshot's id, version and time. */
export function compileTerms(g: EventGraph, x: CompileExtras): SaleTerms {
  const ev = g.event;
  const policy = effectivePolicy(g.organiserPolicy, ev.fee_policy);
  const dayOf = new Map(g.days.map((d) => [d.id, d.day]));
  const products: SnapshotProduct[] = [...g.products]
    .sort((a, b) => a.sort - b.sort || a.id.localeCompare(b.id))
    .map((p) => {
      const steps = stepsOf(g.steps, p.id);
      const plan = g.plans.find((pl) => pl.product_id === p.id);
      return {
        productId: p.id,
        type: p.type,
        name: l10n(p.name),
        perks: (p.perks ?? []).map(l10n),
        zone: p.type === "pass" ? String((p.config as { zone?: string }).zone ?? "") : null,
        config: p.config,
        named: p.named,
        salesFrom: iso(p.sales_from),
        salesUntil: iso(p.sales_until),
        visible: p.visible,
        maxPerOrder: p.max_per_order,
        sortOrder: p.sort,
        poolIds: g.links.filter((l) => l.product_id === p.id).map((l) => l.pool_id).sort(),
        requires: requiresOf(g, p),
        priceSteps: steps.map((s, index) => {
          const b = feeBreakdown(s.priceMinor, policy, g.fee);
          return {
            index,
            mode: s.mode,
            fromAt: s.mode === "by_date" ? iso(s.fromAt) : null,
            afterSold: s.mode === "by_sales" ? s.afterSold : null,
            faceMinor: b.faceMinor,
            allInMinor: b.allInMinor,
            fanFeeMinor: b.fanFeeMinor,
            organiserNetMinor: b.organiserNetMinor,
          };
        }),
        instalments: plan
          ? {
              depositMinor: plan.deposit_minor,
              depositBps: plan.deposit_bps,
              monthlyCount: plan.monthly_count,
              dayOfMonth: plan.day_of_month,
              lastDueBy: lastAllowedDay(ev.starts_at, ev.timezone),
            }
          : null,
      };
    });
  const pools: SnapshotPool[] = [...g.pools]
    .sort((a, b) => a.id.localeCompare(b.id))
    .map((pool) => ({ poolId: pool.id, name: pool.name, scope: pool.scope, instances: poolInstances(g, pool) }));
  return {
    schemaVersion: SNAPSHOT_SCHEMA_VERSION,
    organiserId: x.organiserId,
    brandId: g.brand.id,
    eventId: ev.id,
    currency: g.brand.currency,
    timezone: ev.timezone,
    isSample: x.isSample,
    fee: { policy, bps: g.fee.percentBps, fixedMinor: g.fee.fixedMinor },
    event: {
      slug: ev.slug,
      name: l10n(ev.name),
      description: l10n(ev.description),
      startsAt: ev.starts_at.toISOString(),
      endsAt: ev.ends_at.toISOString(),
      timezone: ev.timezone,
      venue: x.venue ? { name: x.venue.name, address: x.venue.address, lat: x.venue.lat, lng: x.venue.lng } : null,
      days: [...g.days].sort((a, b) => a.day.localeCompare(b.day)).map((d) => ({ date: d.day, opens: d.opens.slice(0, 5), closes: d.closes.slice(0, 5) })),
      lineup: [...g.lineup]
        .map((l) => ({ date: dayOf.get(l.day_id) ?? "", artist: l.artist, stage: l.stage, starts: l.starts.slice(0, 5), sort: l.sort, id: l.id, sample: !!l.sample }))
        .sort((a, b) => a.date.localeCompare(b.date) || a.sort - b.sort || a.id.localeCompare(b.id))
        .map((l) => ({ date: l.date, artist: l.artist, stage: l.stage, starts: l.starts, sample: l.sample })),
    },
    rules: {
      minAge: ev.min_age,
      consentUnderAge: ev.consent_under_age,
      maxPerAccount: ev.max_per_account,
      holdMinutes: ev.hold_minutes ?? 8,
      checkoutQuestions: { afterContact: [...x.checkoutQuestions.afterContact], consents: [...x.checkoutQuestions.consents] },
    },
    pools,
    products,
  };
}

/** The full document in its fixed key order (the stored text is exactly JSON.stringify of it). */
export function buildSnapshot(terms: SaleTerms, meta: { snapshotId: string; version: number; publishedAt: string }): SaleSnapshot {
  const { schemaVersion, organiserId, brandId, eventId, ...rest } = terms;
  return { schemaVersion, snapshotId: meta.snapshotId, organiserId, brandId, eventId, version: meta.version, publishedAt: meta.publishedAt, ...rest };
}

export const sha256Hex = (text: string) => createHash("sha256").update(text, "utf8").digest("hex");

/** The hash of the sale terms, which decides whether an event needs a new version. */
export function termsHash(terms: SaleTerms): string {
  return sha256Hex(JSON.stringify(terms));
}

export { slotStarts };
