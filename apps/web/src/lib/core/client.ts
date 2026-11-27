// Typed client for the Go core, generated from api/openapi.yaml (S3).
import createClient from "openapi-fetch";
import type { paths } from "./schema";
import { coreBaseUrl } from "./url";

export type CoreClient = ReturnType<typeof createClient<paths>>;

export function coreClient(opts: { token?: string; baseUrl?: string; fetch?: typeof fetch } = {}): CoreClient | null {
  const baseUrl = opts.baseUrl ?? coreBaseUrl();
  if (!baseUrl) return null;
  return createClient<paths>({
    baseUrl,
    fetch: opts.fetch,
    headers: opts.token ? { Authorization: `Bearer ${opts.token}` } : undefined,
  });
}
