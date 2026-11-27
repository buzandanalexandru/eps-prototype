import type pg from "pg";
import { withTx } from "../db/tx";
import { coreRoleClaim } from "../organisers/roles";

export type Membership = { organiserId: string; role: string };

/**
 * Returns the user's active organiser membership, or null: the session's active organiser when the
 * user is still a member of it (design D1.1), else the oldest membership. `role` is the core-token
 * claim (owner, admin or staff).
 */
export type MembershipLookup = (userId: string, preferredOrganiserId?: string | null) => Promise<Membership | null>;

export function dbMembershipLookup(pool: Pick<pg.Pool, "connect">): MembershipLookup {
  return (userId, preferred) =>
    withTx(pool, { userId }, async (c) => {
      const r = await c.query<{ organiser_id: string; role: string }>(
        `select organiser_id::text, role from control.memberships where user_id = $1
         order by (organiser_id::text = $2) desc, created_at, organiser_id limit 1`,
        [userId, preferred ?? ""],
      );
      const row = r.rows[0];
      return row ? { organiserId: row.organiser_id, role: coreRoleClaim(row.role) } : null;
    });
}

/** How long one function instance reuses a user's membership (a removed role lasts at most this). */
export const MEMBERSHIP_CACHE_MS = 30_000;

/** Caches lookups per user and preferred organiser for ttlMs, bounded to max entries (oldest evicted first). */
export function cachedMemberships(lookup: MembershipLookup, ttlMs = MEMBERSHIP_CACHE_MS, max = 1000): MembershipLookup {
  const cache = new Map<string, { at: number; value: Membership | null }>();
  return async (userId, preferred) => {
    const key = `${userId}\u0000${preferred ?? ""}`;
    const now = Date.now();
    const hit = cache.get(key);
    if (hit && now - hit.at >= 0 && now - hit.at < ttlMs) return hit.value;
    const value = await lookup(userId, preferred);
    cache.delete(key);
    cache.set(key, { at: now, value });
    if (cache.size > max) cache.delete(cache.keys().next().value as string);
    return value;
  };
}

let shared: MembershipLookup | undefined;

/** The per-instance cached lookup over the app pool. */
export function sharedMembershipLookup(pool: Pick<pg.Pool, "connect">): MembershipLookup {
  if (!shared) shared = cachedMemberships(dbMembershipLookup(pool));
  return shared;
}
