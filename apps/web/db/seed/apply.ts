// Applies the sample organisers (design D1.7): idempotent upserts by stable v5 ids, through the
// same zod schemas and domain functions as Studio, only ever on organisers flagged is_sample.
// Re-running restores the sample catalogue to its defined state (extra rows a visitor added to a
// sample organiser in Studio are removed). Each organiser is one transaction.
import type pg from "pg";
import { withTx } from "@/lib/db/tx";
import { DomainError } from "@/lib/errors";
import { createBrand, getBrand, organiserInput, setEventFeePolicy, setFeePolicy, updateBrand } from "@/lib/organisers/domain";
import { slugProblem } from "@/lib/organisers/slugs";
import {
  addLineupSlot,
  loadEventGraph,
  saveEvent,
  savePool,
  saveProduct,
  saveVenue,
  setDays,
  setInstalmentPlan,
  setLadder,
  setProductPools,
  setRules,
} from "@/lib/catalogue/domain";
import { eventReadiness, type Problem } from "@/lib/catalogue/readiness";
import { dateRange, zonedToUtc } from "@/lib/catalogue/time";
import { demoOrganiserEmail } from "@/lib/auth/demo-plugin";
import { nibiru } from "./nibiru";
import { sunwave } from "./sunwave";
import type { SampleOrganiser } from "./types";
import { sid, uuidv5 } from "./uuid";

export const SAMPLE_ORGANISERS: SampleOrganiser[] = [nibiru, sunwave];

export function organiserId(o: SampleOrganiser): string {
  return sid(o.key, "organiser", o.key);
}

/** The demo organiser's user id when the seed has to create it (Better Auth ids are text). */
export const DEMO_ORGANISER_USER_ID = `seed_${uuidv5("user/demo-organiser").replaceAll("-", "").slice(0, 24)}`;

export type SeedResult = { organiser: string; id: string; events: { key: string; id: string; problems: Problem[] }[] };

async function ensureDemoOrganiser(c: pg.ClientBase, domain: string): Promise<string> {
  const email = demoOrganiserEmail(domain);
  await c.query(
    `insert into control."user" (id, name, email, "emailVerified") values ($1, 'Demo organiser', $2, true) on conflict (email) do nothing`,
    [DEMO_ORGANISER_USER_ID, email],
  );
  return (await c.query<{ id: string }>(`select id from control."user" where email = $1`, [email])).rows[0].id;
}

