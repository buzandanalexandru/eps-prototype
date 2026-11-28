"use server";
// Builder server actions (design D2.1, D2.5, D2.7): each checks the session, the active organiser and
// the designer role, validates its input with zod and runs in one transaction scoped to the
// organiser. Results are plain data for the builder's client state.
import * as z from "zod";
import { DomainError, fromPgError } from "@/lib/errors";
import { assertCanDesign } from "@/lib/storefront/access";
import { loadShopCatalogue } from "@/lib/storefront/catalogue-load";
import type { ShopCatalogue } from "@/lib/storefront/catalogue";
import {
  applyTemplate,
  createPreviewLink,
  DraftConflictError,
  getStorefront,
  listPreviewLinks,
  listVersions,
  PublishBlockedError,
  publishDraft,
  restoreVersion,
  revokePreviewLink,
} from "@/lib/storefront/domain";
import type { PublishProblem } from "@/lib/storefront/publish-checks";
import type { StorefrontDocument } from "@/lib/storefront/schema";
import { TEMPLATE_IDS } from "@/lib/storefront/templates";
import { requireActive, studioTx } from "@/lib/studio/context";

export type VersionView = { number: number; state: string; restoredFrom: number | null; publishedAt: string; author: string | null };
export type LinkView = { id: string; createdAt: string; expiresAt: string; revoked: boolean; expired: boolean; author: string | null };

export type ActionResult<T> = { ok: true; data: T } | { ok: false; code: string; revision?: number; problems?: PublishProblem[] };

const uuid = z.uuid();
const bad: { ok: false; code: string; revision?: number; problems?: PublishProblem[] } = { ok: false, code: "invalid" };
function parse<T extends z.ZodType>(schema: T, v: unknown): z.infer<T> | undefined {
  const r = schema.safeParse(v);
  return r.success ? r.data : undefined;
}

async function run<T>(fn: (s: Awaited<ReturnType<typeof requireActive>>) => Promise<T>, write = true): Promise<ActionResult<T>> {
  try {
    const s = await requireActive();
    if (write) assertCanDesign(s.active.role);
    return { ok: true, data: await fn(s) };
  } catch (err) {
    if (err instanceof DraftConflictError) return { ok: false, code: "draft_conflict", revision: err.currentRevision };
    if (err instanceof PublishBlockedError) return { ok: false, code: "publish_blocked", problems: err.problems };
    if (err instanceof z.ZodError) return { ok: false, code: "invalid" };
    const d = err instanceof DomainError ? err : fromPgError(err);
    if (d) return { ok: false, code: d.code };
    console.error("builder action failed", (err as Error)?.message);
    return { ok: false, code: "unexpected" };
  }
}

async function history(c: Parameters<typeof listVersions>[0], id: string): Promise<{ versions: VersionView[]; live: number | null }> {
  const sf = await getStorefront(c, id);
  if (!sf) throw new DomainError("not_found");
  const versions = (await listVersions(c, id, { limit: 50 })).map((v) => ({
    number: v.number,
    state: v.state,
    restoredFrom: v.restored_from,
    publishedAt: v.published_at.toISOString(),
    author: v.author,
  }));
  return { versions, live: sf.live_number };
}

export async function publishAction(storefrontId: string, revision: number) {
  const id = parse(uuid, storefrontId);
  if (id === undefined) return bad;
  const rev = parse(z.int().min(1), revision);
  if (rev === undefined) return bad;
  return run((s) =>
    studioTx(s, async (c) => {
      const v = await publishDraft(c, { storefrontId: id, revision: rev, userId: s.user.id });
      return { number: v.number, created: v.created, ...(await history(c, id)) };
    }),
  );
}

export async function restoreAction(storefrontId: string, number: number) {
  const id = parse(uuid, storefrontId);
  if (id === undefined) return bad;
  const n = parse(z.int().min(1), number);
  if (n === undefined) return bad;
  return run((s) =>
    studioTx(s, async (c) => {
      const v = await restoreVersion(c, { storefrontId: id, number: n, userId: s.user.id });
      const sf = (await getStorefront(c, id))!;
      return { number: v.number, revision: sf.draft_revision, document: sf.document as StorefrontDocument, ...(await history(c, id)) };
    }),
  );
}

export async function historyAction(storefrontId: string) {
  const id = parse(uuid, storefrontId);
  if (id === undefined) return bad;
  return run((s) => studioTx(s, (c) => history(c, id)), false);
}

function linkView(l: Awaited<ReturnType<typeof listPreviewLinks>>[number]): LinkView {
  return {
    id: l.id,
    createdAt: l.created_at.toISOString(),
    expiresAt: l.expires_at.toISOString(),
    revoked: !!l.revoked_at,
    expired: l.expired,
    author: l.author,
  };
}

export async function listLinksAction(storefrontId: string) {
  const id = parse(uuid, storefrontId);
  if (id === undefined) return bad;
  return run((s) => studioTx(s, async (c) => (await listPreviewLinks(c, id)).map(linkView)), false);
}

export async function createLinkAction(storefrontId: string) {
  const id = parse(uuid, storefrontId);
  if (id === undefined) return bad;
  return run((s) =>
    studioTx(s, async (c) => {
      const l = await createPreviewLink(c, id, s.user.id);
      return { token: l.token, path: `/p/${l.token}`, links: (await listPreviewLinks(c, id)).map(linkView) };
    }),
  );
}

export async function revokeLinkAction(storefrontId: string, linkId: string) {
  const id = parse(uuid, storefrontId);
  if (id === undefined) return bad;
  const lid = parse(uuid, linkId);
  if (lid === undefined) return bad;
  return run((s) =>
    studioTx(s, async (c) => {
      await revokePreviewLink(c, id, lid);
      return (await listPreviewLinks(c, id)).map(linkView);
    }),
  );
}

export async function changeTemplateAction(storefrontId: string, revision: number, template: string) {
  const id = parse(uuid, storefrontId);
  if (id === undefined) return bad;
  const rev = parse(z.int().min(1), revision);
  if (rev === undefined) return bad;
  const t = parse(z.enum(TEMPLATE_IDS), template);
  if (t === undefined) return bad;
  return run((s) => studioTx(s, (c) => applyTemplate(c, id, { revision: rev, template: t })));
}

/** Catalogue data for the preview frame: only the organiser's events (others are simply absent). */
export async function frameCatalogueAction(storefrontId: string, eventIds: string[]): Promise<ActionResult<ShopCatalogue>> {
  const id = parse(uuid, storefrontId);
  if (id === undefined) return bad;
  const ids = parse(z.array(uuid).max(60), eventIds);
  if (ids === undefined) return bad;
  return run(
    (s) =>
      studioTx(s, async (c) => {
        const sf = await getStorefront(c, id);
        if (!sf) throw new DomainError("not_found");
        const cat = await loadShopCatalogue(c, ids);
        // Only the brand's own events appear in its shop.
        const own = await c.query<{ id: string }>("select id::text from control.events where brand_id = $1 and id = any($2::uuid[])", [sf.brand_id, ids]);
        const keep = new Set(own.rows.map((r) => r.id));
        return Object.fromEntries(Object.entries(cat).filter(([k]) => keep.has(k)));
      }),
    false,
  );
}
