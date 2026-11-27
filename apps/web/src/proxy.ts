// Runs before every page (Next.js 16 "proxy", formerly middleware). Two jobs (design D0.7):
// 1. Read the host: platform hosts route by path; any other host is passed on as a candidate brand
//    domain (resolved in step 3).
// 2. An optimistic Studio guard on the session cookie (the Studio layout re-checks the session).
import { NextResponse, type NextRequest } from "next/server";
import { getSessionCookie } from "better-auth/cookies";
import { classifyHost } from "@/lib/host";
import { signInRedirect } from "@/lib/auth/guard";

export function proxy(req: NextRequest) {
  const { pathname, search } = req.nextUrl;
  const target = signInRedirect(pathname, search, Boolean(getSessionCookie(req)));
  if (target) return NextResponse.redirect(new URL(target, req.url));

  const host = classifyHost(req.headers.get("host"), process.env.EPS_PLATFORM_HOSTS);
  const headers = new Headers(req.headers);
  headers.delete("x-eps-brand-host"); // never trust a client-supplied value
  if (host.kind === "brand") headers.set("x-eps-brand-host", host.host);
  return NextResponse.next({ request: { headers } });
}

export const config = {
  matcher: ["/((?!_next/|api/|favicon.ico).*)"],
};
