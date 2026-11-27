// Readiness check (design D1.5): every problem that would stop an event from being published, each
// with a code, the record it concerns and the field to fix. Step 3 refuses to publish while the
// list is non-empty. Pure over a loaded EventGraph, so tests and the seed check it the same way.
import { activePriceStep } from "../pricing/ladder";
import { feeBreakdown, effectivePolicy } from "../pricing/fees";
import { instalmentSchedule } from "../pricing/instalments";
import { ladderProblems } from "../pricing/ladder";
import { planDeposit, planReference, stepsOf, type EventGraph } from "./domain";
import { productConfigSchemas, TICKET_TYPES, type ProductType } from "./schema";

export type ProblemCode =
  | "missing_translation"
  | "no_venue"
  | "no_days"
  | "no_products"
  | "product_without_pool"
  | "product_without_price"
  | "pool_capacity_zero"
  | "pool_scope_invalid"
  | "slot_grid_mismatch"
  | "ladder_unordered"
  | "ladder_mixed"
  | "invalid_config"
  | "product_day_not_in_event"
  | "instalments_end_after_event"
  | "deposit_too_large"
  | "addon_requires_missing"
  | "consent_below_min_age";

export type Problem = {
  code: ProblemCode;
  /** Where to fix it. */
  target: { kind: "event" | "product" | "pool"; id: string };
  field?: string;
  /** Language of a missing translation, product or pool name, etc. */
  detail?: string;
  /** Studio path of the screen with the field. */
  href: string;
};

