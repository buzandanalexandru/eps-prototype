// Who may change a storefront: owners and admins design the shop; box office and finance see it.
import { DomainError } from "../errors";

export function assertCanDesign(role: string): void {
  if (role !== "owner" && role !== "admin") throw new DomainError("designer_only");
}

/** Route handlers with cookie sign-in accept writes only from pages of the same origin. */
export function sameOrigin(req: Request): boolean {
  const origin = req.headers.get("origin");
  const site = req.headers.get("sec-fetch-site");
  if (site && site !== "same-origin") return false;
  if (!origin) return site === "same-origin";
  const host = req.headers.get("x-forwarded-host") ?? req.headers.get("host");
  try {
    const o = new URL(origin);
    return o.host === host || o.origin === new URL(req.url).origin;
  } catch {
    return false;
  }
}
