import { describe, expect, it, vi } from "vitest";
import pg from "pg";
import { txSetup, withTx } from "@/lib/db/tx";
import { AUTH_SESSION_SETUP, RolePool } from "@/lib/db/pool";
import { appDatabaseUrl, authDatabaseUrl } from "@/lib/env";

type Call = string;

function fakeClient(fail: (sql: string) => boolean = () => false) {
  const calls: Call[] = [];
  const release = vi.fn();
  const client = {
    query: vi.fn(async (sql: string) => {
      calls.push(sql);
      if (fail(sql)) throw new Error(`fail: ${sql}`);
      return { rows: [], rowCount: 0 };
    }),
    release,
  };
  return { client, calls, release };
}

describe("withTx (web transaction helper)", () => {
  it("opens with one statement: BEGIN, SET LOCAL ROLE eps_web, organiser and user scope; then COMMIT", async () => {
    const { client, calls, release } = fakeClient();
    const out = await withTx({ connect: async () => client as never }, { organiserId: "00000000-0000-4000-8000-00000000000a", userId: "u_1" }, async (c) => {
      await c.query("select 1");
      return 42;
    });
    expect(out).toBe(42);
    expect(calls[0]).toBe(
      "BEGIN; SET LOCAL ROLE eps_web; SELECT set_config('app.organiser_id', '00000000-0000-4000-8000-00000000000a', true), set_config('app.user_id', 'u_1', true)",
    );
    expect(calls.slice(1)).toEqual(["select 1", "COMMIT"]);
    expect(release).toHaveBeenCalledWith(undefined);
  });

  it("no scope sets empty values (organiser tables then show nothing)", () => {
    expect(txSetup({})).toContain("set_config('app.organiser_id', '', true)");
  });

  it("rolls back and releases on error; a failed rollback destroys the connection", async () => {
    const a = fakeClient();
    await expect(withTx({ connect: async () => a.client as never }, {}, async () => { throw new Error("x"); })).rejects.toThrow("x");
    expect(a.calls.at(-1)).toBe("ROLLBACK");
    expect(a.release).toHaveBeenCalledWith(undefined);

    const b = fakeClient((sql) => sql === "ROLLBACK");
    await expect(withTx({ connect: async () => b.client as never }, {}, async () => { throw new Error("y"); })).rejects.toThrow("y");
    expect(b.release).toHaveBeenCalledWith(true);
  });

  it("a failed role switch never runs the caller's queries", async () => {
    const { client, calls } = fakeClient((sql) => sql.startsWith("BEGIN"));
    const fn = vi.fn();
    await expect(withTx({ connect: async () => client as never }, {}, fn)).rejects.toThrow();
    expect(fn).not.toHaveBeenCalled();
    expect(calls.at(-1)).toBe("ROLLBACK");
  });

  it("refuses scope values that are not ids, before taking a connection", async () => {
    const connect = vi.fn();
    for (const bad of ["x'; reset role; --", "a b", "é"]) {
      await expect(withTx({ connect } as never, { organiserId: bad }, async () => 1)).rejects.toThrow(/scope ids/);
    }
    expect(connect).not.toHaveBeenCalled();
  });
});

describe("auth pool (Better Auth connections)", () => {
  it("every connection switches to eps_web and the control schema once, before use", async () => {
    const { client, calls } = fakeClient();
    const spy = vi.spyOn(pg.Pool.prototype, "connect").mockResolvedValue(client as never);
    try {
      const pool = new RolePool({ max: 1 });
      expect(await pool.connect()).toBe(client);
      await pool.connect();
      expect(calls).toEqual([AUTH_SESSION_SETUP]);
      expect(AUTH_SESSION_SETUP).toBe("SET ROLE eps_web; SET search_path TO control");
    } finally {
      spy.mockRestore();
    }
  });

  it("fails closed: if SET ROLE fails the connection is destroyed and the caller gets the error", async () => {
    const { client, release } = fakeClient(() => true);
    const spy = vi.spyOn(pg.Pool.prototype, "connect").mockResolvedValue(client as never);
    try {
      const pool = new RolePool({ max: 1 });
      await expect(pool.connect()).rejects.toThrow(/fail/);
      expect(release).toHaveBeenCalledWith(true);
      // The callback form (used by pool.query) fails the same way.
      const err = await new Promise((resolve) => pool.connect((e) => resolve(e)));
      expect(err).toBeInstanceOf(Error);
    } finally {
      spy.mockRestore();
    }
  });
});

describe("database URLs", () => {
  it("previews never fall back to the integration's DATABASE_URL (a copy of production)", () => {
    const integration = { VERCEL: "1", VERCEL_ENV: "preview", DATABASE_URL: "postgres://copy", DATABASE_URL_UNPOOLED: "postgres://copy-direct" };
    expect(appDatabaseUrl(integration)).toBeUndefined();
    expect(authDatabaseUrl(integration)).toBeUndefined();
    const ci = { ...integration, EPS_DATABASE_URL: "postgres://ci-pooled", EPS_DATABASE_URL_UNPOOLED: "postgres://ci-direct" };
    expect(appDatabaseUrl(ci)).toBe("postgres://ci-pooled");
    expect(authDatabaseUrl(ci)).toBe("postgres://ci-direct");
  });

  it("the auth pool needs an unpooled URL on Vercel; locally DATABASE_URL is direct", () => {
    expect(authDatabaseUrl({ VERCEL: "1", VERCEL_ENV: "production", DATABASE_URL: "postgres://pooled" })).toBeUndefined();
    expect(authDatabaseUrl({ VERCEL: "1", VERCEL_ENV: "production", DATABASE_URL: "p", DATABASE_URL_UNPOOLED: "postgres://direct" })).toBe("postgres://direct");
    expect(authDatabaseUrl({ DATABASE_URL: "postgres://local" })).toBe("postgres://local");
    expect(appDatabaseUrl({ VERCEL_ENV: "production", DATABASE_URL: "postgres://prod" })).toBe("postgres://prod");
  });
});
