// Forwards an allowed call to the core with a fresh core token of the signed-in user (or none), and
// answers HTTP 200 with {status, body}: an expected outcome (a ticket that needs a name, a locked
// name) is something the page shows, not a browser network error.
import "server-only";
import { NextResponse } from "next/server";
import { authSecret, webOrigin } from "@/lib/env";
import { issueCoreToken } from "@/lib/auth/core-token";
import { sharedMembershipLookup } from "@/lib/auth/membership";
import { getAuth } from "@/lib/auth/server";
import { getAppPool } from "@/lib/db/pool";
import { coreBaseUrl } from "@/lib/core/url";
import type { Route } from "./routes";

const noStore = { "Cache-Control": "no-store" };

export function envelope(status: number, body: unknown) {
  return NextResponse.json({ status, body }, { headers: noStore });
}

export function problem(status: number, code: string) {
  return envelope(status, { type: `https://eps.dev/problems/${code}`, title: code, status, code, requestId: "" });
}

export async function forwardToCore(req: Request, route: Route | null, target: string, opts: { organiser?: boolean } = {}) {
  if (!route) return problem(404, "not_found");
  const base = coreBaseUrl();
  if (!base) return problem(503, "EPS-UNAVAILABLE");
  let token = "";
  if (route.token === "account") {
    const tok = await issueCoreToken(
      { auth: getAuth(), memberships: sharedMembershipLookup(getAppPool()), secret: authSecret(), secureCookies: webOrigin().startsWith("https://") },
      { headers: req.headers, guest: false },
    );
    if (tok.status !== 200) return problem(401, "unauthenticated");
    if (opts.organiser && tok.kind !== "organiser") return problem(404, "EPS-EVENT-NOT-FOUND");
    token = tok.token;
  }
  const body = req.method === "GET" ? undefined : await req.text();
  if (body && body.length > 64_000) return problem(413, "invalid_request");
  const headers = new Headers({ Accept: "application/json" });
  if (token) headers.set("Authorization", `Bearer ${token}`);
  if (body) headers.set("Content-Type", "application/json");
  let res: Response;
  try {
    res = await fetch(`${base}${target}`, { method: req.method, headers, body, cache: "no-store", signal: AbortSignal.timeout(15_000) });
  } catch {
    return problem(503, "EPS-UNAVAILABLE");
  }
  const text = res.status === 204 ? "" : await res.text();
  let parsed: unknown = null;
  try {
    parsed = text ? JSON.parse(text) : null;
  } catch {
    parsed = { type: "", title: "bad gateway", status: 502, code: "EPS-UNAVAILABLE", requestId: "" };
  }
  return envelope(res.status, parsed);
}
