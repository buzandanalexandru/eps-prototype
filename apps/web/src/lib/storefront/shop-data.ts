// The app's cached shop loader (server-only): Next.js's data cache, tagged per shop address and
// revalidated by publish (design D3.6). Entries never expire on their own.
import "server-only";
import { revalidateTag, unstable_cache } from "next/cache";
import { getAppPool } from "../db/pool";
import { withTx } from "../db/tx";
import { loadPublishedShop } from "./published";
import { shopLoader, type CacheFn } from "./shop-cache";

// Vercel's data cache outlives a deployment and is shared by every preview: the key names this
// deployment, so a shop never shows data another deployment (another branch's database, an older
// schema) cached under the same address.
const DEPLOYMENT = process.env.VERCEL_DEPLOYMENT_ID || process.env.VERCEL_GIT_COMMIT_SHA || "local";

const nextCache: CacheFn = (key, tags, fn, seconds) => unstable_cache(fn, [DEPLOYMENT, ...key], { tags, revalidate: seconds ?? false })();

export const getPublishedShop = shopLoader(nextCache, (k) => withTx(getAppPool(), {}, (c) => loadPublishedShop(c, k)));

/** After a publish: the next request to any of the brand's shop addresses renders the new version. */
export function revalidateShop(tags: string[]): void {
  for (const t of tags) revalidateTag(t, { expire: 0 });
}
