// Runs before every page (Next.js 16 "proxy", formerly middleware). Its jobs (design D0.7, D3.5):
// 1. Read the host: platform hosts route by path; any other host is a candidate brand hostname and
//    is served its shop at the root: /<locale>/... is rewritten to /s/~<host>/<locale>/..., and the
//    page looks the host up (cached; an unknown host gets the platform 404). The ~ form is refused
//    on platform hosts. No database call here.
// 2. /s/<brand> (and / on a brand host) redirects to the visitor's language: the static shop page
//    then redirects once more if the shop does not sell in it.
// 3. An optimistic Studio guard on the session cookie (the Studio layout re-checks the session).
// 4. Security headers: no framing (except the builder frame, same origin only); a nonce-based
//    Content-Security-Policy on Studio and preview links; a fixed one on the static shop pages.
import { NextResponse, type NextRequest } from "next/server";
import { getSessionCookie } from "better-auth/cookies";
import { classifyHost } from "@/lib/host";
import { signInRedirect } from "@/lib/auth/guard";
import { LOCALE_COOKIE, resolveLocale } from "@/i18n/locale";
import { contentSecurityPolicy, isFrameRoute, isShopRoute, needsCsp, shopContentSecurityPolicy, studioConnectOrigins } from "@/lib/security-headers";
import { gatewayBaseUrl } from "@/lib/core/url";
import { HOST_MARKER, hostRewrite, shopLocaleOfPath } from "@/lib/storefront/routing";

const NOT_FOUND = "/_eps/not-found"; // matches no route: the platform 404

export function proxy(req: NextRequest) {
  const { search } = req.nextUrl;
  const host = classifyHost(req.headers.get("host"), process.env.EPS_PLATFORM_HOSTS);
  // On a brand host every page is a page of its shop (Studio and sign-in stay on platform hosts).
  const rewritten = host.kind === "brand" ? hostRewrite(req.nextUrl.pathname, host.host) : null;
  const pathname = rewritten ?? req.nextUrl.pathname;
  if (host.kind === "platform") {
    if (pathname.startsWith(`/s/${HOST_MARKER}`) || pathname.startsWith(`/s/${encodeURIComponent(HOST_MARKER)}`)) {
      return NextResponse.rewrite(new URL(NOT_FOUND, req.url), { headers: baseHeaders(false) });
    }
    const target = signInRedirect(pathname, search, Boolean(getSessionCookie(req)));
    if (target) return NextResponse.redirect(new URL(target, req.url));
  }
  // A shop address without a language: the visitor's language (cookie, then browser).
  const bare = /^\/s\/([^/]+)\/?$/.exec(pathname);
  if (bare) {
    const locale = resolveLocale(req.cookies.get(LOCALE_COOKIE)?.value, req.headers.get("accept-language"));
    const to = host.kind === "brand" ? `/${locale}` : `/s/${bare[1]}/${locale}`;
    const res = NextResponse.redirect(new URL(`${to}${search}`, req.url), 307);
    res.headers.set("Vary", "Cookie, Accept-Language");
    res.headers.set("Cache-Control", "private, no-store");
    return res;
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
  headers.delete("content-security-policy");
  headers.delete("x-nonce");
  const env = { vercelPreview: process.env.VERCEL_ENV === "preview", dev: process.env.NODE_ENV === "development" };
  if (isShopRoute(pathname)) {
    // Static shop pages: a fixed policy, no nonce (it would make the page per-request).
    csp = shopContentSecurityPolicy(env);
  } else if (needsCsp(pathname)) {
    const nonce = btoa(crypto.randomUUID());
    csp = contentSecurityPolicy(nonce, { frame, ...env, connect: studioConnectOrigins(pathname, gatewayBaseUrl()) });
    // Next.js reads the nonce from the request's policy and puts it on its own scripts.
    headers.set("x-nonce", nonce);
    headers.set("content-security-policy", csp);
  }
  const res = rewritten ? NextResponse.rewrite(new URL(`${rewritten}${search}`, req.url), { request: { headers } }) : NextResponse.next({ request: { headers } });
  for (const [k, v] of Object.entries(baseHeaders(frame))) res.headers.set(k, v);
  if (csp) res.headers.set("Content-Security-Policy", csp);
  if (pathname.startsWith("/p/")) {
    res.headers.set("Cache-Control", "private, no-store");
    res.headers.set("X-Robots-Tag", "noindex, nofollow");
    res.headers.set("Referrer-Policy", "no-referrer");
  }
  return res;
}

function baseHeaders(frame: boolean): Record<string, string> {
  return {
    "Content-Security-Policy": `frame-ancestors ${frame ? "'self'" : "'none'"}`,
    "X-Frame-Options": frame ? "SAMEORIGIN" : "DENY",
    "X-Content-Type-Options": "nosniff",
  };
}

export function layoutFor(pathname: string): "bare" | "wide" | null {
  if (pathname.startsWith("/studio/builder/frame/") || pathname.startsWith("/p/") || pathname === "/s" || pathname.startsWith("/s/")) return "bare";
  if (pathname === "/studio/builder" || pathname.startsWith("/studio/builder/")) return "wide";
  return null;
}

export const config = {
  matcher: ["/((?!_next/|api/|favicon.ico).*)"],
};
