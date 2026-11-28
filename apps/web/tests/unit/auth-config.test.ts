import { describe, expect, it } from "vitest";
import { createElement } from "react";
import { createAuthEndpoint } from "better-auth/api";
import { handleOAuthUserInfo } from "better-auth/oauth2";
import type { BetterAuthPlugin } from "better-auth";
import * as z from "zod";
import en from "../../messages/en.json";
import { googleConfigured, demoSignInEnabled } from "@/lib/env";
import { SignInForm } from "@/app/(platform)/sign-in/sign-in-form";
import { renderWithIntl } from "./render";
import { cookieFrom, DOMAIN, makeAuth, post } from "./helpers";

function renderForm(env: Record<string, string | undefined>) {
  return renderWithIntl("en", en, createElement(SignInForm, {
        next: "/studio",
        google: googleConfigured(env),
        demo: demoSignInEnabled(env),
        initialEmail: "",
      }));
}

// Stands in for Google's callback: runs Better Auth's own OAuth user-info handling with a verified
// Google profile, exactly as the real callback does after the token exchange.
const fakeGoogleCallback: BetterAuthPlugin = {
  id: "test-fake-google",
  endpoints: {
    fakeGoogle: createAuthEndpoint(
      "/test/fake-google",
      { method: "POST", body: z.object({ email: z.string(), sub: z.string() }) },
      async (ctx) => {
        const r = await handleOAuthUserInfo(ctx, {
          userInfo: { id: ctx.body.sub, email: ctx.body.email, emailVerified: true, name: "G User", image: null },
          account: { providerId: "google", accountId: ctx.body.sub, accessToken: "at" },
          callbackURL: "/studio",
          source: { method: "oauth", oauth: { providerId: "google" } },
        });
        return ctx.json({ error: r.error ?? null, userId: r.data?.user.id ?? null });
      },
    ),
  },
};

describe("Google sign-in configuration", () => {
  it("shows no Google button and registers no provider when credentials are absent", async () => {
    expect(renderForm({})).not.toContain("Continue with Google");
    const { options } = makeAuth({});
    expect(Object.keys(options.socialProviders)).toEqual([]);
  });

  it("offers Google when both credentials exist", () => {
    const env = { GOOGLE_CLIENT_ID: "id", GOOGLE_CLIENT_SECRET: "secret" };
    expect(renderForm(env)).toContain("Continue with Google");
    const { options } = makeAuth(env);
    expect(Object.keys(options.socialProviders)).toEqual(["google"]);
    expect(renderForm({ GOOGLE_CLIENT_ID: "id" })).not.toContain("Continue with Google");
  });

  it("a verified Google address that already has a user attaches to that user", async () => {
    const a = makeAuth({ GOOGLE_CLIENT_ID: "id", GOOGLE_CLIENT_SECRET: "secret" }, [fakeGoogleCallback]);
    const email = "linked@example.com"; // a real-world address (magic link with a mail provider)
    const ctx = await a.auth.$context;
    const existing = await ctx.internalAdapter.createUser({ email, name: "Linked", emailVerified: true }, { method: "magic-link" } as never);

    const res = await a.auth.handler(post("/test/fake-google", { email, sub: "google-sub-1" }));
    const body = await res.json();
    expect(body.error).toBeNull();
    expect(body.userId).toBe(existing.id);
    expect(a.db.user).toHaveLength(1);
    const accounts = a.db.account as { providerId: string; userId: string }[];
    expect(accounts.filter((x) => x.providerId === "google").map((x) => x.userId)).toEqual([existing.id]);
  });

  it("never links a Google account to a demo-domain identity, nor creates one from Google", async () => {
    const a = makeAuth({ GOOGLE_CLIENT_ID: "id", GOOGLE_CLIENT_SECRET: "secret", EPS_DEMO_SIGNIN: "on" }, [fakeGoogleCallback]);
    expect(DOMAIN).toBe("demo.eps.invalid");
    // The shared demo organiser exists...
    const org = await (await a.auth.handler(post("/demo/sign-in", { as: "organiser" }))).json();
    // ...and a Google account claiming its address does not get it.
    const res = await a.auth.handler(post("/test/fake-google", { email: org.email, sub: "attacker" }));
    expect((await res.json()).userId).toBeNull();
    const accounts = a.db.account as { providerId: string }[];
    expect(accounts.filter((x) => x.providerId === "google")).toHaveLength(0);
    // A new demo-domain address through Google is refused too.
    const fresh = await a.auth.handler(post("/test/fake-google", { email: `someone@${DOMAIN}`, sub: "g2" }));
    expect((await fresh.json()).userId).toBeNull();
    expect((a.db.user as { email: string }[]).map((u) => u.email)).toEqual([org.email]);
  });
});

