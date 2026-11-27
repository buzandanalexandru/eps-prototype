// How a deployment finds its own Go core (design D0.5). The slug rule is implemented twice, here and
// in deploy/scripts/slug.sh for CI; both are tested against deploy/preview-slug-vectors.json.
import { createHash } from "node:crypto";
import type { Env } from "../env";

export const MAX_SLUG_BODY = 40;

/** Environment slug for a branch: "prod" for main, else a DNS label of at most 47 characters. */
export function envSlug(branch: string): string {
  if (branch === "main") return "prod";
  const hash = createHash("sha256").update(branch, "utf8").digest("hex").slice(0, 6);
  const clean = branch
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  if (clean === "") return `env-${hash}`;
  if (clean.length <= MAX_SLUG_BODY) return clean;
  return `${clean.slice(0, MAX_SLUG_BODY).replace(/-+$/, "")}-${hash}`;
}

/** The core's base URL: CORE_API_URL when set, else derived from the deployment's branch. */
export function coreBaseUrl(env: Env = process.env): string | null {
  if (env.CORE_API_URL) return env.CORE_API_URL.replace(/\/$/, "");
  const branch = env.VERCEL_GIT_COMMIT_REF || env.EPS_BRANCH;
  if (branch && env.CORE_BASE_DOMAIN) return `https://core.${envSlug(branch)}.${env.CORE_BASE_DOMAIN}`;
  if (!env.VERCEL) return "http://localhost:8080";
  return null;
}

/** The gateway's base URL, by the same rule. */
export function gatewayBaseUrl(env: Env = process.env): string | null {
  if (env.GATEWAY_API_URL) return env.GATEWAY_API_URL.replace(/\/$/, "");
  const branch = env.VERCEL_GIT_COMMIT_REF || env.EPS_BRANCH;
  if (branch && env.CORE_BASE_DOMAIN) return `https://gw.${envSlug(branch)}.${env.CORE_BASE_DOMAIN}`;
  if (!env.VERCEL) return "http://localhost:8081";
  return null;
}
