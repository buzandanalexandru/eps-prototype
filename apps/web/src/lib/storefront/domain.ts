// Storefront operations (design D2.1, D2.4, D2.5): one storefront per brand, its draft (saved with a
// revision check), immutable published versions, restore as a new version, and preview links. Every
// function runs on a client inside withTx, so row-level security limits it to one organiser.
import { createHash, randomBytes } from "node:crypto";
import type { ClientBase } from "pg";
import { DomainError } from "../errors";
import { CURRENT_ORG, getBrand, type BrandRow } from "../organisers/domain";
import { ownedAssets } from "./assets";
import { publishProblems, type PublishProblem } from "./publish-checks";
import { parseDocument, referencedAssets, referencedEvents, type StorefrontDocument } from "./schema";
import { changeTemplate, templateForBrand, type TemplateId } from "./templates";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;
const need = <T>(v: T | null | undefined): T => {
  if (!v) throw new DomainError("not_found");
  return v;
};

export class PublishBlockedError extends DomainError {
  constructor(readonly problems: PublishProblem[]) {
    super("publish_blocked");
  }
}

export class DraftConflictError extends DomainError {
  constructor(readonly currentRevision: number) {
    super("draft_conflict");
  }
}

export type StorefrontRow = {
  id: string;
  brand_id: string;
  live_version_id: string | null;
  draft_revision: number;
  draft_updated_at: Date;
  document: StorefrontDocument;
  live_number: number | null;
  live_published_at: Date | null;
};

const SF_SELECT = `select s.id, s.brand_id, s.live_version_id, d.revision as draft_revision, d.updated_at as draft_updated_at, d.document,
    l.number as live_number, l.published_at as live_published_at
  from control.storefronts s
  join control.storefront_versions d on d.storefront_id = s.id and d.state = 'draft'
  left join control.storefront_versions l on l.id = s.live_version_id`;

export async function getStorefront(c: ClientBase, id: string): Promise<StorefrontRow | null> {
  if (!UUID.test(id)) return null;
  return (await c.query<StorefrontRow>(`${SF_SELECT} where s.id = $1`, [id])).rows[0] ?? null;
}

export async function getStorefrontByBrand(c: ClientBase, brandId: string): Promise<StorefrontRow | null> {
  if (!UUID.test(brandId)) return null;
  return (await c.query<StorefrontRow>(`${SF_SELECT} where s.brand_id = $1`, [brandId])).rows[0] ?? null;
}

async function firstEvent(c: ClientBase, brandId: string) {
  return (
    await c.query<{ id: string; slug: string; name: { ro: string; en: string } }>(
      "select id, slug, name from control.events where brand_id = $1 order by starts_at, id limit 1",
      [brandId],
    )
  ).rows[0] ?? null;
}

/**
 * The brand's storefront, created on the first visit from the brand's template choice (B2), with its
 * first event if it has one. Safe under concurrent first visits (unique brand, one draft).
 */
export async function ensureStorefront(
  c: ClientBase,
  brandId: string,
  userId: string | null,
  opts: { document?: StorefrontDocument; source?: "template" | "seed" } = {},
): Promise<StorefrontRow> {
  const existing = await getStorefrontByBrand(c, brandId);
  if (existing) return existing;
  const brand = need(await getBrand(c, brandId));
  await c.query("select pg_advisory_xact_lock(hashtext('storefront:' || $1))", [brandId]);
  const again = await getStorefrontByBrand(c, brandId);
  if (again) return again;
  const doc =
    opts.document ?? templateForBrand(brand.template, { brandName: brand.name, languages: brand.languages, event: await firstEvent(c, brandId) });
  const valid = parseDocument(doc);
  if (!valid.ok) throw new DomainError("invalid", valid.path);
  const sf = await c.query<{ id: string }>(
    `insert into control.storefronts (organiser_id, brand_id) values (${CURRENT_ORG}, $1) returning id`,
    [brandId],
  );
  await c.query(
    `insert into control.storefront_versions (organiser_id, storefront_id, state, document, revision, source, created_by)
     values (${CURRENT_ORG}, $1, 'draft', $2, 1, $3, $4)`,
    [sf.rows[0].id, JSON.stringify(valid.doc), opts.source ?? "template", userId],
  );
  return need(await getStorefront(c, sf.rows[0].id));
}

