import { afterEach, describe, expect, it, vi } from "vitest";
import { createLocalJWKSet, decodeJwt, decodeProtectedHeader, jwtVerify, type JSONWebKeySet } from "jose";
import { issueCoreToken, type AuthLike } from "@/lib/auth/core-token";
import { GUEST_COOKIE, signGuest, verifyGuest } from "@/lib/auth/guest";
import type { MembershipLookup } from "@/lib/auth/membership";
import { cookieFrom, get, makeAuth, post } from "./helpers";

const SECRET = "test-secret-test-secret-test-secret-0000";
const ORG = "00000000-0000-4000-8000-00000000000a";

afterEach(() => vi.useRealTimers());

async function signIn(a: ReturnType<typeof makeAuth>, as: "organiser" | "fan") {
  const res = await a.auth.handler(post("/demo/sign-in", { as }));
  return { cookie: cookieFrom(res), userId: (await res.json()).userId as string };
}

async function jwks(a: ReturnType<typeof makeAuth>): Promise<JSONWebKeySet> {
  return (await a.auth.handler(get("/api/auth/jwks"))).json();
}

function deps(a: ReturnType<typeof makeAuth>, memberships: MembershipLookup = async () => null) {
  return { auth: a.auth as unknown as AuthLike, memberships, secret: SECRET, secureCookies: true };
}

describe("core tokens", () => {
  it("organiser token: kind, organiser, role, audience eps-core, at most 5 minutes", async () => {
    const a = makeAuth({ EPS_DEMO_SIGNIN: "on" });
    const { cookie, userId } = await signIn(a, "organiser");
    const lookup: MembershipLookup = async (uid) => (uid === userId ? { organiserId: ORG, role: "owner" } : null);
    const res = await issueCoreToken(deps(a, lookup), { headers: new Headers({ cookie }), guest: false });
    if (res.status !== 200) throw new Error("expected a token");
    const { payload, protectedHeader } = await jwtVerify(res.token, createLocalJWKSet(await jwks(a)), {
      audience: "eps-core",
    });
    expect(protectedHeader.alg).toBe("EdDSA");
    expect(protectedHeader.kid).toBeTruthy();
    expect(payload).toMatchObject({ sub: userId, kind: "organiser", org: ORG, role: "owner", aud: "eps-core" });
    expect(payload.sid).toBeTruthy();
    expect(payload.jti).toBeTruthy();
    expect(payload.iss).toBe("http://localhost:3000");
    expect(payload.exp! - payload.iat!).toBeLessThanOrEqual(300);
    expect(payload.exp! - Math.floor(Date.now() / 1000)).toBeLessThanOrEqual(300);
  });

  it("fan token has kind=fan and no organiser claim", async () => {
    const a = makeAuth({ EPS_DEMO_SIGNIN: "on" });
    const { cookie } = await signIn(a, "fan");
    const res = await issueCoreToken(deps(a), { headers: new Headers({ cookie }), guest: true });
    if (res.status !== 200) throw new Error("expected a token");
    const claims = decodeJwt(res.token);
    expect(claims.kind).toBe("fan");
    expect(claims).not.toHaveProperty("org");
    expect(claims).not.toHaveProperty("role");
    expect(res.setCookie).toBeUndefined(); // a signed-in fan never becomes a guest
  });

  it("a visitor without a session gets a guest cookie and a guest token; the cookie is reused", async () => {
    const a = makeAuth();
    const res = await issueCoreToken(deps(a), { headers: new Headers(), guest: true });
    if (res.status !== 200) throw new Error("expected a token");
    expect(res.setCookie).toMatch(new RegExp(`^${GUEST_COOKIE}=`));
    expect(res.setCookie).toContain("HttpOnly");
    expect(res.setCookie).toContain("Secure");
    const value = res.setCookie!.split(";")[0].slice(GUEST_COOKIE.length + 1);
    const guestId = verifyGuest(value, SECRET);
    expect(guestId).toBeTruthy();
    const claims = decodeJwt(res.token);
    expect(claims).toMatchObject({ kind: "guest", sub: `guest:${guestId}`, aud: "eps-core" });
    expect(claims).not.toHaveProperty("org");

    const again = await issueCoreToken(deps(a), {
      headers: new Headers({ cookie: `${GUEST_COOKIE}=${value}` }),
      guest: true,
    });
    if (again.status !== 200) throw new Error("expected a token");
    expect(again.setCookie).toBeUndefined();
    expect(decodeJwt(again.token).sub).toBe(`guest:${guestId}`);
  });

  it("a forged guest cookie is replaced, not trusted", async () => {
    const a = makeAuth();
    const forged = signGuest("11111111-1111-4111-8111-111111111111", "another-secret-another-secret-0000");
    const res = await issueCoreToken(deps(a), { headers: new Headers({ cookie: `${GUEST_COOKIE}=${forged}` }), guest: true });
    if (res.status !== 200) throw new Error("expected a token");
    expect(res.setCookie).toBeDefined();
    expect(decodeJwt(res.token).sub).not.toBe("guest:11111111-1111-4111-8111-111111111111");
  });

  it("without a session and without asking for a guest token, the answer is 401", async () => {
    const a = makeAuth();
    expect((await issueCoreToken(deps(a), { headers: new Headers(), guest: false })).status).toBe(401);
  });

  it("rotation: new tokens use a new key id; the previous key stays published for 7 days", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    const t0 = new Date("2026-11-27T12:00:00Z");
    vi.setSystemTime(t0);
    const a = makeAuth();
    const first = await issueCoreToken(deps(a), { headers: new Headers(), guest: true });
    if (first.status !== 200) throw new Error("expected a token");
    const kid1 = decodeProtectedHeader(first.token).kid;

    // 31 days later the key has rotated.
    vi.setSystemTime(new Date(t0.getTime() + 31 * 86400_000));
    const second = await issueCoreToken(deps(a), { headers: new Headers(), guest: true });
    if (second.status !== 200) throw new Error("expected a token");
    const kid2 = decodeProtectedHeader(second.token).kid;
    expect(kid2).not.toBe(kid1);
    const published = (await jwks(a)).keys.map((k) => k.kid);
    expect(published).toEqual(expect.arrayContaining([kid1, kid2]));
    // A token signed with the previous key still verifies (checked at its own time).
    await expect(
      jwtVerify(first.token, createLocalJWKSet(await jwks(a)), { audience: "eps-core", currentDate: t0 }),
    ).resolves.toBeTruthy();

    // 30 + 7 days after the first key was made, plus a day: it is no longer published.
    vi.setSystemTime(new Date(t0.getTime() + 38 * 86400_000));
    const later = (await jwks(a)).keys.map((k) => k.kid);
    expect(later).not.toContain(kid1);
    expect(later).toContain(kid2);
  });
});
