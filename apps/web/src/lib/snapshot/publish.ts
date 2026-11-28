// Publishing sale snapshots (design D3.1): a hook that runs inside step 2's publish transaction,
// after the storefront version is written. For every event the storefront sells it checks
// readiness, checks the new capacities against what the core reports as sold or held, compiles
// the snapshot and makes it live; nothing is committed if any event fails. Events whose sale terms
// did not change keep their live version (a design-only publish writes no new snapshot).
import { randomUUID } from "node:crypto";
import type { ClientBase } from "pg";
import { loadEventGraphs, type EventGraph } from "../catalogue/domain";
import { eventReadiness } from "../catalogue/readiness";
import { CURRENT_ORG } from "../organisers/domain";
import { PublishBlockedError, type PublishHook } from "../storefront/domain";
import type { PublishProblem } from "../storefront/publish-checks";
import { referencedEvents } from "../storefront/schema";
import { buildSnapshot, compileTerms, sha256Hex, termsHash, type CompileExtras } from "./compile";
import type { SaleSnapshot, SaleTerms } from "./types";

/** Units sold or held per pool instance, per event (from the core). Throws when it cannot tell. */
export type UsageSource = (eventIds: string[]) => Promise<Map<string, Record<string, number>>>;

/**
 * Usage for a brand's first publish, before any of its events can have been sold (the seed of a
 * fresh database). Never used by Studio, which always asks the core.
 */
export const noSalesBeforeFirstPublish: UsageSource = async (ids) => new Map(ids.map((id) => [id, {}]));

export type CompiledEvent = { eventId: string; version: number; created: boolean; sha256: string };

/** Loads the venue and sample flag the compiler needs, for a set of graphs (two queries). */
export async function compileExtras(c: ClientBase, graphs: EventGraph[]): Promise<Map<string, CompileExtras>> {
  const venueIds = [...new Set(graphs.map((g) => g.event.venue_id).filter((v): v is string => !!v))];
  const venues = venueIds.length
    ? (await c.query<{ id: string; name: string; address: string; lat: number | null; lng: number | null }>("select id, name, address, lat, lng from control.venues where id = any($1::uuid[])", [venueIds])).rows
    : [];
  const org = (await c.query<{ id: string; is_sample: boolean }>(`select id::text, is_sample from control.organisers where id = ${CURRENT_ORG}`)).rows[0];
  if (!org) throw new Error("compileExtras: no organiser in scope");
  return new Map(
    graphs.map((g) => {
      const v = venues.find((x) => x.id === g.event.venue_id) ?? null;
      return [g.event.id, { venue: v && { name: v.name, address: v.address, lat: v.lat, lng: v.lng }, isSample: org.is_sample, organiserId: org.id }];
    }),
  );
}

/** Readiness problems of the sold events, in the builder's problem shape (each with its Studio link). */
export function readinessProblems(graphs: EventGraph[], now: Date): PublishProblem[] {
  return graphs.flatMap((g) =>
    eventReadiness(g, now).map((p) => ({
      code: "event_not_ready" as const,
      path: `events.${g.event.id}`,
      detail: p.code,
      reasonDetail: p.detail,
      event: g.event.name?.en || g.event.name?.ro || g.event.slug,
      href: p.href,
    })),
  );
}

/** Pool instances whose new capacity is below what is sold or held, or that would disappear with usage. */
export function capacityProblems(g: EventGraph, terms: SaleTerms, used: Record<string, number>): PublishProblem[] {
  const out: PublishProblem[] = [];
  const instances = new Map(terms.pools.flatMap((p) => p.instances.map((i) => [i.instanceId, { pool: p, capacity: i.capacity }] as const)));
  for (const [instanceId, n] of Object.entries(used)) {
    if (!(n > 0)) continue;
    const poolId = instanceId.split("@")[0];
    const href = `/studio/events/${g.event.id}/pools#pool-${poolId}`;
    const hit = instances.get(instanceId);
    const label = (name: string) => (instanceId.includes("@") ? `${name} (${instanceId.split("@")[1]})` : name);
    if (!hit) {
      const name = g.pools.find((p) => p.id === poolId)?.name ?? poolId;
      out.push({ code: "removed_with_usage", path: `events.${g.event.id}.pools.${poolId}`, pool: label(name), used: n, capacity: 0, href });
    } else if (hit.capacity < n) {
      out.push({ code: "capacity_below_usage", path: `events.${g.event.id}.pools.${poolId}`, pool: label(hit.pool.name), used: n, capacity: hit.capacity, href });
    }
  }
  return out;
}

