// Runtime configuration read from the environment. Server-only.
// Every flag that turns on a demo-only path defaults to off.

export type Env = Record<string, string | undefined>;

export const DEFAULT_DEMO_EMAIL_DOMAIN = "demo.eps-prototype.dev";

export function demoSignInEnabled(env: Env = process.env): boolean {
  return env.EPS_DEMO_SIGNIN === "on";
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
