// Applies the sample organisers (design D1.7): idempotent upserts by stable v5 ids, through the
// same zod schemas and domain functions as Studio, only ever on organisers flagged is_sample.
// Re-running restores the sample catalogue to its defined state: it updates seeded draft rows and
// removes the drafts a visitor added. It never deletes or changes a published (or archived) event or
// anything that belongs to one, so sales made under a snapshot (steps 3 and 4) are never touched.
// The organiser registry row is written as the connecting owner (the sample flag is not writable
// by the web role); everything else runs inside withTx as eps_web, scoped to the sample organiser.
import type pg from "pg";
import { withTx } from "@/lib/db/tx";
import { DomainError } from "@/lib/errors";
import { createBrand, getBrand, organiserInput, setEventFeePolicy, setFeePolicy, updateBrand } from "@/lib/organisers/domain";
import { slugProblem } from "@/lib/organisers/slugs";
import {
  addLineupSlot,
  createEvent,
  createPool,
  createProduct,
  createVenue,
  loadEventGraph,
  setDays,
  setInstalmentPlan,
  setLadder,
  setProductPools,
  setRules,
  updateEvent,
  updatePool,
  updateProduct,
  updateVenue,
} from "@/lib/catalogue/domain";
import { eventReadiness, type Problem } from "@/lib/catalogue/readiness";
import { dateRange, zonedToUtc } from "@/lib/catalogue/time";
import { maxMonthlyPayments } from "@/lib/pricing/instalments";
import { demoOrganiserEmail } from "@/lib/auth/demo-plugin";
import { ensureStorefront, getStorefront, publishDraft, saveDraft } from "@/lib/storefront/domain";
import type { StorefrontDocument } from "@/lib/storefront/schema";
import { noSalesBeforeFirstPublish, snapshotHook } from "@/lib/snapshot/publish";
import { buildTemplate } from "@/lib/storefront/templates";
import { ensureTiles, mapTilesForDocument, type TileFetcher } from "@/lib/storefront/maps";
import { nibiru } from "./nibiru";
import { sunwave } from "./sunwave";
import type { SampleOrganiser, SampleStorefront } from "./types";
import { sid, uuidv5 } from "./uuid";

export const SAMPLE_ORGANISERS: SampleOrganiser[] = [nibiru, sunwave];

export function organiserId(o: SampleOrganiser): string {
  return sid(o.key, "organiser", o.key);
}

/** The demo organiser's user id when the seed has to create it (Better Auth ids are text). */
export const DEMO_ORGANISER_USER_ID = `seed_${uuidv5("user/demo-organiser").replaceAll("-", "").slice(0, 24)}`;

export type SeedResult = { organiser: string; id: string; events: { key: string; id: string; skipped?: "published"; problems: Problem[] }[] };

function canonical(v: unknown): string {
  if (Array.isArray(v)) return `[${v.map(canonical).join(",")}]`;
  if (v && typeof v === "object") return `{${Object.keys(v).sort().map((k) => `${JSON.stringify(k)}:${canonical((v as Record<string, unknown>)[k])}`).join(",")}}`;
  return JSON.stringify(v);
}

/** A sample shop document with stable ids, so re-running the seed writes the same document. */
export function sampleStorefront(spec: SampleStorefront, brandName: string, languages: ("ro" | "en")[], event: { id: string; slug: string; name: { ro: string; en: string } } | null): StorefrontDocument {
  const doc = buildTemplate(spec.template, { brandName, languages, event });
  doc.tokens.logo = spec.logo;
  let n = 0;
  for (const p of doc.pages) {
    p.id = `p${n++}`;
    for (const sct of p.sections) {
      sct.id = `${sct.type.replace("_", "")}-${n++}`;
      for (const b of sct.blocks) b.id = `b${n++}`;
    }
  }
  return doc;
}

