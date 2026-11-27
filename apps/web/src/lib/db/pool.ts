// Postgres pools for the web app, one of each per function instance, bounded and reused across
// requests. Server-only.
import pg from "pg";
import { appDatabaseUrl, authDatabaseUrl, dbPoolMax, type Env } from "../env";

let appPool: pg.Pool | undefined;
let authPool: pg.Pool | undefined;

/** Pooled URL for application transactions (always through withTx, which sets the role per transaction). */
export function getAppPool(): pg.Pool {
  if (!appPool) {
    const url = appDatabaseUrl();
    if (!url) throw new Error("No application database URL (EPS_DATABASE_URL, or DATABASE_URL outside previews)");
    appPool = new pg.Pool({ connectionString: url, max: dbPoolMax(), idleTimeoutMillis: 10_000 });
  }
  return appPool;
}

export const AUTH_SESSION_SETUP = "SET ROLE eps_web; SET search_path TO control";

const prepared = new WeakSet<pg.PoolClient>();

/**
 * A pool whose every connection runs as eps_web with search_path control before it is handed out
 * (design D0.2), so Better Auth too runs under the role that cannot bypass row-level security.
 * Fails closed: if the setup fails, the connection is destroyed (never returned to the pool as the
 * login user) and the caller gets the error.
 */
export class RolePool extends pg.Pool {
  // pg's Pool.connect has a promise and a callback form; pool.query uses the callback form.
  connect(): Promise<pg.PoolClient>;
  connect(cb: (err: Error | undefined, client: pg.PoolClient | undefined, done: (release?: unknown) => void) => void): void;
  connect(
    cb?: (err: Error | undefined, client: pg.PoolClient | undefined, done: (release?: unknown) => void) => void,
  ): Promise<pg.PoolClient> | void {
    const p = (async () => {
      const client = await super.connect();
      if (!prepared.has(client)) {
        try {
          await client.query(AUTH_SESSION_SETUP);
        } catch (err) {
          client.release(true); // destroy: the connection must never serve queries as the login user
          throw err;
        }
        prepared.add(client);
      }
      return client;
    })();
    if (!cb) return p;
    p.then(
      (client) => cb(undefined, client, (release) => client.release(release as boolean | Error | undefined)),
      (err: Error) => cb(err, undefined, () => {}),
    );
  }
}

/** Unpooled URL for Better Auth; the role and search_path are session settings on each connection. */
export function getAuthPool(env: Env = process.env): pg.Pool {
  if (!authPool) {
    const url = authDatabaseUrl(env);
    if (!url) {
      throw new Error("No unpooled auth database URL (EPS_DATABASE_URL_UNPOOLED, or DATABASE_URL_UNPOOLED outside previews)");
    }
    authPool = new RolePool({ connectionString: url, max: dbPoolMax(env), idleTimeoutMillis: 10_000 });
  }
  return authPool;
}

export function hasDatabase(): boolean {
  return Boolean(appDatabaseUrl());
}