async function applyOrganiser(c: pg.ClientBase, o: SampleOrganiser, opts: { demoDomain: string; now: Date }): Promise<SeedResult> {
  const id = organiserId(o);
  const org = organiserInput.parse({ name: o.name, slug: o.slug });
  const bad = slugProblem(org.slug);
  if (bad) throw new DomainError(bad, `${o.key}.slug`);
  const up = await c.query(
    `insert into control.organisers (id, slug, name, is_sample, onboarding_state) values ($1, $2, $3, true, 'done')
     on conflict (id) do update set slug = excluded.slug, name = excluded.name, onboarding_state = 'done'
     where control.organisers.is_sample returning id`,
    [id, org.slug, org.name],
  );
  if (!up.rowCount) throw new Error(`organiser ${id} exists and is not a sample organiser: refusing to touch it`);

  // Team: the demo organiser as the only owner; pending invitations are cleared.
  await c.query("delete from control.invitations");
  if (o.owner === "demo-organiser") {
    const userId = await ensureDemoOrganiser(c, opts.demoDomain);
    await c.query("delete from control.memberships where organiser_id = $1 and user_id <> $2", [id, userId]);
    await c.query(
      `insert into control.memberships (organiser_id, user_id, role) values ($1, $2, 'owner')
       on conflict (organiser_id, user_id) do update set role = 'owner', scope_type = 'organiser', scope_id = null`,
      [id, userId],
    );
  } else {
    await c.query("delete from control.memberships where organiser_id = $1", [id]);
  }
  await setFeePolicy(c, "owner", o.feePolicy);

  const brandIds = new Map(o.brands.map((b) => [b.key, sid(o.key, "brand", b.key)]));
  const venueIds = new Map(o.venues.map((v) => [v.key, sid(o.key, "venue", v.key)]));
  const eventIds = new Map(o.events.map((e) => [e.key, sid(o.key, "event", e.key)]));
  // Rows a visitor added to the sample organiser go first (their events may use seeded venues).
  await c.query("delete from control.events where not (id = any($1::uuid[]))", [[...eventIds.values()]]);
  await c.query("delete from control.venues where not (id = any($1::uuid[]))", [[...venueIds.values()]]);
  await c.query("delete from control.brands where not (id = any($1::uuid[]))", [[...brandIds.values()]]);
  await c.query("delete from control.brand_hostnames");

  for (const b of o.brands) {
    const bid = brandIds.get(b.key)!;
    const { key: _key, ...input } = b; // eslint-disable-line @typescript-eslint/no-unused-vars
    if (await getBrand(c, bid)) await updateBrand(c, id, bid, input);
    else await createBrand(c, id, input, bid);
  }
  for (const v of o.venues) await saveVenue(c, v, venueIds.get(v.key));

  const events: SeedResult["events"] = [];
  for (const e of o.events) {
    const eid = eventIds.get(e.key)!;
    const tz = e.timezone ?? "Europe/Bucharest";
    const ek = (kind: string, key: string) => sid(o.key, `${e.key}/${kind}`, key);
    await saveEvent(
      c,
      {
        brandId: brandIds.get(e.brand)!,
        venueId: venueIds.get(e.venue)!,
        slug: e.slug,
        name: e.name,
        description: e.description,
        timezone: tz,
        startsAt: zonedToUtc(e.starts, tz)!,
        endsAt: zonedToUtc(e.ends, tz)!,
      },
      eid,
    );
    await c.query("update control.events set status = 'draft', edition_of = null where id = $1 and status <> 'published'", [eid]);
    await setRules(c, eid, e.rules ?? {});
    await setEventFeePolicy(c, "owner", eid, null);
    const dates = dateRange(e.days.from, e.days.to);
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
      await addLineupSlot(c, eid, { dayId: day.id, artist: l.artist, stage: l.stage, starts: l.starts, sort: i }, ek("lineup", String(i)));
    }
    const poolIds = new Map(e.pools.map((p) => [p.key, ek("pool", p.key)]));
    const productIds = new Map(e.products.map((p) => [p.key, ek("product", p.key)]));
    await c.query("delete from control.capacity_pools where event_id = $1 and not (id = any($2::uuid[]))", [eid, [...poolIds.values()]]);
    await c.query("delete from control.products where event_id = $1 and not (id = any($2::uuid[]))", [eid, [...productIds.values()]]);
    for (const p of e.pools) await savePool(c, eid, { name: p.name, capacity: p.capacity, scope: p.scope }, poolIds.get(p.key));
    for (const p of e.products) {
      const { key, pools, ladder, plan, ...product } = p;
      const pid = productIds.get(key)!;
      await saveProduct(c, eid, product, pid);
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
      await setInstalmentPlan(c, pid, plan ?? null, opts.now);
    }
    const g = await loadEventGraph(c, eid);
    events.push({ key: e.key, id: eid, problems: eventReadiness(g!, opts.now) });
  }
  return { organiser: o.key, id, events };
}

export async function applySeed(
  pool: Pick<pg.Pool, "connect">,
  opts: { demoDomain: string; now?: Date; organisers?: SampleOrganiser[] },
): Promise<SeedResult[]> {
  const now = opts.now ?? new Date();
  const out: SeedResult[] = [];
  for (const o of opts.organisers ?? SAMPLE_ORGANISERS) {
    out.push(
      await withTx(pool, { organiserId: organiserId(o) }, async (c) => {
        const r = await applyOrganiser(c, o, { demoDomain: opts.demoDomain, now });
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