/** The brand's own events among the ids (others, including other organisers', are unknown). */
export async function knownEvents(c: ClientBase, brandId: string, ids: string[]): Promise<Set<string>> {
  const valid = ids.filter((i) => UUID.test(i));
  if (!valid.length) return new Set();
  const r = await c.query<{ id: string }>("select id::text from control.events where brand_id = $1 and id = any($2::uuid[])", [brandId, valid]);
  return new Set(r.rows.map((x) => x.id));
}

/** Validates a document for saving: schema, the brand's own events, the organiser's own uploads. */
export async function validateForSave(c: ClientBase, brandId: string, raw: unknown): Promise<StorefrontDocument> {
  const parsed = parseDocument(raw);
  if (!parsed.ok) throw new DomainError(parsed.code, parsed.path);
  const doc = parsed.doc;
  const evs = referencedEvents(doc);
  const known = await knownEvents(c, brandId, evs);
  const unknown = evs.find((e) => !known.has(e));
  if (unknown) throw new DomainError("unknown_event", pathOfEvent(doc, unknown));
  const assets = referencedAssets(doc);
  const owned = await ownedAssets(c, assets);
  if (assets.some((a) => !owned.has(a))) throw new DomainError("unknown_image");
  return doc;
}

function pathOfEvent(doc: StorefrontDocument, eventId: string): string {
  for (const [pi, p] of doc.pages.entries()) {
    if (p.eventId === eventId) return `pages.${pi}.eventId`;
    for (const [si, s] of p.sections.entries()) if ((s.settings as { eventId?: string }).eventId === eventId) return `pages.${pi}.sections.${si}.settings.eventId`;
  }
  return "";
}

/**
 * Autosave (D2.4): replaces the draft only when the caller's revision is the current one, and returns
 * the new revision. An older revision (another tab saved meanwhile) is refused with the current one.
 */
export async function saveDraft(c: ClientBase, storefrontId: string, p: { revision: number; document: unknown }): Promise<{ revision: number }> {
  const sf = need(await getStorefront(c, storefrontId));
  const doc = await validateForSave(c, sf.brand_id, p.document);
  const r = await c.query<{ revision: number }>(
    `update control.storefront_versions set document = $3, revision = revision + 1, updated_at = now()
     where storefront_id = $1 and state = 'draft' and revision = $2 returning revision`,
    [storefrontId, p.revision, JSON.stringify(doc)],
  );
  if (!r.rows[0]) {
    const cur = await c.query<{ revision: number }>("select revision from control.storefront_versions where storefront_id = $1 and state = 'draft'", [storefrontId]);
    throw new DraftConflictError(need(cur.rows[0]).revision);
  }
  return r.rows[0];
}

export type PublishHookContext = {
  storefrontId: string;
  brandId: string;
  versionId: string;
  number: number;
  document: StorefrontDocument;
  restoredFrom: number | null;
};
/** Runs inside the publish transaction, after the version is written (step 3 compiles snapshots). */
export type PublishHook = (c: ClientBase, ctx: PublishHookContext) => Promise<void>;
export const publishHooks: PublishHook[] = [];

export type VersionRow = {
  id: string;
  number: number;
  state: "published" | "archived";
  revision: number;
  restored_from: number | null;
  published_at: Date;
  author: string | null;
};

async function lockStorefront(c: ClientBase, storefrontId: string): Promise<{ id: string; brand_id: string; live_version_id: string | null }> {
  if (!UUID.test(storefrontId)) throw new DomainError("not_found");
  return need((await c.query("select id, brand_id, live_version_id from control.storefronts where id = $1 for update", [storefrontId])).rows[0]);
}

