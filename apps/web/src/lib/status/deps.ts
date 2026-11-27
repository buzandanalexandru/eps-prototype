import "server-only";
import { headers } from "next/headers";
import { authSecret, webCommit, webOrigin } from "../env";
import { issueCoreToken } from "../auth/core-token";
import { dbMembershipLookup } from "../auth/membership";
import { getAuth } from "../auth/server";
import { coreClient } from "../core/client";
import { coreBaseUrl } from "../core/url";
import { getAppPool, hasDatabase } from "../db/pool";
import { withTx } from "../db/tx";
import type { StatusDeps } from "./loader";

export async function statusDeps(): Promise<StatusDeps> {
  const h = await headers();
  const db = hasDatabase();
  return {
    webCommit: webCommit(),
    coreUrl: coreBaseUrl(),
    readSchemaVersions: db
      ? () =>
          withTx(getAppPool(), {}, async (c) => {
            const r = await c.query<{ control: string; sales: string }>(
              `select (select coalesce(max(version_id), 0) from control.goose_db_version where is_applied) as control,
                      (select coalesce(max(version_id), 0) from sales.goose_db_version where is_applied) as sales`,
            );
            return { control: Number(r.rows[0].control), sales: Number(r.rows[0].sales) };
          })
      : null,
    core: (token) => coreClient({ token }),
    coreToken: async () => {
      if (!db) return null;
      const res = await issueCoreToken(
        {
          auth: getAuth(),
          memberships: dbMembershipLookup(getAppPool()),
          secret: authSecret(),
          secureCookies: webOrigin().startsWith("https://"),
        },
        { headers: h, guest: false },
      );
      return res.status === 200 ? res.token : null;
    },
  };
}
