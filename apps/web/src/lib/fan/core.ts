// Server-side calls of the fan's pages (trip wallet, account) to the core with the signed-in user's
// own core token. Server-only.
import "server-only";
import { headers } from "next/headers";
import { authSecret, webOrigin } from "../env";
import { issueCoreToken } from "../auth/core-token";
import { sharedMembershipLookup } from "../auth/membership";
import { getAuth } from "../auth/server";
import { getAppPool } from "../db/pool";
import { coreBaseUrl } from "../core/url";
import { withTx } from "../db/tx";

export type CoreAnswer<T> = { ok: true; data: T } | { ok: false; status: number; code: string };

export async function fanCore<T>(method: "GET" | "POST" | "PUT", path: string, body?: unknown): Promise<CoreAnswer<T>> {
  const base = coreBaseUrl();
  if (!base) return { ok: false, status: 503, code: "EPS-UNAVAILABLE" };
  const tok = await issueCoreToken(
    { auth: getAuth(), memberships: sharedMembershipLookup(getAppPool()), secret: authSecret(), secureCookies: webOrigin().startsWith("https://") },
    { headers: await headers(), guest: false },
  );
  if (tok.status !== 200) return { ok: false, status: 401, code: "unauthenticated" };
  try {
    const res = await fetch(`${base}${path}`, {
      method,
      headers: { Authorization: `Bearer ${tok.token}`, Accept: "application/json", ...(body === undefined ? {} : { "Content-Type": "application/json" }) },
      body: body === undefined ? undefined : JSON.stringify(body),
      cache: "no-store",
      signal: AbortSignal.timeout(8000),
    });
    const json = await res.json().catch(() => null);
    if (res.ok) return { ok: true, data: json as T };
    return { ok: false, status: res.status, code: (json && typeof json.code === "string" && json.code) || "EPS-UNAVAILABLE" };
  } catch {
    return { ok: false, status: 503, code: "EPS-UNAVAILABLE" };
  }
}

export type BrandLook = { brandId: string; slug: string; name: string; primary: string; onPrimary: string; surface: string; onSurface: string; logo: string | null };

/** Each brand's name, slug and published colours (for the trip wallet's event cards). */
export async function brandLooks(ids: string[]): Promise<Record<string, BrandLook>> {
  const uniq = [...new Set(ids.filter((i) => /^[0-9a-f-]{36}$/.test(i)))].slice(0, 50);
  if (!uniq.length) return {};
  const { imageUrl } = await import("../storefront/links");
  const rows = await withTx(getAppPool(), {}, async (c) =>
    (await c.query<{ brand_id: string; brand_slug: string; brand_name: string; tokens: { light?: Record<string, string>; logo?: string } | null }>(
      "select brand_id::text, brand_slug, brand_name, tokens from control.public_brand_looks($1::uuid[])",
      [uniq],
    )).rows,
  );
  const hex = (v: unknown, d: string) => (typeof v === "string" && /^#[0-9a-fA-F]{6}$/.test(v) ? v : d);
  const out: Record<string, BrandLook> = {};
  for (const r of rows) {
    const l = r.tokens?.light ?? {};
    out[r.brand_id] = {
      brandId: r.brand_id,
      slug: r.brand_slug,
      name: r.brand_name,
      primary: hex(l.primary, "#1f2937"),
      onPrimary: hex(l.onPrimary, "#ffffff"),
      surface: hex(l.surface, "#ffffff"),
      onSurface: hex(l.onSurface, "#111827"),
      logo: r.tokens?.logo ? imageUrl(r.tokens.logo) : null,
    };
  }
  return out;
}
