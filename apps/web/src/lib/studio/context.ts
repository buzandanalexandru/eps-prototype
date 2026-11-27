// Who is using Studio and for which organiser (design D1.1). The active organiser is held in the
// sign-in session (switcher in the header), never in the URL, and is always re-checked against the
// user's memberships. Every Studio read and write goes through studioTx, scoped to it.
import "server-only";
import { redirect } from "next/navigation";
import type pg from "pg";
import { cache } from "react";
import { currentSession } from "../auth/session";
import { getAppPool } from "../db/pool";
import { withTx } from "../db/tx";
import { DomainError } from "../errors";
import { pickActive, userOrganisers, type UserOrganiser } from "../organisers/domain";

export type Studio = {
  user: { id: string; name: string; email: string };
  sessionId: string;
  organisers: UserOrganiser[];
  active: UserOrganiser | null;
};

/** The signed-in Studio user and the active organiser (null before onboarding). Cached per request. */
export const getStudio = cache(async (): Promise<Studio | null> => {
  const session = await currentSession();
  if (!session) return null;
  const organisers = await withTx(getAppPool(), { userId: session.user.id }, (c) => userOrganisers(c, session.user.id));
  const preferred = (session.session as { activeOrganiserId?: string | null }).activeOrganiserId;
  return {
    user: { id: session.user.id, name: session.user.name, email: session.user.email },
    sessionId: session.session.id,
    organisers,
    active: pickActive(organisers, preferred),
  };
});

export type ActiveStudio = Studio & { active: UserOrganiser };

/** For Studio pages: sign-in first, then onboarding until the organiser has its first brand. */
export async function requireStudio(): Promise<ActiveStudio> {
  const s = await getStudio();
  if (!s) redirect("/sign-in?next=%2Fstudio");
  if (!s.active || s.active.onboarding_state !== "done") redirect("/studio/onboarding");
  return s as ActiveStudio;
}

/** For server actions: the same checks, as errors instead of redirects. */
export async function requireActive(): Promise<ActiveStudio> {
  const s = await getStudio();
  if (!s) throw new DomainError("signed_out");
  if (!s.active) throw new DomainError("no_organiser");
  return s as ActiveStudio;
}

/** A transaction scoped to the active organiser and the user. */
export function studioTx<T>(s: ActiveStudio, fn: (c: pg.PoolClient) => Promise<T>): Promise<T> {
  return withTx(getAppPool(), { organiserId: s.active.id, userId: s.user.id }, fn);
}