async function writeVersion(
  c: ClientBase,
  sf: { id: string; brand_id: string; live_version_id: string | null },
  p: { document: StorefrontDocument; revision: number; userId: string | null; restoredFrom: number | null },
  hooks: PublishHook[],
): Promise<VersionRow> {
  const known = await knownEvents(c, sf.brand_id, referencedEvents(p.document));
  const problems = publishProblems(p.document, { knownEvents: known });
  if (problems.length) throw new PublishBlockedError(problems);
  const next = (await c.query<{ n: number }>("select coalesce(max(number), 0) + 1 as n from control.storefront_versions where storefront_id = $1", [sf.id])).rows[0].n;
  const v = await c.query<{ id: string }>(
    `insert into control.storefront_versions (organiser_id, storefront_id, number, state, document, revision, restored_from, created_by, published_at)
     values (${CURRENT_ORG}, $1, $2, 'published', $3, $4, $5, $6, now()) returning id`,
    [sf.id, next, JSON.stringify(p.document), p.revision, p.restoredFrom, p.userId],
  );
  if (sf.live_version_id) await c.query("update control.storefront_versions set state = 'archived' where id = $1 and state = 'published'", [sf.live_version_id]);
  await c.query("update control.storefronts set live_version_id = $2 where id = $1", [sf.id, v.rows[0].id]);
  for (const h of hooks) {
    await h(c, { storefrontId: sf.id, brandId: sf.brand_id, versionId: v.rows[0].id, number: next, document: p.document, restoredFrom: p.restoredFrom });
  }
  return need(await getVersion(c, sf.id, next));
}

/**
 * Publishes the draft at `revision` as a new numbered version (one transaction; D2.1). Publishing the
 * same revision again returns the version it already made (a double click publishes once).
 */
export async function publishDraft(
  c: ClientBase,
  p: { storefrontId: string; revision: number; userId: string | null },
  hooks: PublishHook[] = publishHooks,
): Promise<VersionRow & { created: boolean }> {
  const sf = await lockStorefront(c, p.storefrontId);
  const draft = need(
    (await c.query<{ document: StorefrontDocument; revision: number }>("select document, revision from control.storefront_versions where storefront_id = $1 and state = 'draft'", [sf.id])).rows[0],
  );
  // Only while the draft is still at that revision: a double click publishes once, but a publish of
  // an older revision (another tab saved since) is a conflict, never a silent no-op.
  if (draft.revision !== p.revision) throw new DraftConflictError(draft.revision);
  const done = await c.query<{ number: number }>(
    "select number from control.storefront_versions where storefront_id = $1 and state <> 'draft' and revision = $2",
    [sf.id, p.revision],
  );
  const parsed = parseDocument(draft.document);
  if (done.rows[0]) {
    // The look is already live, but the catalogue may have changed since: the hooks (step 3's sale
    // snapshots) run again, so publishing a catalogue-only change makes it live.
    const v = need(await getVersion(c, sf.id, done.rows[0].number));
    if (parsed.ok) {
      for (const h of hooks) await h(c, { storefrontId: sf.id, brandId: sf.brand_id, versionId: v.id, number: v.number, document: parsed.doc, restoredFrom: v.restored_from });
    }
    return { ...v, created: false };
  }
  if (!parsed.ok) throw new PublishBlockedError([{ code: "invalid", path: parsed.path, detail: parsed.code }]);
  return { ...(await writeVersion(c, sf, { document: parsed.doc, revision: draft.revision, userId: p.userId, restoredFrom: null }, hooks)), created: true };
}

/**
 * Rollback (D2.1): publishes version `number`'s document as a new version marked restored_from, and
 * makes the draft equal to it; history is never rewritten.
 */