/** The demo organiser user (by email; created with a stable id when missing). Runs as the owner. */
async function ensureDemoOrganiser(c: pg.ClientBase, domain: string): Promise<string> {
  const email = demoOrganiserEmail(domain);
  const found = await c.query<{ id: string }>(`select id from control."user" where email = $1`, [email]);
  if (found.rows[0]) return found.rows[0].id;
  // The stable id may exist under an older demo domain: move it to the current address.
  const r = await c.query<{ id: string }>(
    `insert into control."user" (id, name, email, "emailVerified") values ($1, 'Demo organiser', $2, true)
     on conflict (id) do update set email = excluded.email, "updatedAt" = now() returning id`,
    [DEMO_ORGANISER_USER_ID, email],
  );
  return r.rows[0].id;
}

/** The registry row, as the owner: created or renamed, never taken over from a real organiser. */
async function ensureOrganiserRow(c: pg.ClientBase, o: SampleOrganiser, id: string) {
  const org = organiserInput.parse({ name: o.name, slug: o.slug });
  const bad = slugProblem(org.slug, { sample: true });
  if (bad) throw new DomainError(bad, `${o.key}.slug`);
  const up = await c.query(
    `insert into control.organisers (id, slug, name, is_sample, onboarding_state) values ($1, $2, $3, true, 'done')
     on conflict (id) do update set name = excluded.name, onboarding_state = 'done'
     where control.organisers.is_sample returning id`,
    [id, org.slug, org.name],
  );
  if (!up.rowCount) throw new Error(`organiser ${id} exists and is not a sample organiser: refusing to touch it`);
}

async function ids(c: pg.ClientBase, table: string, where = "true", args: unknown[] = []): Promise<Set<string>> {
  return new Set((await c.query<{ id: string }>(`select id::text from control.${table} where ${where}`, args)).rows.map((r) => r.id));
}

