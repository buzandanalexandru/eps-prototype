// Caching the public shop's data (design D3.6): one entry per shop address (slug or hostname), kept
// until the brand's next publish, so a shop page on a cache hit makes no database query. The cache
// itself is injected: Next.js's data cache (unstable_cache, tagged) in the app, a map in tests.
import { normaliseKey, type PublishedShop, type ShopKey } from "./published";

/** Runs `fn` once per key until one of `tags` is revalidated. */
export type CacheFn = <T>(key: string[], tags: string[], fn: () => Promise<T>) => Promise<T>;

export const slugTag = (slug: string) => `shop:slug:${slug}`;
export const hostTag = (host: string) => `shop:host:${host}`;
export const brandTag = (brandId: string) => `brand:${brandId}`;
export const eventTag = (eventId: string) => `event:${eventId}`;

/** Every tag a publish of this brand must revalidate: its slug, its hostnames, the brand and its events. */
export function publishTags(p: { brandId: string; slug: string; hostnames: string[]; eventIds: string[]; previousSlug?: string | null }): string[] {
  return [
    slugTag(p.slug),
    ...(p.previousSlug && p.previousSlug !== p.slug ? [slugTag(p.previousSlug)] : []),
    ...p.hostnames.map(hostTag),
    brandTag(p.brandId),
    ...p.eventIds.map(eventTag),
  ];
}

/** A cached shop loader: `load` runs on a miss only. Unknown and malformed keys never query. */
export function shopLoader(cache: CacheFn, load: (k: ShopKey) => Promise<PublishedShop | null>) {
  return async (key: ShopKey): Promise<PublishedShop | null> => {
    const k = normaliseKey(key);
    if (!k) return null;
    const id = "slug" in k ? ["shop", "slug", k.slug] : ["shop", "host", k.host];
    const tags = ["slug" in k ? slugTag(k.slug) : hostTag(k.host)];
    return cache(id, tags, () => load(k));
  };
}

/** An in-memory CacheFn with tag invalidation (tests, and a fallback outside Next.js). */
export function memoryCache(): CacheFn & { revalidate(tag: string): void } {
  const entries = new Map<string, { tags: string[]; value: Promise<unknown> }>();
  const fn = (<T,>(key: string[], tags: string[], run: () => Promise<T>): Promise<T> => {
    const k = JSON.stringify(key);
    const hit = entries.get(k);
    if (hit) return hit.value as Promise<T>;
    const value = run();
    entries.set(k, { tags, value });
    value.catch(() => entries.delete(k)); // failures are not cached
    return value;
  }) as CacheFn & { revalidate(tag: string): void };
  fn.revalidate = (tag: string) => {
    for (const [k, e] of entries) if (e.tags.includes(tag)) entries.delete(k);
  };
  return fn;
}
