// Runtime configuration read from the environment. Server-only.
// Every flag that turns on a demo-only path defaults to off.

export type Env = Record<string, string | undefined>;

/** Demo identities live under a reserved, never-resolvable domain (RFC 2606 ".invalid"). */
export const DEFAULT_DEMO_EMAIL_DOMAIN = "demo.eps.invalid";

export function demoSignInEnabled(env: Env = process.env): boolean {
  return env.EPS_DEMO_SIGNIN === "on";
}

/** Demo sign-ins allowed per client address per minute (default 30). */
export function demoSignInPerMinute(env: Env = process.env): number {
  const n = Number(env.EPS_DEMO_SIGNIN_PER_MINUTE);
  return Number.isInteger(n) && n > 0 ? n : 30;
}

/** Magic-link requests and verifications allowed per client address per minute (default 5). */
export function magicLinkPerMinute(env: Env = process.env): number {
  const n = Number(env.EPS_MAGIC_LINK_PER_MINUTE);
  return Number.isInteger(n) && n > 0 ? n : 5;
}

export function demoEmailDomain(env: Env = process.env): string {
  return (env.EPS_DEMO_EMAIL_DOMAIN || DEFAULT_DEMO_EMAIL_DOMAIN).toLowerCase();
}

export function googleConfigured(env: Env = process.env): boolean {
  return Boolean(env.GOOGLE_CLIENT_ID && env.GOOGLE_CLIENT_SECRET);
}

/** The web app's own origin: explicit, else the Vercel deployment URL, else localhost. */
export function webOrigin(env: Env = process.env): string {
  if (env.BETTER_AUTH_URL) return env.BETTER_AUTH_URL.replace(/\/$/, "");
  if (env.VERCEL_ENV === "production" && env.VERCEL_PROJECT_PRODUCTION_URL)
    return `https://${env.VERCEL_PROJECT_PRODUCTION_URL}`;
  if (env.VERCEL_BRANCH_URL) return `https://${env.VERCEL_BRANCH_URL}`;
  if (env.VERCEL_URL) return `https://${env.VERCEL_URL}`;
  return `http://localhost:${env.PORT || 3000}`;
}

/** The commit this deployment was built from. */
export function webCommit(env: Env = process.env): string {
  return env.VERCEL_GIT_COMMIT_SHA || env.EPS_COMMIT || "dev";
}

export function authSecret(env: Env = process.env): string {
  const s = env.BETTER_AUTH_SECRET;
  if (s && s.length >= 32) return s;
  if (env.NODE_ENV === "production" && env.VERCEL_ENV) {
    throw new Error("BETTER_AUTH_SECRET is missing or shorter than 32 characters");
  }
  // Local development and tests only.
  return "eps-local-development-secret-not-for-deploys-0000";
}

/** A Vercel preview deployment (pull request), as opposed to production or local development. */
export function isVercelPreview(env: Env = process.env): boolean {
  return env.VERCEL_ENV === "preview";
}

/**
 * Pooled URL for application transactions. EPS_DATABASE_URL (set by CI, branch-scoped, as the
 * eps_web_login role on the CI-made schema-only branch) wins. On a preview there is no fallback:
 * the Neon–Vercel integration's DATABASE_URL points at a branch copied from production with its
 * data, so a preview without EPS_DATABASE_URL has no database rather than production's data.
 */
export function appDatabaseUrl(env: Env = process.env): string | undefined {
  if (env.EPS_DATABASE_URL) return env.EPS_DATABASE_URL;
  if (isVercelPreview(env)) return undefined;
  return env.DATABASE_URL || undefined;
}

/**
 * Unpooled URL for Better Auth, whose connections keep a session-level SET ROLE and search_path
 * (a transaction-mode pooler would hand those settings to other clients). Never the pooled URL on
 * Vercel; locally DATABASE_URL is a direct connection.
 */
export function authDatabaseUrl(env: Env = process.env): string | undefined {
  if (env.EPS_DATABASE_URL_UNPOOLED) return env.EPS_DATABASE_URL_UNPOOLED;
  if (isVercelPreview(env)) return undefined;
  if (env.DATABASE_URL_UNPOOLED) return env.DATABASE_URL_UNPOOLED;
  return env.VERCEL ? undefined : env.DATABASE_URL || undefined;
}

/** Connections per pool per function instance (default 3, at most 10). */
export function dbPoolMax(env: Env = process.env): number {
  const n = Number(env.EPS_DB_POOL_MAX);
  return Number.isInteger(n) && n > 0 && n <= 10 ? n : 3;
}