async function applyCatalogue(c: pg.ClientBase, o: SampleOrganiser, opts: { now: Date; userId: string | null; tileFetcher?: TileFetcher }): Promise<SeedResult> {
  const id = organiserId(o);
  // Team: the demo organiser as the only owner; pending invitations are cleared.
  await c.query("delete from control.invitations");
  if (opts.userId) {
    await c.query("delete from control.memberships where organiser_id = $1 and user_id <> $2", [id, opts.userId]);
    await c.query(
      `insert into control.memberships (organiser_id, user_id, role) values ($1, $2, 'owner')
       on conflict (organiser_id, user_id) do update set role = 'owner', scope_type = 'organiser', scope_id = null`,
      [id, opts.userId],
    );
  } else {
    await c.query("delete from control.memberships where organiser_id = $1", [id]);
  }
  await setFeePolicy(c, "owner", o.feePolicy);

  const brandIds = new Map(o.brands.map((b) => [b.key, sid(o.key, "brand", b.key)]));
  const venueIds = new Map(o.venues.map((v) => [v.key, sid(o.key, "venue", v.key)]));
  const eventIds = new Map(o.events.map((e) => [e.key, sid(o.key, "event", e.key)]));
  const status = new Map(
    (await c.query<{ id: string; status: string }>("select id::text, status from control.events")).rows.map((r) => [r.id, r.status]),
  );
  // Visitor-added drafts go; published or archived events (and their brands and venues) stay.
  await c.query("delete from control.events where status = 'draft' and not (id = any($1::uuid[]))", [[...eventIds.values()]]);
  await c.query("delete from control.brand_hostnames");

  const existingBrands = await ids(c, "brands");
  for (const b of o.brands) {
    const bid = brandIds.get(b.key)!;
    const { key: _key, storefront: _sf, ...input } = b; // eslint-disable-line @typescript-eslint/no-unused-vars
    if (existingBrands.has(bid) && (await getBrand(c, bid))) await updateBrand(c, id, bid, input);
    else await createBrand(c, id, input, bid, { sample: true });
  }
  const existingVenues = await ids(c, "venues");
  for (const v of o.venues) {
    const vid = venueIds.get(v.key)!;
    if (existingVenues.has(vid)) await updateVenue(c, vid, v);
    else await createVenue(c, v, vid);
  }

  const events: SeedResult["events"] = [];
  for (const e of o.events) {
    const eid = eventIds.get(e.key)!;
    if (status.has(eid) && status.get(eid) !== "draft") {
      events.push({ key: e.key, id: eid, skipped: "published", problems: [] });
      continue; // published: its catalogue is frozen in a snapshot and may have sales
    }
    const tz = e.timezone ?? "Europe/Bucharest";
    const ek = (kind: string, key: string) => sid(o.key, `${e.key}/${kind}`, key);
    const input = {
      brandId: brandIds.get(e.brand)!,
      venueId: venueIds.get(e.venue)!,
      slug: e.slug,
      name: e.name,
      description: e.description,
      timezone: tz,
      startsAt: zonedToUtc(e.starts, tz)!,
      endsAt: zonedToUtc(e.ends, tz)!,
    };
    const dates = dateRange(e.days.from, e.days.to);
    if (status.has(eid)) {
      // Days a visitor added outside the seeded dates would block moving the dates back.
      await c.query("delete from control.event_days where event_id = $1 and not (day::text = any($2))", [eid, dates]);
      await updateEvent(c, eid, input);
    } else {
      await createEvent(c, input, eid);
    }
    await c.query("update control.events set edition_of = null where id = $1", [eid]);
    await setRules(c, eid, e.rules ?? {});
    await setEventFeePolicy(c, "owner", eid, null);
    const days = await setDays(
      c,
      eid,
      dates.map((day) => ({ day, opens: e.days.opens, closes: e.days.closes })),
      Object.fromEntries(dates.map((d) => [d, ek("day", d)])),
    );
    await c.query("delete from control.lineup_slots where event_id = $1", [eid]);
    for (const [i, l] of (e.lineup ?? []).entries()) {
      const day = days.find((d) => d.day === l.day);
      if (!day) throw new Error(`${e.key}: line-up day ${l.day} is not an event day`);
      const slot = await addLineupSlot(c, eid, { dayId: day.id, artist: l.artist, stage: l.stage, starts: l.starts, sort: i }, ek("lineup", String(i)));
      // Sample data: every seeded artist is a placeholder, labelled as a sample artist by the shop.
      await c.query("update control.lineup_slots set sample = true where id = $1", [slot.id]);
    }
    const poolIds = new Map(e.pools.map((p) => [p.key, ek("pool", p.key)]));
    const productIds = new Map(e.products.map((p) => [p.key, ek("product", p.key)]));
    await c.query("delete from control.capacity_pools where event_id = $1 and not (id = any($2::uuid[]))", [eid, [...poolIds.values()]]);
    await c.query("delete from control.products where event_id = $1 and not (id = any($2::uuid[]))", [eid, [...productIds.values()]]);
    const existingPools = await ids(c, "capacity_pools", "event_id = $1", [eid]);
    for (const p of e.pools) {
      const pid = poolIds.get(p.key)!;
      const input = { name: p.name, capacity: p.capacity, scope: p.scope };
      if (existingPools.has(pid)) await updatePool(c, eid, pid, input);
      else await createPool(c, eid, input, pid);
    }
    const existingProducts = await ids(c, "products", "event_id = $1", [eid]);
    for (const p of e.products) {
      const { key, pools, ladder, plan, ...product } = p;
      const pid = productIds.get(key)!;
      if (existingProducts.has(pid)) await updateProduct(c, eid, pid, product);
      else await createProduct(c, eid, product, pid);
      await setProductPools(
        c,
        pid,
        pools.map((k) => {
          const x = poolIds.get(k);
          if (!x) throw new Error(`${e.key}/${key}: unknown pool ${k}`);
          return x;
        }),
      );
      await setLadder(c, pid, ladder);
      // A plan's count is a maximum capped per buyer: the seed stores what still fits for a buyer
      // today (the Studio check), so re-running the seed later never fails on a shorter season.
      let stored = plan ?? null;
      if (plan) {
        const fits = maxMonthlyPayments(opts.now, plan.dayOfMonth ? Number(plan.dayOfMonth) : 1, input.startsAt, tz);
        const want = Number(plan.monthlyCount);
        stored = fits >= 1 ? { ...plan, monthlyCount: Math.min(want, fits) } : null;
      }
      await setInstalmentPlan(c, pid, stored, opts.now);
    }
    const g = await loadEventGraph(c, eid);
    events.push({ key: e.key, id: eid, problems: eventReadiness(g!, opts.now) });
  }
  // Sample shops (step 2): each brand's draft is reset to its sample document; a brand with no
  // published version yet gets version 1. Published versions are never changed.
  for (const b of o.brands) {
    if (!b.storefront) continue;
    const bid = brandIds.get(b.key)!;
    const ev = b.storefront.event ? await loadEventGraph(c, eventIds.get(b.storefront.event)!) : null;
    const doc = sampleStorefront(b.storefront, b.name, b.languages as ("ro" | "en")[], ev ? { id: ev.event.id, slug: ev.event.slug, name: ev.event.name } : null);
    const sf = await ensureStorefront(c, bid, opts.userId, { document: doc, source: "seed" });
    const fresh = await getStorefront(c, sf.id);
    let revision = fresh!.draft_revision;
    if (canonical(fresh!.document) !== canonical(doc)) revision = (await saveDraft(c, sf.id, { revision, document: doc })).revision;
    // The first publish also compiles the sale snapshots of the events the shop sells (step 3);
    // a brand's first publish has no sales to check against.
    if (!fresh!.live_version_id) {
      await publishDraft(c, { storefrontId: sf.id, revision, userId: opts.userId }, [snapshotHook({ usage: noSalesBeforeFirstPublish, userId: opts.userId, now: () => opts.now })]);
    }
    // Static map tiles for the sample shops' map sections (best effort; the map shows a link without).
    await ensureTiles(c, await mapTilesForDocument(c, doc), opts.tileFetcher, 3);
  }

  // Venues and brands a visitor added go only when nothing uses them any more.
  await c.query("delete from control.venues v where not (id = any($1::uuid[])) and not exists (select 1 from control.events e where e.venue_id = v.id)", [
    [...venueIds.values()],
  ]);
  await c.query(
    `delete from control.brands b where not (id = any($1::uuid[])) and not exists (select 1 from control.events e where e.brand_id = b.id)
     and not exists (select 1 from control.storefronts s where s.brand_id = b.id and s.live_version_id is not null)`,
    [
    [...brandIds.values()],
  ]);
  return { organiser: o.key, id, events };
}

/**
 * `pool` must connect as the database owner (the migrating role): the registry row and the demo
 * user are written directly, the catalogue through withTx as eps_web.
 */
export async function applySeed(
  pool: Pick<pg.Pool, "connect">,
  opts: { demoDomain: string; now?: Date; organisers?: SampleOrganiser[] },
): Promise<SeedResult[]> {
  const now = opts.now ?? new Date();
  const out: SeedResult[] = [];
  for (const o of opts.organisers ?? SAMPLE_ORGANISERS) {
    const id = organiserId(o);
    const owner = await pool.connect();
    let userId: string | null = null;
    try {
      await ensureOrganiserRow(owner, o, id);
      if (o.owner === "demo-organiser") userId = await ensureDemoOrganiser(owner, opts.demoDomain);
    } finally {
      owner.release();
    }
    out.push(
      await withTx(pool, { organiserId: id }, async (c) => {
        const r = await applyCatalogue(c, o, { now, userId });
        const notReady = r.events.filter((e) => e.problems.length);
        if (notReady.length) {
          throw new Error(`seed: events not ready: ${notReady.map((e) => `${e.key} (${e.problems.map((p) => p.code).join(", ")})`).join("; ")}`);
        }
        return r;
      }),
    );
  }
  return out;
}
