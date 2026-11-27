// GET /api/core-token[?guest=1]: a short-lived core token for direct calls from the browser to the
// Go gateway (step 5). Never cached.
import { NextResponse } from "next/server";
import { authSecret, webOrigin } from "@/lib/env";
import { issueCoreToken } from "@/lib/auth/core-token";
import { sharedMembershipLookup } from "@/lib/auth/membership";
import { getAuth } from "@/lib/auth/server";
import { getAppPool } from "@/lib/db/pool";

export const dynamic = "force-dynamic";

export async function GET(req: Request) {
  const guest = new URL(req.url).searchParams.get("guest") === "1";
  const res = await issueCoreToken(
    {
      auth: getAuth(),
      memberships: sharedMembershipLookup(getAppPool()),
      secret: authSecret(),
      secureCookies: webOrigin().startsWith("https://"),
    },
    { headers: req.headers, guest },
  );
  const noStore = { "Cache-Control": "no-store" };
  if (res.status === 401) {
    return NextResponse.json(
      { type: "https://eps.dev/problems/unauthenticated", title: "Unauthorized", status: 401, code: "unauthenticated" },
      { status: 401, headers: { ...noStore, "Content-Type": "application/problem+json" } },
    );
  }
  const out = NextResponse.json({ token: res.token, kind: res.kind, expiresAt: res.expiresAt }, { headers: noStore });
  if (res.setCookie) out.headers.append("Set-Cookie", res.setCookie);
  return out;
}
