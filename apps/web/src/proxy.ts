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
  // Page chrome (step 2): the builder frame and preview links render the shop alone; the builder is wide.
  headers.delete("x-eps-layout");
  const layout = layoutFor(pathname);
  if (layout) headers.set("x-eps-layout", layout);
  const res = NextResponse.next({ request: { headers } });
  if (pathname.startsWith("/p/")) {
    res.headers.set("Cache-Control", "private, no-store");
    res.headers.set("X-Robots-Tag", "noindex, nofollow");
    res.headers.set("Referrer-Policy", "no-referrer");
  }
  if (pathname.startsWith("/studio/builder/frame/")) res.headers.set("X-Frame-Options", "SAMEORIGIN");
  return res;
}

export function layoutFor(pathname: string): "bare" | "wide" | null {
  if (pathname.startsWith("/studio/builder/frame/") || pathname.startsWith("/p/")) return "bare";
  if (pathname === "/studio/builder" || pathname.startsWith("/studio/builder/")) return "wide";
  return null;
}

export const config = {
  matcher: ["/((?!_next/|api/|favicon.ico).*)"],
};
