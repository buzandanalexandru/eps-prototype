// Organiser, brand, team and fee-policy operations (design D1.1 to D1.3, D1.6). Every function takes
// a client inside the transaction helper (withTx), so row-level security scopes it to one organiser.
// Studio's server actions and the seed both call these: no write skips validation.
import { createHash, randomBytes, randomUUID } from "node:crypto";
import type { ClientBase } from "pg";
import * as z from "zod";
import { DomainError } from "../errors";
import { DEFAULT_FEE_POLICY, isFeePolicy, type FeePolicy, type PlatformFee } from "../pricing/fees";
import type { Currency } from "../pricing/money";
import { assertKeepsAnOwner, assertOwner, isRole, ROLES, type Role, type ScopeType } from "./roles";
import { checkSlug, type SlugKind } from "./slugs";

export const TEMPLATES = ["festival", "resort", "club", "blank"] as const;
export type Template = (typeof TEMPLATES)[number];
export const LANGUAGES = ["ro", "en"] as const;
export type Language = (typeof LANGUAGES)[number];

export const organiserInput = z.object({
  name: z.string().trim().min(1).max(120),
  slug: z.string().trim(),
});

export const brandInput = z.object({
  name: z.string().trim().min(1).max(120),
  slug: z.string().trim(),
  languages: z.array(z.enum(LANGUAGES)).min(1, "languages_required").max(2),
  currency: z.enum(["RON", "EUR"]),
  template: z.enum(TEMPLATES).default("festival"),
  logoUrl: z
    .string()
    .trim()
    .max(500)
    .refine((s) => s === "" || /^https:\/\/[^\s]+$/.test(s), "logo_https")
    .optional()
    .transform((s) => s || null),
});
export type BrandInput = z.input<typeof brandInput>;

async function assertSlug(c: ClientBase, kind: SlugKind, slug: string, opts: { sample?: boolean } = {}) {
  const r = await checkSlug(c, kind, slug, opts);
  if (r !== "ok") throw new DomainError(r, "slug");
}

// ---------------------------------------------------------------- organisers and onboarding

export type OrganiserRow = { id: string; slug: string; name: string; is_sample: boolean; onboarding_state: "brand" | "done" };

/**
 * Onboarding step 1: creates the organisation with the user as owner, at onboarding state "brand".
 * The transaction must be opened with organiserId = the new id (see newOrganiserId), because the
 * registry and the owner membership are under row-level security. Idempotent per user: a per-user
 * lock serialises double submits, and a user who already owns an organisation still at the brand
 * step gets that one back instead of a second organisation.
 */
export async function createOrganiser(
  c: ClientBase,
  p: { id: string; userId: string; input: z.input<typeof organiserInput> },
): Promise<OrganiserRow> {
  const input = organiserInput.parse(p.input);
  await c.query("select pg_advisory_xact_lock(hashtext('onboarding:' || $1))", [p.userId]);
  const pending = (await userOrganisers(c, p.userId)).find((o) => o.onboarding_state === "brand" && o.role === "owner");
  if (pending) return pending;
  await assertSlug(c, "organiser", input.slug);
  const r = await c.query<OrganiserRow>(
    `insert into control.organisers (id, slug, name, onboarding_state) values ($1, $2, $3, 'brand')
     returning id, slug, name, is_sample, onboarding_state`,
    [p.id, input.slug, input.name],
  );
  await c.query("insert into control.memberships (organiser_id, user_id, role) values ($1, $2, 'owner')", [p.id, p.userId]);
  return r.rows[0];
}

export function newOrganiserId(): string {
  return randomUUID();
}

/**
 * Onboarding step 2: the first brand and the template; the organiser is then done onboarding.
 * Idempotent: a second submit (double click, retry) returns the brand the first one created.
 */
export async function completeOnboarding(c: ClientBase, organiserId: string, input: BrandInput): Promise<{ brandId: string }> {
  await c.query("select pg_advisory_xact_lock(hashtext('onboarding-org:' || $1))", [organiserId]);
  const org = await c.query<{ onboarding_state: string }>("select onboarding_state from control.organisers where id = $1", [organiserId]);
  if (!org.rows[0]) throw new DomainError("not_found");
  if (org.rows[0].onboarding_state === "done") {
    const first = (await listBrands(c))[0];
    if (first) return { brandId: first.id };
  }
  const brand = await createBrand(c, organiserId, input);
  await c.query("update control.organisers set onboarding_state = 'done' where id = $1", [organiserId]);
  return { brandId: brand.id };
}

// ---------------------------------------------------------------- brands

