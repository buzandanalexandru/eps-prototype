// Data for the /status page: the full round trip browser → web → Neon and → Go core (design D0.7).
import type { CoreClient } from "../core/client";

export type SchemaVersions = { control: number; sales: number };

export type StatusData = {
  web: { commit: string };
  database:
    | { state: "ready"; versions: SchemaVersions }
    | { state: "not_ready"; versions?: Partial<SchemaVersions>; reason: string }
    | { state: "unconfigured" };
  core:
    | { state: "ok"; url: string; commit: string; service: string; schemas: { control?: number; sales?: number } }
    | { state: "unreachable"; url: string | null; reason: string };
  whoami:
    | { state: "signed_out" }
    | { state: "ok"; identity: { sub: string; kind: string; org?: string; role?: string; sid: string; exp: number } }
    | { state: "error"; reason: string };
};

export type StatusDeps = {
  webCommit: string;
  coreUrl: string | null;
  /** null when the database is not configured. Throws or returns zeros when tables are missing. */
  readSchemaVersions: (() => Promise<SchemaVersions>) | null;
  core: (token?: string) => CoreClient | null;
  /** A core token for the signed-in user, or null when signed out. */
  coreToken: () => Promise<string | null>;
};

function message(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

export async function loadStatus(deps: StatusDeps): Promise<StatusData> {
  const [database, core, whoami] = await Promise.all([loadDatabase(deps), loadCore(deps), loadWhoami(deps)]);
  return { web: { commit: deps.webCommit }, database, core, whoami };
}

async function loadDatabase(deps: StatusDeps): Promise<StatusData["database"]> {
  if (!deps.readSchemaVersions) return { state: "unconfigured" };
  try {
    const v = await deps.readSchemaVersions();
    if (!v.control || !v.sales) {
      return { state: "not_ready", versions: v, reason: "migrations have not run on this database branch yet" };
    }
    return { state: "ready", versions: v };
  } catch (err) {
    const m = message(err);
    // 42P01 undefined_table / 3F000 invalid_schema_name: the branch exists but is not migrated.
    const missing = /does not exist/i.test(m);
    return { state: "not_ready", reason: missing ? "tables are missing: migrations have not run yet" : "database unreachable" };
  }
}

async function loadCore(deps: StatusDeps): Promise<StatusData["core"]> {
  const client = deps.core();
  if (!client || !deps.coreUrl) return { state: "unreachable", url: deps.coreUrl, reason: "no core address configured" };
  try {
    const { data, error, response } = await client.GET("/v1/version", { signal: AbortSignal.timeout(5000) });
    if (!data) return { state: "unreachable", url: deps.coreUrl, reason: `HTTP ${response.status}${error ? "" : ""}` };
    return { state: "ok", url: deps.coreUrl, commit: data.commit, service: data.service, schemas: data.schemas };
  } catch (err) {
    return { state: "unreachable", url: deps.coreUrl, reason: message(err) };
  }
}

async function loadWhoami(deps: StatusDeps): Promise<StatusData["whoami"]> {
  let token: string | null;
  try {
    token = await deps.coreToken();
  } catch (err) {
    return { state: "error", reason: `could not issue a core token: ${message(err)}` };
  }
  if (!token) return { state: "signed_out" };
  const client = deps.core(token);
  if (!client) return { state: "error", reason: "no core address configured" };
  try {
    const { data, response } = await client.GET("/v1/whoami", { signal: AbortSignal.timeout(5000) });
    if (!data) return { state: "error", reason: `core answered HTTP ${response.status}` };
    return {
      state: "ok",
      identity: { sub: data.sub, kind: data.kind, org: data.org, role: data.role, sid: data.sid, exp: data.exp },
    };
  } catch (err) {
    return { state: "error", reason: message(err) };
  }
}
