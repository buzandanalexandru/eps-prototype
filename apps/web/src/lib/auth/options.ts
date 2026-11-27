// Better Auth configuration (U1, design D0.4), built from plain inputs so tests can run it against
// the memory adapter and deployments against Postgres.
import type { BetterAuthOptions } from "better-auth";
import { jwt, magicLink } from "better-auth/plugins";
import { nextCookies } from "better-auth/next-js";
import {
  authSecret,
  demoEmailDomain,
  demoSignInEnabled,
  googleConfigured,
  webOrigin,
  type Env,
} from "../env";
import { demoSignIn } from "./demo-plugin";
import type { MailSender } from "./mailer";

export const CORE_AUDIENCE = "eps-core";
export const CORE_TOKEN_TTL_SECONDS = 5 * 60;
export const MAGIC_LINK_TTL_SECONDS = 15 * 60;
export const KEY_ROTATION_SECONDS = 30 * 24 * 3600;
export const KEY_GRACE_SECONDS = 7 * 24 * 3600;

export type AuthDeps = {
  database: BetterAuthOptions["database"];
  mailer: MailSender;
  env?: Env;
  /** Rate-limit storage: "database" on serverless deployments, "memory" in tests. */
  rateLimitStorage?: "memory" | "database";
};

/** Origins allowed to call the auth endpoints: this deployment plus the project's previews. */
export function trustedOrigins(env: Env): string[] {
  const origins = new Set<string>([webOrigin(env)]);
  for (const o of (env.EPS_TRUSTED_ORIGINS || "").split(",")) if (o.trim()) origins.add(o.trim());
  if (env.VERCEL_URL) origins.add(`https://${env.VERCEL_URL}`);
  if (env.VERCEL_BRANCH_URL) origins.add(`https://${env.VERCEL_BRANCH_URL}`);
  return [...origins];
}

export function buildAuthOptions(deps: AuthDeps) {
  const env = deps.env ?? process.env;
  const origin = webOrigin(env);
  const plugins = [
    magicLink({
      expiresIn: MAGIC_LINK_TTL_SECONDS,
      storeToken: "hashed",
      sendMagicLink: async ({ email, url }) => {
        await deps.mailer.sendMagicLink({ email, url });
      },
    }),
    jwt({
      jwks: {
        keyPairConfig: { alg: "EdDSA", crv: "Ed25519" },
        rotationInterval: KEY_ROTATION_SECONDS,
        gracePeriod: KEY_GRACE_SECONDS,
      },
      jwt: {
        issuer: origin,
        audience: CORE_AUDIENCE,
        expirationTime: CORE_TOKEN_TTL_SECONDS,
      },
      // Core tokens are issued only by /api/core-token with the claims of design D0.4.
      disableSettingJwtHeader: true,
    }),
    ...(demoSignInEnabled(env) ? [demoSignIn({ domain: demoEmailDomain(env) })] : []),
    nextCookies(),
  ];

  return {
    appName: "EPS",
    baseURL: origin,
    basePath: "/api/auth",
    secret: authSecret(env),
    database: deps.database,
    trustedOrigins: trustedOrigins(env),
    emailAndPassword: { enabled: false },
    socialProviders: googleConfigured(env)
      ? {
          google: {
            clientId: env.GOOGLE_CLIENT_ID as string,
            clientSecret: env.GOOGLE_CLIENT_SECRET as string,
          },
        }
      : {},
    account: {
      accountLinking: { enabled: true, trustedProviders: ["google"] },
    },
    session: {
      expiresIn: 7 * 24 * 3600,
      updateAge: 24 * 3600,
    },
    rateLimit: {
      enabled: true,
      window: 60,
      max: 100,
      storage: deps.rateLimitStorage ?? "memory",
    },
    advanced: {
      ipAddress: { ipAddressHeaders: ["x-vercel-forwarded-for", "x-real-ip", "x-forwarded-for"] },
    },
    plugins,
  } satisfies BetterAuthOptions;
}
