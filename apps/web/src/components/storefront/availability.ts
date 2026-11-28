"use client";
// The availability island's data (design D3.6): one poll per event shared by every component on the
// page (ticket grid, "from" price). The core is asked through our own origin. An answer for another
// snapshot version than the page's (a publish in flight) is not mixed with this page's prices: the
// page's own facts are used and the core is asked again soon. Failures back off with jitter, from
// 10 seconds to a minute, so a crowd does not retry in step.
import { useEffect, useState, useSyncExternalStore } from "react";
import type { AvailabilityState } from "@/lib/storefront/picker";
import type { Availability, InstanceAvailability } from "@/lib/storefront/sale-view";

export type Load =
  | { state: "loading" }
  | { state: "ok"; data: AvailabilityState }
  /** The core answered for another snapshot version: the page's own prices and statuses apply. */
  | { state: "stale" }
  | { state: "failed" };

type Entry = { load: Load; listeners: Set<() => void>; timer?: ReturnType<typeof setTimeout>; failures: number; running: boolean };
const entries = new Map<string, Entry>();
const LOADING: Load = { state: "loading" };

type Body = {
  version?: number;
  products?: (Availability & { productId: string })[];
  instances?: InstanceAvailability[];
};

function jitter(ms: number) {
  return Math.round(ms * (0.75 + Math.random() * 0.5));
}

function entryFor(eventId: string, version: number): Entry {
  const key = `${eventId}@${version}`;
  let e = entries.get(key);
  if (!e) {
    e = { load: LOADING, listeners: new Set(), failures: 0, running: false };
    entries.set(key, e);
  }
  return e;
}

async function poll(eventId: string, version: number, e: Entry) {
  let next: Load = { state: "failed" };
  try {
    const res = await fetch(`/api/public/events/${eventId}/availability`, { headers: { accept: "application/json" } });
    if (res.ok) {
      const body = (await res.json()) as Body;
      if (body.version !== version) next = { state: "stale" };
      else if (Array.isArray(body.products)) {
        next = {
          state: "ok",
          data: {
            products: Object.fromEntries(body.products.map((p) => [p.productId, p])),
            instances: Object.fromEntries((body.instances ?? []).map((i) => [i.instanceId, i])),
          },
        };
      }
    }
  } catch {
    // network failure: buying stays paused, the page keeps its prices
  }
  e.load = next;
  e.failures = next.state === "ok" ? 0 : e.failures + 1;
  for (const l of e.listeners) l();
  if (!e.listeners.size) {
    e.running = false;
    return;
  }
  const wait = next.state === "ok" ? 30_000 : next.state === "stale" ? 3_000 : Math.min(60_000, 10_000 * 2 ** Math.min(e.failures - 1, 3));
  e.timer = setTimeout(() => void poll(eventId, version, e), jitter(wait));
}

export function useAvailability(eventId: string, version: number): Load {
  const e = entryFor(eventId, version);
  const load = useSyncExternalStore(
    (cb) => {
      e.listeners.add(cb);
      if (!e.running) {
        e.running = true;
        void poll(eventId, version, e);
      }
      return () => {
        e.listeners.delete(cb);
        if (!e.listeners.size && e.timer) {
          clearTimeout(e.timer);
          e.running = false;
        }
      };
    },
    () => e.load,
    () => LOADING,
  );
  return load;
}

/** The present, after hydration (the first render uses the page's render time, so nothing mismatches). */
export function useNow(renderedAt: string): Date {
  const [now, setNow] = useState(() => new Date(renderedAt));
  useEffect(() => {
    setNow(new Date());
    const t = setInterval(() => setNow(new Date()), 60_000);
    return () => clearInterval(t);
  }, []);
  return now;
}
