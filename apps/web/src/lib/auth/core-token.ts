// Short-lived tokens for the Go core (U1, design D0.4): aud eps-core, kind, org and role for
// organisers, sid, jti, at most 5 minutes. Signed with the JWT plugin's current EdDSA key.
import { randomUUID } from "node:crypto";
import { CORE_AUDIENCE, CORE_TOKEN_TTL_SECONDS } from "./options";
import { GUEST_COOKIE, guestCookie, newGuestId, signGuest, verifyGuest } from "./guest";
import type { MembershipLookup } from "./membership";

export type CoreKind = "organiser" | "fan" | "guest";

/** The slice of a Better Auth instance this module needs. */
export type AuthLike = {
  api: {
    getSession(opts: { headers: Headers; query?: { disableCookieCache?: boolean } }): Promise<{
      session: { id: string; activeOrganiserId?: string | null };
      user: { id: string; email?: string | null; emailVerified?: boolean | null };
    } | null>;
    signJWT(opts: { body: { payload: Record<string, unknown> } }): Promise<{ token: string }>;
  };
};

export type CoreTokenDeps = {
  auth: AuthLike;
  /** Wrap with cachedMemberships in deployments, so a token costs at most one database round trip. */
  memberships: MembershipLookup;
  secret: string;
  secureCookies: boolean;
};

export type CoreTokenResult =
  | { status: 200; token: string; kind: CoreKind; expiresAt: number; setCookie?: string }
  | { status: 401 };

function readCookie(headers: Headers, name: string): string | undefined {
  const raw = headers.get("cookie") || "";
  for (const part of raw.split(";")) {
    const [k, ...v] = part.trim().split("=");
    if (k === name) return v.join("=");
  }
  return undefined;
}

/**
 * Issues a core token for the request. A signed-in user gets an organiser or fan token. Without a
 * session, a token is issued only when `guest` is asked for (storefront buy paths): the visitor
 * gets (or keeps) a guest cookie and a guest token. Otherwise 401, so a signed-out user cannot
 * renew a token.
 */
export async function issueCoreToken(
  deps: CoreTokenDeps,
  req: { headers: Headers; guest: boolean },
): Promise<CoreTokenResult> {
  const now = Math.floor(Date.now() / 1000);
  const exp = now + CORE_TOKEN_TTL_SECONDS;
  const base = { aud: CORE_AUDIENCE, iat: now, exp, jti: randomUUID() };

  // Always from the database, never the session cookie cache: a signed-out or revoked session must
  // not renew a token. With the signing key and memberships cached in process, this read is the
  // token's only database round trip.
  const session = await deps.auth.api.getSession({ headers: req.headers, query: { disableCookieCache: true } });
  if (session) {
    const m = await deps.memberships(session.user.id, session.session.activeOrganiserId);
    // A verified email lets the core add guest orders placed with it to the account (step 4, D-9).
    const email = session.user.emailVerified && session.user.email ? { email: session.user.email.trim().toLowerCase() } : {};
    const payload = m
      ? { ...base, ...email, sub: session.user.id, sid: session.session.id, kind: "organiser", org: m.organiserId, role: m.role }
      : { ...base, ...email, sub: session.user.id, sid: session.session.id, kind: "fan" };
    const { token } = await deps.auth.api.signJWT({ body: { payload } });
    return { status: 200, token, kind: payload.kind as CoreKind, expiresAt: exp };
  }
  if (!req.guest) return { status: 401 };

  let guestId = verifyGuest(readCookie(req.headers, GUEST_COOKIE), deps.secret);
  let setCookie: string | undefined;
  if (!guestId) {
    guestId = newGuestId();
    setCookie = guestCookie(signGuest(guestId, deps.secret), deps.secureCookies);
  }
  const { token } = await deps.auth.api.signJWT({
    body: { payload: { ...base, sub: `guest:${guestId}`, sid: `guest:${guestId}`, kind: "guest" } },
  });
  return { status: 200, token, kind: "guest", expiresAt: exp, setCookie };
}