export type BrandRow = {
  id: string;
  organiser_id: string;
  slug: string;
  name: string;
  languages: Language[];
  currency: Currency;
  template: Template;
  logo_url: string | null;
};

/** The transaction's organiser (set by withTx). */
export const CURRENT_ORG = "nullif(current_setting('app.organiser_id', true), '')::uuid";

const BRAND_COLS = "id, organiser_id, slug, name, languages, currency, template, logo_url";

/** Creates a brand. `id` and `opts.sample` are for the seed only (never taken from a request). */
export async function createBrand(c: ClientBase, organiserId: string, raw: BrandInput, id?: string, opts: { sample?: boolean } = {}): Promise<BrandRow> {
  const input = brandInput.parse(raw);
  await assertSlug(c, "brand", input.slug, opts);
  const r = await c.query<BrandRow>(
    `insert into control.brands (id, organiser_id, slug, name, languages, currency, template, logo_url)
     values (coalesce($1, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8) returning ${BRAND_COLS}`,
    [id ?? null, organiserId, input.slug, input.name, input.languages, input.currency, input.template, input.logoUrl],
  );
  return r.rows[0];
}

export async function updateBrand(c: ClientBase, organiserId: string, brandId: string, raw: BrandInput): Promise<BrandRow> {
  const input = brandInput.parse(raw);
  const cur = await getBrand(c, brandId);
  if (!cur || cur.organiser_id !== organiserId) throw new DomainError("not_found");
  if (cur.slug !== input.slug) {
    // A sample organiser's addresses are fixed (the seed restores them).
    const sample = (await c.query<{ is_sample: boolean }>("select is_sample from control.organisers where id = $1", [organiserId])).rows[0]?.is_sample;
    if (sample) throw new DomainError("sample_locked", "slug");
    await assertSlug(c, "brand", input.slug);
  }
  if (cur.currency !== input.currency) {
    // Prices are stored in the brand's currency: it is locked once the brand has events.
    const n = (await c.query<{ n: number }>("select count(*)::int as n from control.events where brand_id = $1", [brandId])).rows[0].n;
    if (n > 0) throw new DomainError("currency_locked", "currency");
  }
  const r = await c.query<BrandRow>(
    `update control.brands set slug = $2, name = $3, languages = $4, currency = $5, template = $6, logo_url = $7
     where id = $1 returning ${BRAND_COLS}`,
    [brandId, input.slug, input.name, input.languages, input.currency, input.template, input.logoUrl],
  );
  return r.rows[0];
}

export async function listBrands(c: ClientBase): Promise<BrandRow[]> {
  return (await c.query<BrandRow>(`select ${BRAND_COLS} from control.brands order by created_at, slug`)).rows;
}

export async function getBrand(c: ClientBase, brandId: string): Promise<BrandRow | null> {
  if (!z.uuid().safeParse(brandId).success) return null;
  return (await c.query<BrandRow>(`select ${BRAND_COLS} from control.brands where id = $1`, [brandId])).rows[0] ?? null;
}

export async function deleteBrand(c: ClientBase, role: string, brandId: string): Promise<void> {
  assertOwner(role, "delete_brand");
  if (!(await getBrand(c, brandId))) throw new DomainError("not_found");
  // Published or archived events (and, later, their sales) keep their brand.
  const live = await c.query<{ n: number }>("select count(*)::int as n from control.events where brand_id = $1 and status <> 'draft'", [brandId]);
  if (live.rows[0].n > 0) throw new DomainError("brand_has_published_events");
  await c.query("delete from control.events where brand_id = $1 and status = 'draft'", [brandId]);
  const r = await c.query("delete from control.brands where id = $1", [brandId]);
  if (r.rowCount === 0) throw new DomainError("not_found");
}

// ---------------------------------------------------------------- team

export type MemberRow = { user_id: string; name: string; email: string; role: string; scope_type: ScopeType; scope_id: string | null };
export type InvitationRow = { id: string; email: string; role: Role; scope_type: ScopeType; scope_id: string | null; created_at: Date; expires_at: Date };

export async function listMembers(c: ClientBase): Promise<MemberRow[]> {
  return (
    await c.query<MemberRow>(
      `select m.user_id, u.name, u.email, m.role, m.scope_type, m.scope_id::text
       from control.memberships m join control."user" u on u.id = m.user_id
       where m.organiser_id = ${CURRENT_ORG}
       order by (m.role = 'owner') desc, m.created_at`,
    )
  ).rows;
}

export async function listInvitations(c: ClientBase): Promise<InvitationRow[]> {
  return (
    await c.query<InvitationRow>(
      `select id, email, role, scope_type, scope_id::text, created_at, expires_at from control.invitations
       where accepted_at is null and expires_at > now() order by created_at desc`,
    )
  ).rows;
}

