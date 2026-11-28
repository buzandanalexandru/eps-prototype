// The core's side of a publish (design D3.1): the pool-usage gate before commit and the activation
// calls after it, through the typed client generated from api/openapi.yaml. Server-only (it holds
// an organiser core token).
import { coreClient } from "../core/client";
import type { UsageSource } from "./publish";

type CoreOpts = { token: string; baseUrl?: string; fetch?: typeof fetch; timeoutMs?: number };

function client(o: CoreOpts) {
  const c = coreClient({ token: o.token, baseUrl: o.baseUrl, fetch: o.fetch });
  if (!c) throw new Error("core address unknown");
  return c;
}

/** Units sold or held per pool instance, asked of the core for each event; throws if it cannot tell. */
export function coreUsage(o: CoreOpts): UsageSource {
  return async (ids) => {
    const c = client(o);
    const out = new Map<string, Record<string, number>>();
    await Promise.all(
      ids.map(async (id) => {
        const r = await c.GET("/v1/organiser/events/{eventId}/pool-usage", { params: { path: { eventId: id } }, signal: AbortSignal.timeout(o.timeoutMs ?? 4000) });
        if (!r.response.ok || !r.data) throw new Error(`pool usage for ${id}: ${r.response.status}`);
        const used: Record<string, number> = {};
        for (const i of r.data.instances.slice(0, 100_000)) if (typeof i.instanceId === "string" && Number.isSafeInteger(i.used)) used[i.instanceId] = i.used;
        out.set(id, used);
      }),
    );
    return out;
  };
}

/**
 * Tells the core which versions are live (after commit). Best effort: a lost call is healed by the
 * core's cache expiry within seconds, so failures are reported, never thrown.
 */
export async function activateAll(o: CoreOpts, events: { eventId: string; version: number }[]): Promise<{ eventId: string; ok: boolean }[]> {
  let c: ReturnType<typeof client>;
  try {
    c = client(o);
  } catch {
    return events.map((e) => ({ eventId: e.eventId, ok: false }));
  }
  return Promise.all(
    events.map(async (e) => {
      try {
        const r = await c.POST("/v1/organiser/events/{eventId}/snapshots/{version}/activate", {
          params: { path: { eventId: e.eventId, version: e.version } },
          signal: AbortSignal.timeout(o.timeoutMs ?? 4000),
        });
        return { eventId: e.eventId, ok: r.response.ok };
      } catch {
        return { eventId: e.eventId, ok: false };
      }
    }),
  );
}