/** Writes snapshot `version` of an event and makes it live (inside the publish transaction). */
async function insertSnapshot(c: ClientBase, terms: SaleTerms, p: { version: number; userId: string | null; storefrontVersionId: string; now: Date }): Promise<SaleSnapshot & { sha256: string }> {
  const doc = buildSnapshot(terms, { snapshotId: randomUUID(), version: p.version, publishedAt: p.now.toISOString() });
  const text = JSON.stringify(doc);
  const sha = sha256Hex(text);
  await c.query(
    `insert into control.sale_snapshots (id, organiser_id, event_id, version, schema_version, document_text, sha256, terms_sha256, storefront_version_id, published_by, published_at)
     values ($1, ${CURRENT_ORG}, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
    [doc.snapshotId, doc.eventId, doc.version, doc.schemaVersion, text, sha, termsHash(terms), p.storefrontVersionId, p.userId, p.now],
  );
  await c.query("update control.events set live_snapshot_version = $2, status = 'published', updated_at = now() where id = $1", [doc.eventId, doc.version]);
  return { ...doc, sha256: sha };
}

/**
 * The step 3 publish hook. `usage` is asked once per publish; when it fails, the publish is refused
 * with a retry message rather than made live unchecked. `onCompiled` receives what was made live,
 * for the activation calls after commit.
 */
export function snapshotHook(opts: { usage: UsageSource; userId: string | null; now?: () => Date; onCompiled?: (events: CompiledEvent[]) => void }): PublishHook {
  return async (c, ctx) => {
    const now = opts.now?.() ?? new Date();
    const ids = [...new Set(referencedEvents(ctx.document))].sort();
    if (!ids.length) {
      opts.onCompiled?.([]);
      return;
    }
    // Lock the events (in id order) so concurrent publishes of them serialise: versions are never
    // duplicated or skipped. The storefront row is already locked by the publish.
    await c.query("select id from control.events where id = any($1::uuid[]) order by id for update", [ids]);
    const graphs = await loadEventGraphs(c, ids);
    const notReady = readinessProblems(graphs, now);
    if (notReady.length) throw new PublishBlockedError(notReady);
    const extras = await compileExtras(c, graphs);
    const compiled = graphs.map((g) => ({ g, terms: compileTerms(g, extras.get(g.event.id)!) }));
    let usage: Map<string, Record<string, number>>;
    try {
      usage = await opts.usage(ids);
    } catch {
      throw new PublishBlockedError([{ code: "core_unreachable", path: "" }]);
    }
    const tooSmall = compiled.flatMap(({ g, terms }) => capacityProblems(g, terms, usage.get(g.event.id) ?? {}));
    if (tooSmall.length) throw new PublishBlockedError(tooSmall);

    const live = new Map(
      (
        await c.query<{ event_id: string; version: number; terms_sha256: string; sha256: string }>(
          `select s.event_id::text, s.version, s.terms_sha256, s.sha256 from control.sale_snapshots s
           join control.events e on e.id = s.event_id and e.live_snapshot_version = s.version where s.event_id = any($1::uuid[])`,
          [ids],
        )
      ).rows.map((r) => [r.event_id, r]),
    );
    const last = new Map(
      (await c.query<{ event_id: string; n: number }>("select event_id::text, max(version)::int as n from control.sale_snapshots where event_id = any($1::uuid[]) group by event_id", [ids])).rows.map((r) => [r.event_id, r.n]),
    );
    const out: CompiledEvent[] = [];
    for (const { g, terms } of compiled) {
      const cur = live.get(g.event.id);
      if (cur && cur.terms_sha256 === termsHash(terms)) {
        out.push({ eventId: g.event.id, version: cur.version, created: false, sha256: cur.sha256 });
        continue;
      }
      const version = (last.get(g.event.id) ?? 0) + 1;
      const doc = await insertSnapshot(c, terms, { version, userId: opts.userId, storefrontVersionId: ctx.versionId, now });
      out.push({ eventId: g.event.id, version, created: true, sha256: doc.sha256 });
    }
    opts.onCompiled?.(out);
  };
}

export type PublicationState = { liveVersion: number | null; publishedAt: Date | null; changed: boolean };

/** For Studio: an event's live snapshot version and time, and whether its catalogue has changed since. */
export async function publicationState(c: ClientBase, eventId: string): Promise<PublicationState> {
  const row = (
    await c.query<{ version: number | null; published_at: Date | null; terms_sha256: string | null }>(
      `select e.live_snapshot_version as version, s.published_at, s.terms_sha256 from control.events e
       left join control.sale_snapshots s on s.event_id = e.id and s.version = e.live_snapshot_version where e.id = $1`,
      [eventId],
    )
  ).rows[0];
  if (!row || row.version === null) return { liveVersion: null, publishedAt: null, changed: false };
  const graphs = await loadEventGraphs(c, [eventId]);
  if (!graphs.length) return { liveVersion: row.version, publishedAt: row.published_at, changed: false };
  const terms = compileTerms(graphs[0], (await compileExtras(c, graphs)).get(eventId)!);
  return { liveVersion: row.version, publishedAt: row.published_at, changed: termsHash(terms) !== row.terms_sha256 };
}
