// Framing and content-security headers (review decision on step 2). Every page refuses to be framed
// except the builder's preview frame, which only its own origin may frame. Studio and storefront
// pages (Studio, preview links and step 3's /s/ shop) also get a Content-Security-Policy with a
// per-request nonce for scripts: only our own origin's images, fonts, styles and connections.
export function isFrameRoute(pathname: string): boolean {
  return pathname.startsWith("/studio/builder/frame/");
}

export function needsCsp(pathname: string): boolean {
  // Step 7: the fan's ticket pages, the signed guest link and the door page as well.
  return ["/studio", "/wallet", "/account", "/door"].some((p) => pathname === p || pathname.startsWith(`${p}/`)) || pathname.startsWith("/p/") || pathname.startsWith("/t/");
}

/** The public shop (/s/...): static pages cached at the edge, so a fixed policy without a nonce. */
export function isShopRoute(pathname: string): boolean {
  return pathname.startsWith("/s/");
}

/**
 * The shop's policy (design D3.6). Script files must come from our origin and carry Subresource
 * Integrity hashes (next.config experimental.sri). Next.js also writes inline data scripts (the
 * page's server-component payload) whose content changes with every page and version, so they
 * cannot be listed by hash in a static header: 'unsafe-inline' is allowed for scripts on shop
 * pages only. That is acceptable here because a shop renders nothing an organiser or fan can turn
 * into markup: every value is schema-validated JSON rendered as escaped React text, links are
 * restricted to https:, mailto: and site paths, and images to our own origin. Everything else stays
 * closed: no connections, images, fonts or frames from other origins, no framing, no <base>, no
 * form posts elsewhere.
 */
export function shopContentSecurityPolicy(opts: { vercelPreview?: boolean; dev?: boolean } = {}): string {
  const v = opts.vercelPreview ? " https://vercel.live" : "";
  return [
    "default-src 'self'",
    `script-src 'self' 'unsafe-inline'${opts.dev ? " 'unsafe-eval'" : ""}${v}`,
    `style-src 'self' 'unsafe-inline'${v}`,
    `img-src 'self' data: blob:${opts.vercelPreview ? " https://vercel.live https://vercel.com" : ""}`,
    `font-src 'self'${opts.vercelPreview ? " https://vercel.live https://assets.vercel.com" : ""}`,
    `connect-src 'self'${opts.vercelPreview ? " https://vercel.live wss://ws-us3.pusher.com" : ""}${opts.dev ? " ws:" : ""}`,
    // 'none' must stand alone in a directive; the Vercel preview toolbar needs its frame.
    `frame-src ${opts.vercelPreview ? v.trim() : "'none'"}`,
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "object-src 'none'",
  ].join("; ");
}

/** The policy for a page. `vercelPreview` allows the Vercel preview toolbar; `dev` allows eval (dev server). */
export function contentSecurityPolicy(nonce: string, opts: { frame: boolean; vercelPreview?: boolean; dev?: boolean; connect?: string[] }): string {
  const v = opts.vercelPreview ? " https://vercel.live" : "";
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${opts.dev ? " 'unsafe-eval'" : ""}${v}`,
    `style-src 'self' 'unsafe-inline'${v}`,
    `img-src 'self' data: blob:${opts.vercelPreview ? " https://vercel.live https://vercel.com" : ""}`,
    `font-src 'self'${opts.vercelPreview ? " https://vercel.live https://assets.vercel.com" : ""}`,
    `connect-src 'self'${(opts.connect ?? []).map((o) => ` ${o}`).join("")}${opts.vercelPreview ? " https://vercel.live wss://ws-us3.pusher.com" : ""}${opts.dev ? " ws:" : ""}`,
    `frame-src 'self'${v}`,
    `frame-ancestors ${opts.frame ? "'self'" : "'none'"}`,
    "base-uri 'self'",
    "form-action 'self'",
    "object-src 'none'",
  ].join("; ");
}

/**
 * The exact origins a Studio page may connect to besides its own: the live dashboard (step 6) opens
 * the environment's gateway directly. Only a bare http(s) origin is accepted.
 */
export function studioConnectOrigins(pathname: string, gateway: string | null): string[] {
  if (!gateway || !/^\/studio\/sales\/[0-9a-f-]{36}\/live$/.test(pathname)) return [];
  try {
    const u = new URL(gateway);
    return (u.protocol === "https:" || u.protocol === "http:") && !u.username && !u.password ? [u.origin] : [];
  } catch {
    return [];
  }
}