export const scopeInput = z
  .object({ scopeType: z.enum(["organiser", "brand", "event"]).default("organiser"), scopeId: z.string().optional() })
  .transform((s) => (s.scopeType === "organiser" ? { scopeType: "organiser" as const, scopeId: null } : { scopeType: s.scopeType, scopeId: s.scopeId ?? "" }))
  .refine((s) => s.scopeType === "organiser" || z.uuid().safeParse(s.scopeId).success, { message: "scope_required", path: ["scopeId"] });

async function assertScopeExists(c: ClientBase, scope: { scopeType: ScopeType; scopeId: string | null }) {
  if (scope.scopeType === "organiser") return;
  const table = scope.scopeType === "brand" ? "control.brands" : "control.events";
  const r = await c.query(`select 1 from ${table} where id = $1`, [scope.scopeId]);
  if (!r.rowCount) throw new DomainError("not_found", "scopeId");
}

export const inviteInput = z.object({
  email: z.string().trim().toLowerCase().pipe(z.email("email_invalid")),
  role: z.enum(ROLES),
});

export function hashToken(token: string): string {
  return createHash("sha256").update(token).digest("hex");
}

/** Creates a pending invitation and returns the raw token (shown once as a link; only its hash is stored). */
export async function inviteMember(
  c: ClientBase,
  p: { role: string; userId: string; input: { email: string; role: string; scopeType?: string; scopeId?: string } },
): Promise<{ id: string; token: string }> {
  assertOwner(p.role, "team");
  const input = inviteInput.parse(p.input);
  const scope = scopeInput.parse({ scopeType: p.input.scopeType || "organiser", scopeId: p.input.scopeId || undefined });
  await assertScopeExists(c, scope);
  const token = randomBytes(24).toString("base64url");
  const r = await c.query<{ id: string }>(
    `insert into control.invitations (organiser_id, email, role, scope_type, scope_id, token_hash, invited_by)
     values (${CURRENT_ORG}, $1, $2, $3, $4, $5, $6) returning id`,
    [input.email, input.role, scope.scopeType, scope.scopeId, hashToken(token), p.userId],
  );
  return { id: r.rows[0].id, token };
}

export async function revokeInvitation(c: ClientBase, role: string, invitationId: string): Promise<void> {
  assertOwner(role, "team");
  if (!z.uuid().safeParse(invitationId).success) throw new DomainError("not_found");
  await c.query("delete from control.invitations where id = $1 and accepted_at is null", [invitationId]);
}

/**
 * Accepts an invitation: the transaction is scoped to the invitation's organiser (from the link),
 * the token must match a pending, unexpired invitation, and the signed-in user's email must be the
 * invited address. Adds (or updates) the membership.
 */
export async function acceptInvitation(c: ClientBase, p: { organiserId: string; token: string; userId: string; email: string }): Promise<void> {
  const r = await c.query<{ id: string; email: string; role: string; scope_type: string; scope_id: string | null }>(
    `select id, email, role, scope_type, scope_id from control.invitations
     where token_hash = $1 and accepted_at is null and expires_at > now() for update`,
    [hashToken(p.token)],
  );
  const inv = r.rows[0];
  if (!inv) throw new DomainError("invitation_invalid");
  if (inv.email !== p.email.toLowerCase()) throw new DomainError("invitation_other_email");
  await c.query(
    `insert into control.memberships (organiser_id, user_id, role, scope_type, scope_id)
     values ($1, $2, $3, $4, $5)
     on conflict (organiser_id, user_id) do update set role = case when control.memberships.role = 'owner' then 'owner' else excluded.role end,
       scope_type = case when control.memberships.role = 'owner' then control.memberships.scope_type else excluded.scope_type end,
       scope_id = case when control.memberships.role = 'owner' then control.memberships.scope_id else excluded.scope_id end`,
    [p.organiserId, p.userId, inv.role, inv.scope_type, inv.scope_id],
  );
  await c.query("update control.invitations set accepted_at = now() where id = $1", [inv.id]);
}

async function lockedMembers(c: ClientBase) {
  // Lock the organiser's memberships so two concurrent demotions cannot both pass the last-owner rule.
  return (await c.query<{ user_id: string; role: string }>(`select user_id, role from control.memberships where organiser_id = ${CURRENT_ORG} for update`)).rows;
}

