import { describe, expect, it } from "vitest";
import createClient from "openapi-fetch";
import type { paths } from "@/lib/core/schema";
import { loadStatus, type StatusDeps } from "@/lib/status/loader";

const CORE = "https://core.test.example";

/** A fake Go core speaking the contract's shapes. */
function fakeCore(opts: { down?: boolean } = {}) {
  return (async (input: Request | string | URL, init?: RequestInit) => {
    const req = input instanceof Request ? input : new Request(input, init);
    if (opts.down) throw new TypeError("fetch failed");
    const url = new URL(req.url);
    const json = (status: number, body: unknown, type = "application/json") =>
      new Response(JSON.stringify(body), { status, headers: { "content-type": type } });
    if (url.pathname === "/v1/version") return json(200, { service: "core", commit: "abc123", schemas: { control: 3, sales: 1 } });
    if (url.pathname === "/v1/whoami") {
      if (req.headers.get("authorization") !== "Bearer good-token")
        return json(401, { type: "x", title: "Unauthorized", status: 401, code: "unauthenticated", requestId: "r" }, "application/problem+json");
      return json(200, { sub: "user-1", kind: "fan", sid: "s1", exp: 1_900_000_000 });
    }
    return json(404, {});
  }) as typeof fetch;
}

function deps(over: Partial<StatusDeps> = {}, core = fakeCore()): StatusDeps {
  return {
    webCommit: "web-sha",
    coreUrl: CORE,
    readSchemaVersions: async () => ({ control: 3, sales: 1 }),
    core: (token) =>
      createClient<paths>({ baseUrl: CORE, fetch: core, headers: token ? { Authorization: `Bearer ${token}` } : {} }),
    coreToken: async () => "good-token",
    ...over,
  };
}

describe("status loader", () => {
  it("shows the full round trip for a signed-in user", async () => {
    const s = await loadStatus(deps());
    expect(s.web.commit).toBe("web-sha");
    expect(s.database).toEqual({ state: "ready", versions: { control: 3, sales: 1 } });
    expect(s.core).toMatchObject({ state: "ok", commit: "abc123", url: CORE });
    expect(s.whoami).toMatchObject({ state: "ok", identity: { sub: "user-1", kind: "fan" } });
  });

  it("is 'not ready' when the tables are missing", async () => {
    const s = await loadStatus(
      deps({
        readSchemaVersions: async () => {
          throw new Error('relation "control.goose_db_version" does not exist');
        },
      }),
    );
    expect(s.database.state).toBe("not_ready");
    const zero = await loadStatus(deps({ readSchemaVersions: async () => ({ control: 3, sales: 0 }) }));
    expect(zero.database.state).toBe("not_ready");
  });

  it("reports a signed-out visitor, an unreachable core and a rejected token", async () => {
    expect((await loadStatus(deps({ coreToken: async () => null }))).whoami.state).toBe("signed_out");
    const down = await loadStatus(deps({}, fakeCore({ down: true })));
    expect(down.core.state).toBe("unreachable");
    expect(down.whoami.state).toBe("error");
    const bad = await loadStatus(deps({ coreToken: async () => "forged" }));
    expect(bad.whoami).toEqual({ state: "error", reason: "core answered HTTP 401" });
    const noDb = await loadStatus(deps({ readSchemaVersions: null }));
    expect(noDb.database.state).toBe("unconfigured");
  });
});