export async function restoreVersion(
  c: ClientBase,
  p: { storefrontId: string; number: number; revision: number; userId: string | null },
  hooks: PublishHook[] = publishHooks,
): Promise<VersionRow> {
  const sf = await lockStorefront(c, p.storefrontId);
  if (!Number.isInteger(p.number) || p.number < 1) throw new DomainError("not_found");
  // The restore replaces the draft: only the draft the organiser is looking at, never newer work.
  const cur = need((await c.query<{ revision: number }>("select revision from control.storefront_versions where storefront_id = $1 and state = 'draft'", [sf.id])).rows[0]);
  if (cur.revision !== p.revision) throw new DraftConflictError(cur.revision);
  const old = need(
    (await c.query<{ document: StorefrontDocument }>("select document from control.storefront_versions where storefront_id = $1 and number = $2", [sf.id, p.number])).rows[0],
  );
  const parsed = parseDocument(old.document);
  if (!parsed.ok) throw new PublishBlockedError([{ code: "invalid", path: parsed.path, detail: parsed.code }]);
  const d = await c.query<{ revision: number }>(
    `update control.storefront_versions set document = $2, revision = revision + 1, updated_at = now()
     where storefront_id = $1 and state = 'draft' and revision = $3 returning revision`,
    [sf.id, JSON.stringify(parsed.doc), p.revision],
  );
  return writeVersion(c, sf, { document: parsed.doc, revision: need(d.rows[0]).revision, userId: p.userId, restoredFrom: p.number }, hooks);
}

const VERSION_COLS = `v.id, v.number, v.state, v.revision, v.restored_from, v.published_at, u.name as author`;

async function getVersion(c: ClientBase, storefrontId: string, number: number): Promise<VersionRow | null> {
  return (
    await c.query<VersionRow>(
      `select ${VERSION_COLS} from control.storefront_versions v left join control."user" u on u.id = v.created_by
       where v.storefront_id = $1 and v.number = $2`,
      [storefrontId, number],
    )
  ).rows[0] ?? null;
}

/** Published versions, newest first (bounded page). */
export async function listVersions(c: ClientBase, storefrontId: string, p: { limit?: number; offset?: number } = {}): Promise<VersionRow[]> {
  if (!UUID.test(storefrontId)) return [];
  const limit = Math.min(Math.max(p.limit ?? 20, 1), 100);
  return (
    await c.query<VersionRow>(
      `select ${VERSION_COLS} from control.storefront_versions v left join control."user" u on u.id = v.created_by
       where v.storefront_id = $1 and v.state <> 'draft' order by v.number desc limit $2 offset $3`,
      [storefrontId, limit, Math.max(p.offset ?? 0, 0)],
    )
  ).rows;
}

export async function getVersionDocument(c: ClientBase, storefrontId: string, number: number): Promise<StorefrontDocument | null> {
  if (!UUID.test(storefrontId)) return null;
  return (await c.query("select document from control.storefront_versions where storefront_id = $1 and number = $2", [storefrontId, number])).rows[0]?.document ?? null;
}

/** "Change template" (B2): the new template's pages, the brand's tokens kept; saved like an edit. */
export async function applyTemplate(c: ClientBase, storefrontId: string, p: { revision: number; template: TemplateId }): Promise<{ revision: number; document: StorefrontDocument }> {
  const sf = need(await getStorefront(c, storefrontId));
  const brand = need(await getBrand(c, sf.brand_id));
  const doc = changeTemplate(sf.document, p.template, { brandName: brand.name, languages: brand.languages, event: await firstEvent(c, brand.id) });
  const r = await saveDraft(c, storefrontId, { revision: p.revision, document: doc });
  return { revision: r.revision, document: doc };
}

// ---------------------------------------------------------------- preview links

export const PREVIEW_LINK_DAYS = 7;
/** Active (unexpired, unrevoked) preview links per storefront. */
export const MAX_ACTIVE_PREVIEW_LINKS = 20;
const TOKEN = /^[A-Za-z0-9_-]{22}$/;

export function hashPreviewToken(token: string): string {
  return createHash("sha256").update(token, "utf8").digest("hex");
}

