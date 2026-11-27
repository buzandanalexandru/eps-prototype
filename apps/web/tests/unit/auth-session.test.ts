import { describe, expect, it } from "vitest";
import { issueCoreToken, type AuthLike } from "@/lib/auth/core-token";
import { safeNext, signInRedirect } from "@/lib/auth/guard";
import { cookieFrom, makeAuth, post } from "./helpers";

describe("Studio guard", () => {
  it("redirects anonymous visitors to sign-in with the page to return to", () => {
    expect(signInRedirect("/studio", "", false)).toBe("/sign-in?next=%2Fstudio");
    expect(signInRedirect("/studio/events", "?tab=1", false)).toBe("/sign-in?next=%2Fstudio%2Fevents%3Ftab%3D1");
    expect(signInRedirect("/studio", "", true)).toBeNull();
  });

  it("leaves storefronts, status and sign-in open", () => {
    for (const p of ["/", "/status", "/sign-in", "/s/nibiru", "/studios"]) expect(signInRedirect(p, "", false)).toBeNull();
  });

  it("returns only to same-site paths", () => {
    expect(safeNext("/studio")).toBe("/studio");
    expect(safeNext("https://evil.example")).toBe("/studio");
    expect(safeNext("//evil.example")).toBe("/studio");
    expect(safeNext("/\\evil.example")).toBe("/studio");
    expect(safeNext(undefined)).toBe("/studio");
  });
});

describe("sign-out", () => {
  it("ends the session at once and refuses a new core token", async () => {
    const a = makeAuth({ EPS_DEMO_SIGNIN: "on" });
    const res = await a.auth.handler(post("/demo/sign-in", { as: "fan" }));
    const cookie = cookieFrom(res);
    const headers = new Headers({ cookie });
    expect(await a.auth.api.getSession({ headers })).not.toBeNull();
    const deps = {
      auth: a.auth as unknown as AuthLike,
      memberships: async () => null,
      secret: "s".repeat(40),
      secureCookies: false,
    };
    expect((await issueCoreToken(deps, { headers, guest: false })).status).toBe(200);

    const out = await a.auth.handler(post("/sign-out", {}, { cookie }));
    expect(out.status).toBe(200);
    expect(a.db.session).toHaveLength(0);
    // The old cookie no longer opens Studio (getSession is what the Studio layout checks)...
    expect(await a.auth.api.getSession({ headers, query: { disableCookieCache: true } })).toBeNull();
    // ...and it cannot renew a core token.
    expect((await issueCoreToken(deps, { headers, guest: false })).status).toBe(401);
  });
});
