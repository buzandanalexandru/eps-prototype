// Studio's publish with sale snapshots (design D3.1): the hook asks the core for pool usage with the
// organiser's own core token, and after commit the core is told the new versions and the shop's
// cache is revalidated. Server-only.
import "server-only";
import { headers } from "next/headers";
import { issueCoreToken } from "../auth/core-token";
import { sharedMembershipLookup } from "../auth/membership";
import { getAuth } from "../auth/server";
import { getAppPool } from "../db/pool";
import { authSecret, webOrigin } from "../env";
import { publishTags } from "../storefront/shop-cache";
import { revalidateShop } from "../storefront/shop-data";
import { activateAll, coreUsage } from "./core";
import { snapshotHook, type CompiledEvent } from "./publish";

/** A core token for the signed-in organiser of this request, or null. */
export async function organiserCoreToken(): Promise<string | null> {
  const res = await issueCoreToken(
    { auth: getAuth(), memberships: sharedMembershipLookup(getAppPool()), secret: authSecret(), secureCookies: webOrigin().startsWith("https://") },
    { headers: await headers(), guest: false },
  );
  return res.status === 200 && res.kind === "organiser" ? res.token : null;
}

/** A snapshot hook for one Studio publish, and what to do after it committed. */
export async function studioSnapshotPublish(userId: string) {
  const token = await organiserCoreToken();
  let compiled: CompiledEvent[] = [];
  const hook = snapshotHook({
    userId,
    usage: token
      ? coreUsage({ token })
      : async () => {
          throw new Error("no organiser core token");
        },
    onCompiled: (e) => (compiled = e),
  });
  return {
    hook,
    /** After commit: activation (best effort) and cache revalidation for every shop address. */
    async after(shop: { brandId: string; slug: string; hostnames: string[] }) {
      const created = compiled.filter((e) => e.created);
      const activated = token && created.length ? await activateAll({ token }, created) : [];
      revalidateShop(publishTags({ ...shop, eventIds: compiled.map((e) => e.eventId) }));
      return { snapshots: compiled.map((e) => ({ eventId: e.eventId, version: e.version, created: e.created })), activated };
    },
  };
}
