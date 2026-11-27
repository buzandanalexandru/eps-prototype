// Capacity pools (design D1.4, C4; pretix quota semantics): a product is available only while every
// pool it draws on has room, so its available quantity is the smallest remaining amount of its pools.
// Day- and slot-scoped pools hold one counter per day or per slot instance.
import type { PoolScope } from "./schema";

export type PoolLite = { id: string; name: string; capacity: number; scope: PoolScope };
export type Link = { product_id: string; pool_id: string };

/** Instance id of a pool counter (spec map): <poolId>, <poolId>@<date>, <poolId>@<date>T<HH:MM>. */
export function poolInstanceId(pool: { id: string; scope: PoolScope }, day?: string, slot?: string): string {
  if (pool.scope === "event") return pool.id;
  if (!day) throw new Error(`pool ${pool.id} is ${pool.scope}-scoped: a day is needed`);
  if (pool.scope === "day") return `${pool.id}@${day}`;
  if (!slot) throw new Error(`pool ${pool.id} is slot-scoped: a slot is needed`);
  return `${pool.id}@${day}T${slot}`;
}

/**
 * Remaining room in one pool instance. Sold and held counts come from the core at sale time (none in
 * Studio). Without a day (or slot) a scoped pool answers for one fresh instance: its capacity.
 */
export function remaining(pool: PoolLite, usedByInstance: Record<string, number>, day?: string, slot?: string): number {
  if ((pool.scope === "day" && !day) || (pool.scope === "slot" && (!day || !slot))) return pool.capacity;
  return Math.max(0, pool.capacity - (usedByInstance[poolInstanceId(pool, day, slot)] ?? 0));
}

/**
 * How many of a product can still be sold (for a day / slot when its pools are scoped): the minimum
 * remaining over the pools it draws on; 0 for a product that draws on no pool.
 */
export function availableQuantity(
  productId: string,
  pools: PoolLite[],
  links: Link[],
  usedByInstance: Record<string, number> = {},
  at: { day?: string; slot?: string } = {},
): number {
  const mine = links.filter((l) => l.product_id === productId).map((l) => pools.find((p) => p.id === l.pool_id)).filter((p): p is PoolLite => !!p);
  if (mine.length === 0) return 0;
  return Math.min(...mine.map((p) => remaining(p, usedByInstance, at.day, at.slot)));
}

/** Per pool: the products drawing on it and the combined capacity (the Studio pool bar). */
export function poolBars<P extends { id: string }>(pools: PoolLite[], links: Link[], products: P[]) {
  return pools.map((pool) => ({
    pool,
    products: links.filter((l) => l.pool_id === pool.id).map((l) => products.find((p) => p.id === l.product_id)).filter((p): p is P => !!p),
  }));
}

/** Number of counters a pool expands into (step 3): 1, one per day, or one per day × slot start. */
export function instanceCount(pool: PoolLite, days: number, slotsPerDay: number): number {
  return pool.scope === "event" ? 1 : pool.scope === "day" ? days : days * slotsPerDay;
}
