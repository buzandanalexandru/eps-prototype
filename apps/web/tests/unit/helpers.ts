import { betterAuth, type BetterAuthPlugin } from "better-auth";
import { memoryAdapter } from "better-auth/adapters/memory";
import { buildAuthOptions } from "@/lib/auth/options";
import { DemoMailbox, MemoryMailboxStore } from "@/lib/auth/mailer";
import type { Env } from "@/lib/env";

export const ORIGIN = "http://localhost:3000";
export const DOMAIN = "demo.eps.invalid";

export function makeAuth(env: Env = {}, extraPlugins: BetterAuthPlugin[] = []) {
  const db: Record<string, unknown[]> = { user: [], session: [], account: [], verification: [], jwks: [], rateLimit: [] };
  const store = new MemoryMailboxStore();
  const options = buildAuthOptions({
    database: memoryAdapter(db),
    mailer: new DemoMailbox(store, DOMAIN),
    env: { BETTER_AUTH_URL: ORIGIN, ...env },
  });
  const auth = betterAuth({ ...options, plugins: [...options.plugins, ...extraPlugins] });
  return { auth, db, store, options };
}

let ipCounter = 1;
/** A fresh client IP per call keeps the rate limiter out of tests that are not about it. */
export function freshIp() {
  ipCounter++;
  return `10.0.${Math.floor(ipCounter / 250)}.${ipCounter % 250}`;
}

export function post(path: string, body: unknown, init: { cookie?: string; ip?: string } = {}) {
  return new Request(`${ORIGIN}/api/auth${path}`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      origin: ORIGIN,
      "x-forwarded-for": init.ip ?? freshIp(),
      ...(init.cookie ? { cookie: init.cookie } : {}),
    },
    body: JSON.stringify(body),
  });
}

export function get(url: string, cookie?: string) {
  return new Request(url.startsWith("http") ? url : `${ORIGIN}${url}`, {
    headers: { "x-forwarded-for": freshIp(), ...(cookie ? { cookie } : {}) },
    redirect: "manual",
  });
}

/** Collects Set-Cookie headers into a Cookie header value. */
export function cookieFrom(res: Response): string {
  return res.headers
    .getSetCookie()
    .map((c) => c.split(";")[0])
    .filter((c) => !c.endsWith("="))
    .join("; ");
}
