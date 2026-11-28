// Framing and content-security headers (review decision on step 2). Every page refuses to be framed
// except the builder's preview frame, which only its own origin may frame. Studio and storefront
// pages (Studio, preview links and step 3's /s/ shop) also get a Content-Security-Policy with a
// per-request nonce for scripts: only our own origin's images, fonts, styles and connections.
export function isFrameRoute(pathname: string): boolean {
  return pathname.startsWith("/studio/builder/frame/");
}

export function needsCsp(pathname: string): boolean {
  return pathname === "/studio" || pathname.startsWith("/studio/") || pathname.startsWith("/p/") || pathname.startsWith("/s/");
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