describe("demo sign-in switch", () => {
  it("is off unless EPS_DEMO_SIGNIN=on: no buttons and the endpoint answers 404", async () => {
    for (const env of [{}, { EPS_DEMO_SIGNIN: "true" }, { EPS_DEMO_SIGNIN: "1" }]) {
      expect(renderForm(env)).not.toContain("Continue as demo");
      const a = makeAuth(env);
      const res = await a.auth.handler(post("/demo/sign-in", { as: "fan" }));
      expect(res.status).toBe(404);
      expect(a.db.user).toHaveLength(0);
    }
  });

  it("when on, shows both buttons and signs in demo users", async () => {
    const html = renderForm({ EPS_DEMO_SIGNIN: "on" });
    expect(html).toContain("Continue as demo organiser");
    expect(html).toContain("Continue as demo fan");
    const a = makeAuth({ EPS_DEMO_SIGNIN: "on" });
    const org1 = await a.auth.handler(post("/demo/sign-in", { as: "organiser" }));
    const org2 = await a.auth.handler(post("/demo/sign-in", { as: "organiser" }));
    const fan1 = await a.auth.handler(post("/demo/sign-in", { as: "fan" }));
    const fan2 = await a.auth.handler(post("/demo/sign-in", { as: "fan" }));
    const ids = await Promise.all([org1, org2, fan1, fan2].map(async (r) => (await r.json()).userId));
    expect(ids[0]).toBe(ids[1]); // one shared demo organiser
    expect(ids[2]).not.toBe(ids[3]); // a fresh fan per click
    expect(cookieFrom(fan1)).toContain("session_token");
  });

  it("two first clicks at once share one demo organiser", async () => {
    const a = makeAuth({ EPS_DEMO_SIGNIN: "on" });
    // The memory adapter has no unique index; stand in for control."user"."email" being unique.
    const ctx = await a.auth.$context;
    const create = ctx.internalAdapter.createUser.bind(ctx.internalAdapter);
    // Like Postgres, a second insert of the same email waits for the first and then fails.
    const inFlight = new Map<string, Promise<unknown>>();
    ctx.internalAdapter.createUser = (async (user: { email: string }, ...rest: unknown[]) => {
      const first = inFlight.get(user.email);
      if (first) {
        await first.catch(() => {});
        throw new Error('duplicate key value violates unique constraint "user_email_key"');
      }
      const p = (async () => {
        await new Promise((r) => setTimeout(r, 5)); // let the other clicks reach createUser meanwhile
        return (create as (...a: unknown[]) => Promise<unknown>)(user, ...rest);
      })();
      inFlight.set(user.email, p);
      return p;
    }) as typeof ctx.internalAdapter.createUser;
    const res = await Promise.all([1, 2, 3].map(() => a.auth.handler(post("/demo/sign-in", { as: "organiser" }))));
    const ids = await Promise.all(res.map(async (r) => (await r.json()).userId));
    expect(res.every((r) => r.status === 200)).toBe(true);
    expect(new Set(ids).size).toBe(1);
  });

  it("is rate-limited per address", async () => {
    const a = makeAuth({ EPS_DEMO_SIGNIN: "on", EPS_DEMO_SIGNIN_PER_MINUTE: "10" });
    const statuses: number[] = [];
    for (let i = 0; i < 12; i++) {
      statuses.push((await a.auth.handler(post("/demo/sign-in", { as: "fan" }, { ip: "203.0.113.7" }))).status);
    }
    expect(statuses.slice(0, 10).every((s) => s === 200)).toBe(true);
    expect(statuses.slice(10)).toEqual([429, 429]);
    // Another address is unaffected.
    expect((await a.auth.handler(post("/demo/sign-in", { as: "fan" }, { ip: "203.0.113.8" }))).status).toBe(200);
  });
});
