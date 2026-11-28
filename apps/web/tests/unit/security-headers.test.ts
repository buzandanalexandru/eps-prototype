// Review decision on step 2: no page can be framed except the builder frame (same origin only);
// Studio and storefront pages carry a nonce-based Content-Security-Policy.
import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";
import { proxy } from "@/proxy";

function run(path: string, cookie = "better-auth.session_token=x") {
  return proxy(new NextRequest(`http://localhost:3000${path}`, { headers: { cookie, host: "localhost:3000" } }));
}

describe("security headers", () => {
  it("Studio pages: DENY framing and a nonce CSP allowing only our own origin", () => {
    const res = run("/studio/builder");
    expect(res.headers.get("x-frame-options")).toBe("DENY");
    const csp = res.headers.get("content-security-policy")!;
    expect(csp).toMatch(/frame-ancestors 'none'/);
    expect(csp).toMatch(/script-src 'self' 'nonce-[A-Za-z0-9+/=]+' 'strict-dynamic'/);
    expect(csp).toMatch(/img-src 'self' data: blob:/);
    expect(csp).not.toMatch(/openstreetmap/);
  });

  it("the builder frame may be framed by its own origin only", () => {
    const res = run("/studio/builder/frame/00000000-0000-4000-8000-000000000000");
    expect(res.headers.get("x-frame-options")).toBe("SAMEORIGIN");
    expect(res.headers.get("content-security-policy")).toMatch(/frame-ancestors 'self'/);
  });

  it("preview links: CSP, no framing, no caching, no indexing; other pages are not frameable either", () => {
    const res = run("/p/AAAAAAAAAAAAAAAAAAAAAA", "");
    expect(res.headers.get("x-frame-options")).toBe("DENY");
    expect(res.headers.get("content-security-policy")).toMatch(/default-src 'self'/);
    expect(res.headers.get("cache-control")).toBe("private, no-store");
    expect(res.headers.get("x-robots-tag")).toMatch(/noindex/);
    const home = run("/", "");
    expect(home.headers.get("x-frame-options")).toBe("DENY");
    expect(home.headers.get("content-security-policy")).toBe("frame-ancestors 'none'");
  });

  it("each request gets a fresh nonce", () => {
    const a = run("/studio").headers.get("content-security-policy");
    const b = run("/studio").headers.get("content-security-policy");
    expect(a).not.toBe(b);
  });
});