export async function changeRole(
  c: ClientBase,
  p: { role: string; targetUserId: string; input: { role: string; scopeType?: string; scopeId?: string } },
): Promise<void> {
  assertOwner(p.role, "team");
  if (!isRole(p.input.role)) throw new DomainError("invalid", "role");
  const scope = scopeInput.parse({ scopeType: p.input.scopeType || "organiser", scopeId: p.input.scopeId || undefined });
  await assertScopeExists(c, scope);
  const members = await lockedMembers(c);
  const target = members.find((m) => m.user_id === p.targetUserId);
  if (!target) throw new DomainError("not_found");
  assertKeepsAnOwner(members.filter((m) => m.role === "owner").length, target.role === "owner", p.input.role);
  const finalScope = p.input.role === "owner" ? { scopeType: "organiser", scopeId: null } : scope;
  await c.query(`update control.memberships set role = $2, scope_type = $3, scope_id = $4 where user_id = $1 and organiser_id = ${CURRENT_ORG}`, [
    p.targetUserId,
    p.input.role,
    finalScope.scopeType,
    finalScope.scopeId,
  ]);
}

export async function removeMember(c: ClientBase, p: { role: string; targetUserId: string }): Promise<void> {
  assertOwner(p.role, "team");
  const members = await lockedMembers(c);
  const target = members.find((m) => m.user_id === p.targetUserId);
  if (!target) throw new DomainError("not_found");
  assertKeepsAnOwner(members.filter((m) => m.role === "owner").length, target.role === "owner", null);
  await c.query(`delete from control.memberships where user_id = $1 and organiser_id = ${CURRENT_ORG}`, [p.targetUserId]);
}

// ---------------------------------------------------------------- fees

export async function getPlatformFee(c: ClientBase, currency: Currency, at: Date = new Date()): Promise<PlatformFee> {
  const r = await c.query<{ percent_bps: number; fixed_minor: number }>(
    `select percent_bps, fixed_minor from control.platform_fee where currency = $1 and effective_from <= $2
     order by effective_from desc, id desc limit 1`,
    [currency, at],
  );
  const row = r.rows[0];
  if (!row) throw new Error(`no platform fee for ${currency}`);
  return { percentBps: row.percent_bps, fixedMinor: row.fixed_minor };
}

export async function getFeePolicy(c: ClientBase): Promise<FeePolicy> {
  const r = await c.query<{ policy: string }>("select policy from control.fee_policies");
  const p = r.rows[0]?.policy;
  return isFeePolicy(p) ? p : DEFAULT_FEE_POLICY;
}

export async function setFeePolicy(c: ClientBase, role: string, policy: string): Promise<void> {
  assertOwner(role, "fee_policy");
  if (!isFeePolicy(policy)) throw new DomainError("invalid", "policy");
  await c.query(
    `insert into control.fee_policies (organiser_id, policy) values (${CURRENT_ORG}, $1)
     on conflict (organiser_id) do update set policy = excluded.policy, updated_at = now()`,
    [policy],
  );
}

/** Per-event override (null clears it, so the organiser policy applies). */
export async function setEventFeePolicy(c: ClientBase, role: string, eventId: string, policy: string | null): Promise<void> {
  assertOwner(role, "fee_policy");
  if (!z.uuid().safeParse(eventId).success) throw new DomainError("not_found");
  if (policy !== null && !isFeePolicy(policy)) throw new DomainError("invalid", "policy");
  const r = await c.query("update control.events set fee_policy = $2, updated_at = now() where id = $1", [eventId, policy]);
  if (r.rowCount === 0) throw new DomainError("not_found");
}

// ---------------------------------------------------------------- memberships of a user

export type UserOrganiser = OrganiserRow & { role: string };

/** Every organiser the user belongs to (through the own-memberships read policy). */
export async function userOrganisers(c: ClientBase, userId: string): Promise<UserOrganiser[]> {
  return (
    await c.query<UserOrganiser>(
      `select o.id, o.slug, o.name, o.is_sample, o.onboarding_state, m.role
       from control.memberships m join control.organisers o on o.id = m.organiser_id
       where m.user_id = $1 order by m.created_at, o.id`,
      [userId],
    )
  ).rows;
}

/** The active organiser: the session's choice when the user is still a member, else the oldest membership. */
export function pickActive<T extends { id: string }>(orgs: T[], preferred: string | null | undefined): T | null {
  return orgs.find((o) => o.id === preferred) ?? orgs[0] ?? null;
}

export async function setActiveOrganiser(c: ClientBase, p: { sessionId: string; userId: string; organiserId: string }): Promise<void> {
  const orgs = await userOrganisers(c, p.userId);
  if (!orgs.some((o) => o.id === p.organiserId)) throw new DomainError("not_found");
  await c.query(`update control."session" set "activeOrganiserId" = $1 where id = $2 and "userId" = $3`, [
    p.organiserId,
    p.sessionId,
    p.userId,
  ]);
}