/** A new link: 128 random bits in the URL, only their SHA-256 stored. The token is shown once. */
export async function createPreviewLink(c: ClientBase, storefrontId: string, userId: string | null, days = PREVIEW_LINK_DAYS): Promise<{ id: string; token: string; expiresAt: Date }> {
  need(await lockStorefront(c, storefrontId));
  const active = await c.query<{ n: number }>(
    "select count(*)::int as n from control.preview_links where storefront_id = $1 and revoked_at is null and expires_at > now()",
    [storefrontId],
  );
  if (active.rows[0].n >= MAX_ACTIVE_PREVIEW_LINKS) throw new DomainError("too_many_links", undefined, { max: MAX_ACTIVE_PREVIEW_LINKS });
  const token = randomBytes(16).toString("base64url");
  const r = await c.query<{ id: string; expires_at: Date }>(
    `insert into control.preview_links (organiser_id, storefront_id, token_hash, created_by, expires_at)
     values (${CURRENT_ORG}, $1, $2, $3, now() + make_interval(days => $4)) returning id, expires_at`,
    [storefrontId, hashPreviewToken(token), userId, Math.min(Math.max(Math.trunc(days), 1), 30)],
  );
  return { id: r.rows[0].id, token, expiresAt: r.rows[0].expires_at };
}

export type PreviewLinkRow = { id: string; created_at: Date; expires_at: Date; revoked_at: Date | null; expired: boolean; author: string | null };

export async function listPreviewLinks(c: ClientBase, storefrontId: string): Promise<PreviewLinkRow[]> {
  if (!UUID.test(storefrontId)) return [];
  return (
    await c.query<PreviewLinkRow>(
      `select l.id, l.created_at, l.expires_at, l.revoked_at, l.expires_at <= now() as expired, u.name as author from control.preview_links l
       left join control."user" u on u.id = l.created_by where l.storefront_id = $1 and l.revoked_at is null and l.expires_at > now()
       union all
       (select l.id, l.created_at, l.expires_at, l.revoked_at, true, u.name from control.preview_links l
        left join control."user" u on u.id = l.created_by where l.storefront_id = $1 and (l.revoked_at is not null or l.expires_at <= now())
        order by l.created_at desc limit 10)
       order by 2 desc`,
      [storefrontId],
    )
  ).rows;
}

export async function revokePreviewLink(c: ClientBase, storefrontId: string, linkId: string): Promise<void> {
  if (!UUID.test(storefrontId) || !UUID.test(linkId)) throw new DomainError("not_found");
  const r = await c.query("update control.preview_links set revoked_at = coalesce(revoked_at, now()) where id = $1 and storefront_id = $2", [linkId, storefrontId]);
  if (!r.rowCount) throw new DomainError("not_found");
}

/** Revokes every active link of a storefront; returns how many. */
export async function revokeAllPreviewLinks(c: ClientBase, storefrontId: string): Promise<number> {
  need(await getStorefront(c, storefrontId));
  const r = await c.query("update control.preview_links set revoked_at = now() where storefront_id = $1 and revoked_at is null and expires_at > now()", [storefrontId]);
  return r.rowCount ?? 0;
}

/** Which storefront a preview token opens (no organiser scope needed); null when unknown, expired or revoked. */
export async function resolvePreviewToken(c: ClientBase, token: string): Promise<{ organiser_id: string; storefront_id: string } | null> {
  if (!TOKEN.test(token)) return null;
  return (await c.query("select organiser_id::text, storefront_id::text from control.preview_link_target($1)", [hashPreviewToken(token)])).rows[0] ?? null;
}

// ---------------------------------------------------------------- shop context

export type ShopContext = {
  brand: BrandRow;
  organiser: { name: string; is_sample: boolean };
  hostnames: string[];
};

export async function shopContext(c: ClientBase, brandId: string): Promise<ShopContext> {
  const brand = need(await getBrand(c, brandId));
  const org = need((await c.query<{ name: string; is_sample: boolean }>("select name, is_sample from control.organisers where id = $1", [brand.organiser_id])).rows[0]);
  const hosts = await c.query<{ hostname: string }>("select hostname from control.brand_hostnames where brand_id = $1 order by hostname limit 20", [brandId]);
  return { brand, organiser: org, hostnames: hosts.rows.map((h) => h.hostname) };
}
