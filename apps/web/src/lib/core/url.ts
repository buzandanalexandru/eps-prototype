// How a deployment finds its own Go core (design D0.5). The slug rule is implemented twice, here and
// in deploy/scripts/slug.sh for CI; both are tested against deploy/preview-slug-vectors.json.
import { createHash } from "node:crypto";
import type { Env } from "../env";

export const MAX_SLUG_BODY = 40;

/** Bodies that would read as a shared environment; they get a "br-" prefix (the hash is added anyway). */
export const RESERVED_SLUG_BODIES = ["prod", "main", "staging", "eps"];

/**
 * Environment slug for a branch: "prod" for main, else `<body>-<hash>`, a DNS label of at most 47
 * characters. Characters outside ASCII letters and digits become "-" *before* lowercasing, so a
 * non-ASCII letter (for example "İ") never folds into an ASCII one; the 6-character hash of the raw
 * branch name is always added, so no other branch maps to "prod" or to another branch's slug.
 */
export function envSlug(branch: string): string {
  if (branch === "main") return "prod";
  const hash = createHash("sha256").update(branch, "utf8").digest("hex").slice(0, 6);
  let body = branch
    .replace(/[^A-Za-z0-9]+/g, "-")
    .toLowerCase()
    .replace(/^-+|-+$/g, "")
    .slice(0, MAX_SLUG_BODY)
    .replace(/-+$/, "");
  if (body === "") body = "env";
  if (RESERVED_SLUG_BODIES.includes(body) || body.startsWith("kube-")) body = `br-${body}`;
  return `${body}-${hash}`;
}

/** This deployment's environment slug: from its branch, else EPS_ENV, else "dev". */
export function webEnvSlug(env: Env = process.env): string {
  const branch = env.VERCEL_GIT_COMMIT_REF || env.EPS_BRANCH;
  return branch ? envSlug(branch) : env.EPS_ENV || "dev";
}

/**
 * Issuer of this deployment's core tokens: `urn:eps:web:<slug>`. The Go core of the same
 * environment accepts only this issuer (its CORE_TOKEN_ISSUERS, derived from the same slug), so a
 * token minted by one environment never verifies in another.
 */
export function coreTokenIssuer(env: Env = process.env): string {
  return `urn:eps:web:${webEnvSlug(env)}`;
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
