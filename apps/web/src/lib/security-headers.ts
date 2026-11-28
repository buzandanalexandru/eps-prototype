// Framing and content-security headers (review decision on step 2). Every page refuses to be framed
// except the builder's preview frame, which only its own origin may frame. Studio and storefront
// pages (Studio, preview links and step 3's /s/ shop) also get a Content-Security-Policy with a
// per-request nonce for scripts: only our own origin's images, fonts, styles and connections.
export function isFrameRoute(pathname: string): boolean {
  return pathname.startsWith("/studio/builder/frame/");
}

export function needsCsp(pathname: string): boolean {
  return pathname === "/studio" || pathname.startsWith("/studio/") || pathname.startsWith("/p/");
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
    `frame-src 'none'${v}`,
    "frame-ancestors 'none'",
    "base-uri 'self'",
    "form-action 'self'",
    "object-src 'none'",
  ].join("; ");
}

/** The policy for a page. `vercelPreview` allows the Vercel preview toolbar; `dev` allows eval (dev server). */
export function contentSecurityPolicy(nonce: string, opts: { frame: boolean; vercelPreview?: boolean; dev?: boolean }): string {
  const v = opts.vercelPreview ? " https://vercel.live" : "";
  return [
    "default-src 'self'",
    `script-src 'self' 'nonce-${nonce}' 'strict-dynamic'${opts.dev ? " 'unsafe-eval'" : ""}${v}`,
    `style-src 'self' 'unsafe-inline'${v}`,
    `img-src 'self' data: blob:${opts.vercelPreview ? " https://vercel.live https://vercel.com" : ""}`,
    `font-src 'self'${opts.vercelPreview ? " https://vercel.live https://assets.vercel.com" : ""}`,
    `connect-src 'self'${opts.vercelPreview ? " https://vercel.live wss://ws-us3.pusher.com" : ""}${opts.dev ? " ws:" : ""}`,
    `frame-src 'self'${v}`,
    `frame-ancestors ${opts.frame ? "'self'" : "'none'"}`,
    "base-uri 'self'",
    "form-action 'self'",
    "object-src 'none'",
  ].join("; ");
}
