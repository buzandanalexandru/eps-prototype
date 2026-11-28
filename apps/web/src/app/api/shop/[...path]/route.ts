// /api/shop/<path>: the shop's checkout, order, payment and wallet calls, forwarded to the core's
// /v1/shop/<path> with the visitor's core token (lib/shop/proxy.ts); the waiting room's
// queue/<sale>/join and status go to the gateway's /v1/queue/... the same way (step 5). Never cached. The answer is
// always HTTP 200 with the core's status and body inside ({status, body}): a sold-out hold or a
// declined card is an expected outcome the page shows, not a browser network error.
import { NextResponse } from "next/server";
import { authSecret, webOrigin } from "@/lib/env";
import { issueCoreToken } from "@/lib/auth/core-token";
import { sharedMembershipLookup } from "@/lib/auth/membership";
import { getAuth } from "@/lib/auth/server";
import { getAppPool } from "@/lib/db/pool";
import { coreBaseUrl, gatewayBaseUrl } from "@/lib/core/url";
import { allowedShopRoute, forwardHeaders, forwardQuery, isQueueRoute } from "@/lib/shop/proxy";

export const dynamic = "force-dynamic";

type P = { params: Promise<{ path: string[] }> };

const noStore = { "Cache-Control": "no-store" };

function envelope(status: number, body: unknown) {
  return NextResponse.json({ status, body }, { headers: noStore });
}

function problem(status: number, code: string) {
  return envelope(status, { type: `https://eps.dev/problems/${code}`, title: code, status, code, requestId: "" });
}

async function forward(req: Request, { params }: P) {
  const path = (await params).path.join("/");
  if (!allowedShopRoute(req.method, path)) return problem(404, "not_found");
  // The waiting room (step 5) is served by the gateway; everything else by the core.
  const queue = isQueueRoute(req.method, path);
  const base = queue ? gatewayBaseUrl() : coreBaseUrl();
  if (!base) return problem(503, "EPS-UNAVAILABLE");
  // A waiting-room poll carries its queue token alone (step 5 review D-i): no core token, so no
  // session is read for the most frequent call of an on-sale.
  const poll = queue && req.method === "GET";
  const tok = poll
    ? { status: 200 as const, token: "", setCookie: undefined as string | undefined }
    : await issueCoreToken(
        { auth: getAuth(), memberships: sharedMembershipLookup(getAppPool()), secret: authSecret(), secureCookies: webOrigin().startsWith("https://") },
        { headers: req.headers, guest: true },
      );
  if (tok.status !== 200) return problem(401, "unauthenticated");
  const body = req.method === "GET" || req.method === "DELETE" ? undefined : await req.text();
  if (body && body.length > 32_000) return problem(413, "invalid_request");
  let res: Response;
  try {
    const target = queue ? `${base}/v1/${path}` : `${base}/v1/shop/${path}${forwardQuery(new URL(req.url).searchParams)}`;
    res = await fetch(target, {
      method: req.method,
      headers: forwardHeaders(req.headers, tok.token),
      body,
      cache: "no-store",
      signal: AbortSignal.timeout(20_000),
    });
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
  const out = envelope(res.status, parsed);
  if (tok.setCookie) out.headers.append("Set-Cookie", tok.setCookie);
  return out;
}

export const GET = forward;
export const POST = forward;
export const PUT = forward;
export const DELETE = forward;
