// Reading the core's availability answer against the page (design D3.6): an answer for another
// snapshot version than the cached page's (a publish in flight, either way) is never mixed with the
// page's prices: the page's own facts apply until the two agree. Pure, shared by the island and tests.
import type { ShopEvent } from "./catalogue";
import type { AvailabilityState } from "./picker";
import { priceAt, statusByTime, type Availability, type InstanceAvailability } from "./sale-view";

export type Load = { state: "loading" } | { state: "ok"; data: AvailabilityState } | { state: "stale" } | { state: "failed" };

export type Answer = { version?: unknown; products?: unknown; instances?: unknown; degraded?: unknown };

/** The island's state for an answer body (null: no usable answer). */
export function readAnswer(body: Answer | null, version: number): Load {
  if (!body || !Array.isArray(body.products)) return { state: "failed" };
  if (body.version !== version) return { state: "stale" };
  const products = body.products as (Availability & { productId: string })[];
  const instances = (Array.isArray(body.instances) ? body.instances : []) as InstanceAvailability[];
  return {
    state: "ok",
    data: {
      products: Object.fromEntries(products.map((p) => [p.productId, p])),
      instances: Object.fromEntries(instances.map((i) => [i.instanceId, i])),
      ...(body.degraded === true ? { degraded: true } : {}),
    },
  };
}

/** The availability the picker uses: the core's answer, the page's own facts when versions differ, or none. */
export function availabilityFor(load: Load, ev: ShopEvent, now: Date): AvailabilityState | null {
  if (load.state === "ok") return load.data;
  if (load.state !== "stale") return null;
  return {
    products: Object.fromEntries(
      ev.products.map((p) => {
        const price = priceAt(p, now);
        return [p.id, { status: statusByTime(p.sale, now), allInMinor: price.allInMinor, priceStepIndex: price.stepIndex }];
      }),
    ),
    instances: {},
  };
}
