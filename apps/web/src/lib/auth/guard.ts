// Which routes need a session (design D0.4 / identity spec): Studio does; storefronts, status and
// sign-in do not.

export function isProtectedPath(pathname: string): boolean {
  return pathname === "/studio" || pathname.startsWith("/studio/");
}

/** Only same-site relative paths are allowed as a return target. */
export function safeNext(next: string | null | undefined, fallback = "/studio"): string {
  if (!next || !next.startsWith("/") || next.startsWith("//") || next.startsWith("/\\")) return fallback;
  return next;
}

/** Where to send a request, or null to let it through. */
export function signInRedirect(pathname: string, search: string, hasSession: boolean): string | null {
  if (!isProtectedPath(pathname) || hasSession) return null;
  return `/sign-in?next=${encodeURIComponent(pathname + search)}`;
}