export function eventReadiness(g: EventGraph, now: Date = new Date()): Problem[] {
  const out: Problem[] = [];
  const ev = g.event;
  const base = `/studio/events/${ev.id}`;
  const evTarget = { kind: "event" as const, id: ev.id };
  const langs = g.brand.languages;

  for (const lang of langs) {
    if (!ev.name?.[lang]?.trim()) out.push({ code: "missing_translation", target: evTarget, field: `name.${lang}`, detail: lang, href: `${base}#name-${lang}` });
    if (!ev.description?.[lang]?.trim())
      out.push({ code: "missing_translation", target: evTarget, field: `description.${lang}`, detail: lang, href: `${base}#description-${lang}` });
  }
  if (!ev.venue_id) out.push({ code: "no_venue", target: evTarget, field: "venueId", href: `${base}#venueId` });
  if (g.days.length === 0) out.push({ code: "no_days", target: evTarget, field: "days", href: `${base}#days` });
  if (g.products.length === 0) out.push({ code: "no_products", target: evTarget, href: `${base}/products` });
  if (ev.consent_under_age !== null && ev.min_age !== null && ev.consent_under_age < ev.min_age)
    out.push({ code: "consent_below_min_age", target: evTarget, field: "consentUnderAge", href: `${base}#rules` });

  const dayset = new Set(g.days.map((d) => d.day));
  const hasTicket = g.products.some((p) => TICKET_TYPES.includes(p.type));
  const policy = effectivePolicy(g.organiserPolicy, ev.fee_policy);

  for (const p of g.products) {
    const href = `${base}/products/${p.id}`;
    const target = { kind: "product" as const, id: p.id };
    const label = p.name?.en || p.name?.ro || p.type;
    for (const lang of langs) {
      if (!p.name?.[lang]?.trim()) out.push({ code: "missing_translation", target, field: `name.${lang}`, detail: `${label} · ${lang}`, href: `${href}#name-${lang}` });
    }
    const cfg = productConfigSchemas[p.type as ProductType]?.safeParse(p.config);
    if (!cfg?.success) out.push({ code: "invalid_config", target, field: "config", detail: label, href });
    else {
      const c = cfg.data as Record<string, unknown>;
      const days = (Array.isArray(c.days) ? c.days : typeof c.day === "string" ? [c.day] : []) as string[];
      if (days.some((d) => !dayset.has(d))) out.push({ code: "product_day_not_in_event", target, field: "config.days", detail: label, href });
      if (p.type === "add_on") {
        const req = c.requires;
        if (req === "any_ticket" && !hasTicket) out.push({ code: "addon_requires_missing", target, field: "config.requires", detail: label, href });
        if (Array.isArray(req) && req.some((id) => !g.products.some((q) => q.id === id)))
          out.push({ code: "addon_requires_missing", target, field: "config.requires", detail: label, href });
      }
    }
    const myPools = g.links.filter((l) => l.product_id === p.id).map((l) => g.pools.find((x) => x.id === l.pool_id)).filter((x) => !!x);
    if (myPools.length === 0) out.push({ code: "product_without_pool", target, field: "pools", detail: label, href: `${href}#pools` });
    // Pool scopes (orchestrator decision): passes, flexible days and add-ons draw on event pools;
    // per-slot pools are for time slots only.
    const badScope =
      (["pass", "flex_day", "add_on"].includes(p.type) && myPools.some((x) => x.scope !== "event")) ||
      (p.type !== "time_slot" && myPools.some((x) => x.scope === "slot"));
    if (badScope) out.push({ code: "pool_scope_invalid", target, field: "pools", detail: label, href: `${href}#pools` });
    const steps = stepsOf(g.steps, p.id);
    if (steps.length === 0) out.push({ code: "product_without_price", target, field: "ladder", detail: label, href: `${href}#ladder` });
    else {
      const lp = ladderProblems(steps);
      if (lp.includes("ladder_mixed")) out.push({ code: "ladder_mixed", target, field: "ladder", detail: label, href: `${href}#ladder` });
      else if (lp.length) out.push({ code: "ladder_unordered", target, field: "ladder", detail: label, href: `${href}#ladder` });
    }
    const plan = g.plans.find((x) => x.product_id === p.id);
    if (plan && steps.length) {
      // A plan is a maximum: buyers get as many payments as still fit. It blocks publishing only
      // when not even one payment fits for the next buyer, or the deposit is not below the price.
      const from = planReference(p, now);
      const allIn = feeBreakdown(activePriceStep({ steps, at: from }).priceMinor, policy, g.fee).allInMinor;
      const s = instalmentSchedule({
        allInMinor: allIn,
        depositMinor: planDeposit(plan, allIn),
        monthlyCount: plan.monthly_count,
        dayOfMonth: plan.day_of_month,
        from,
        eventStartsAt: ev.starts_at,
        timeZone: ev.timezone,
        capToFit: true,
      });
      if (!s.ok && s.code === "past_event") out.push({ code: "instalments_end_after_event", target, field: "monthlyCount", detail: label, href: `${href}#instalments` });
      if (!s.ok && s.code === "deposit_too_large") out.push({ code: "deposit_too_large", target, field: "depositMinor", detail: label, href: `${href}#instalments` });
    }
  }
  // One time grid per slot pool: every time slot drawing on it has the same length and starts.
  for (const pool of g.pools.filter((x) => x.scope === "slot")) {
    const grids = new Set(
      g.links
        .filter((l) => l.pool_id === pool.id)
        .map((l) => g.products.find((x) => x.id === l.product_id))
        .filter((x) => x?.type === "time_slot")
        .map((x) => `${x!.config.slotMinutes}/${x!.config.firstStart}/${x!.config.lastStart}`),
    );
    if (grids.size > 1) out.push({ code: "slot_grid_mismatch", target: { kind: "pool", id: pool.id }, field: "pools", detail: pool.name, href: `${base}/pools#pool-${pool.id}` });
  }
  for (const pool of g.pools) {
    if (pool.capacity === 0) out.push({ code: "pool_capacity_zero", target: { kind: "pool", id: pool.id }, field: "capacity", detail: pool.name, href: `${base}/pools#pool-${pool.id}` });
  }
  return out;
}

export function isReady(problems: Problem[]): boolean {
  return problems.length === 0;
}
