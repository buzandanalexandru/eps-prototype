import type pg from "pg";
import { withTx } from "../db/tx";

export type Membership = { organiserId: string; role: string };

/** Returns the user's active organiser membership (step 0: the oldest one), or null. */
export type MembershipLookup = (userId: string) => Promise<Membership | null>;

export function dbMembershipLookup(pool: Pick<pg.Pool, "connect">): MembershipLookup {
  return (userId) =>
    withTx(pool, { userId }, async (c) => {
      const r = await c.query<{ organiser_id: string; role: string }>(
        "select organiser_id::text, role from control.memberships where user_id = $1 order by created_at, organiser_id limit 1",
        [userId],
      );
      const row = r.rows[0];
      return row ? { organiserId: row.organiser_id, role: row.role } : null;
    });
}
