// Runs before every page (Next.js 16 "proxy", formerly middleware). Two jobs (design D0.7):
// 1. Read the host: platform hosts route by path; any other host is a candidate brand hostname and
//    is served its shop at the root (step 3): /<locale>/... is rewritten to /s/~<host>/<locale>/...,
//    and the page looks the host up (cached; an unknown host gets the platform 404).
// 2. An optimistic Studio guard on the session cookie (the Studio layout re-checks the session).
// 3. Security headers: no framing (except the builder frame, same origin only) and a nonce-based
//    Content-Security-Policy on Studio and storefront pages.
import { NextResponse, type NextRequest } from "next/server";
import { getSessionCookie } from "better-auth/cookies";
import { classifyHost } from "@/lib/host";
import { signInRedirect } from "@/lib/auth/guard";
import { contentSecurityPolicy, isFrameRoute, needsCsp } from "@/lib/security-headers";
import { hostRewrite, shopLocaleOfPath } from "@/lib/storefront/routing";

export function proxy(req: NextRequest) {
  const { search } = req.nextUrl;
  const host = classifyHost(req.headers.get("host"), process.env.EPS_PLATFORM_HOSTS);
  // On a brand host every page is a page of its shop (Studio and sign-in stay on platform hosts).
  const rewritten = host.kind === "brand" ? hostRewrite(req.nextUrl.pathname, host.host) : null;
  const pathname = rewritten ?? req.nextUrl.pathname;
  if (host.kind === "platform") {
    const target = signInRedirect(pathname, search, Boolean(getSessionCookie(req)));
    if (target) return NextResponse.redirect(new URL(target, req.url));
  }

  const headers = new Headers(req.headers);
  headers.delete("x-eps-brand-host"); // never trust a client-supplied value
  headers.delete("x-eps-shop-locale");
  if (host.kind === "brand") headers.set("x-eps-brand-host", host.host);
  const shopLocale = shopLocaleOfPath(pathname);
  if (shopLocale) headers.set("x-eps-shop-locale", shopLocale);
  // Page chrome (step 2): the builder frame and preview links render the shop alone; the builder is wide.
  headers.delete("x-eps-layout");
  const layout = layoutFor(pathname);
  if (layout) headers.set("x-eps-layout", layout);
  // Images and map tiles under /assets set their own stricter headers.
  if (pathname.startsWith("/assets/") || pathname.startsWith("/samples/")) return NextResponse.next({ request: { headers } });
  const frame = isFrameRoute(pathname);
  let csp: string | null = null;
  if (needsCsp(pathname)) {
    const nonce = btoa(crypto.randomUUID());
    csp = contentSecurityPolicy(nonce, { frame, vercelPreview: process.env.VERCEL_ENV === "preview", dev: process.env.NODE_ENV === "development" });
    // Next.js reads the nonce from the request's policy and puts it on its own scripts.
    headers.set("x-nonce", nonce);
    headers.set("content-security-policy", csp);
  }
  const res = rewritten ? NextResponse.rewrite(new URL(`${rewritten}${search}`, req.url), { request: { headers } }) : NextResponse.next({ request: { headers } });
  if (csp) res.headers.set("Content-Security-Policy", csp);
  else res.headers.set("Content-Security-Policy", `frame-ancestors ${frame ? "'self'" : "'none'"}`);
  res.headers.set("X-Frame-Options", frame ? "SAMEORIGIN" : "DENY");
  res.headers.set("X-Content-Type-Options", "nosniff");
  if (pathname.startsWith("/p/")) {
    res.headers.set("Cache-Control", "private, no-store");
    res.headers.set("X-Robots-Tag", "noindex, nofollow");
    res.headers.set("Referrer-Policy", "no-referrer");
  }
  return res;
}

export function layoutFor(pathname: string): "bare" | "wide" | null {
  if (pathname.startsWith("/studio/builder/frame/") || pathname.startsWith("/p/") || pathname === "/s" || pathname.startsWith("/s/")) return "bare";
  if (pathname === "/studio/builder" || pathname.startsWith("/studio/builder/")) return "wide";
  return null;
}

export const config = {
  matcher: ["/((?!_next/|api/|favicon.ico).*)"],
};
