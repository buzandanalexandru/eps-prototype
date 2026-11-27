// Postgres pools for the web app. Server-only.
import pg from "pg";

let appPool: pg.Pool | undefined;
let authPool: pg.Pool | undefined;

/** Pooled URL for application transactions (always through withTx). */
export function getAppPool(): pg.Pool {
  if (!appPool) {
    const url = process.env.DATABASE_URL;
    if (!url) throw new Error("DATABASE_URL is not set");
    appPool = new pg.Pool({ connectionString: url, max: 5, idleTimeoutMillis: 10_000 });
  }
  return appPool;
}

/**
 * Unpooled URL for Better Auth (design D0.2). Every connection switches to the web role and the
 * control schema, so Better Auth too runs under a role that cannot bypass row-level security.
 */
export function getAuthPool(): pg.Pool {
  if (!authPool) {
    const url = process.env.DATABASE_URL_UNPOOLED || process.env.DATABASE_URL;
    if (!url) throw new Error("DATABASE_URL is not set");
    authPool = new pg.Pool({ connectionString: url, max: 5, idleTimeoutMillis: 10_000 });
    authPool.on("connect", (client) => {
      client.query("SET ROLE eps_web; SET search_path TO control").catch(() => {
        // The query error surfaces on the first real query; nothing secret is logged here.
      });
    });
  }
  return authPool;
}

export function hasDatabase(): boolean {
  return Boolean(process.env.DATABASE_URL);
}
