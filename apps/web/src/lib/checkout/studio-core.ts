// Studio's calls to the core for step 4 (provider knobs, stand payments, season close), with the
// signed-in organiser's own core token. Server-only.
import "server-only";
import { coreBaseUrl } from "../core/url";
import { organiserCoreToken } from "../snapshot/studio";

export type CoreAnswer<T> = { ok: true; data: T } | { ok: false; status: number; code: string };

export async function organiserCore<T>(method: "GET" | "PUT" | "POST", path: string, body?: unknown): Promise<CoreAnswer<T>> {
  const base = coreBaseUrl();
  const token = await organiserCoreToken();
  if (!base || !token) return { ok: false, status: 503, code: "EPS-UNAVAILABLE" };
  try {
    const res = await fetch(`${base}${path}`, {
      method,
      headers: { Authorization: `Bearer ${token}`, ...(body === undefined ? {} : { "Content-Type": "application/json" }) },
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

export type ProviderState = {
  provider: "a" | "b";
  state: "closed" | "open" | "half_open";
  calls?: number;
  failures?: number;
  reachable?: boolean;
  knobs?: { failure_rate?: number; decline_rate?: number; latency_ms?: number; latency_jitter_ms?: number; outage?: boolean; outage_mode?: string; timeout_rate?: number; webhook_delay_ms?: number; webhook_drop_rate?: number; challenge_rate?: number };
};
