// Better Auth configuration (U1, design D0.4), built from plain inputs so tests can run it against
// the memory adapter and deployments against Postgres.
import type { BetterAuthOptions } from "better-auth";
import { jwt, magicLink } from "better-auth/plugins";
import { nextCookies } from "better-auth/next-js";
import {
  authSecret,
  demoEmailDomain,
  demoSignInEnabled,
  demoSignInPerMinute,
  magicLinkPerMinute,
  googleConfigured,
  webOrigin,
  type Env,
} from "../env";
import { coreTokenIssuer } from "../core/url";
import { DEMO_SIGN_IN_PATH, demoSignIn } from "./demo-plugin";
import { isDemoAddress, type MailSender } from "./mailer";

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

/** How long a function instance reuses the JWKS rows (signing key included) before re-reading them. */
export const JWKS_CACHE_MS = 60_000;
/** Session cookie cache: getSession reads the signed cookie instead of the database for this long. */
export const SESSION_COOKIE_CACHE_SECONDS = 60;

type JwkRow = { id: string; publicKey: string; privateKey: string; createdAt: Date; expiresAt?: Date; alg?: string; crv?: string };
type AdapterCtx = {
  context: {
    adapter: {
      findMany<T>(q: { model: string }): Promise<T[]>;
      create<T>(q: { model: string; data: Record<string, unknown> }): Promise<T>;
    };
  };
};

/**
 * The JWT plugin reads every JWKS row (and decrypts the newest) on each signJWT. This keeps the rows
 * in process for JWKS_CACHE_MS, so a core token costs no database round trip for the key; a key
 * this instance creates (rotation) clears the cache. Another instance's new key is seen within a
 * minute, while the previous key keeps signing validly (it stays published for 7 days).
 */
export function cachedJwksAdapter(ttlMs = JWKS_CACHE_MS) {
  let cache: { at: number; rows: JwkRow[] } | undefined;
  return {
    getJwks: async (ctx: unknown) => {
      const now = Date.now();
      if (cache && now - cache.at >= 0 && now - cache.at < ttlMs) return cache.rows;
      const rows = await (ctx as AdapterCtx).context.adapter.findMany<JwkRow>({ model: "jwks" });
      cache = { at: now, rows };
      return rows;
    },
    createJwk: async (data: Omit<JwkRow, "id">, ctx: unknown) => {
      const row = await (ctx as AdapterCtx).context.adapter.create<JwkRow>({
        model: "jwks",
        data: { ...data, createdAt: new Date() },
      });
      cache = undefined;
      return row;
    },
  };
}

/** Paths through which a user with a demo-domain address may be created. */
const DEMO_USER_PATHS = new Set([DEMO_SIGN_IN_PATH, "/magic-link/verify"]);

export function buildAuthOptions(deps: AuthDeps) {
  const env = deps.env ?? process.env;
  const origin = webOrigin(env);
  const demoDomain = demoEmailDomain(env);
  const plugins = [
    magicLink({
      expiresIn: MAGIC_LINK_TTL_SECONDS,
      storeToken: "hashed",
      rateLimit: { window: 60, max: magicLinkPerMinute(env) },
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
      adapter: cachedJwksAdapter() as never,
      jwt: {
        issuer: coreTokenIssuer(env),
        audience: CORE_AUDIENCE,
        expirationTime: CORE_TOKEN_TTL_SECONDS,
      },
      // Core tokens are issued only by /api/core-token with the claims of design D0.4.
      disableSettingJwtHeader: true,
    }),
    ...(demoSignInEnabled(env) ? [demoSignIn({ domain: demoDomain, perMinute: demoSignInPerMinute(env) })] : []),
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
      // The active organiser (design D1.1), set only by Studio's switcher (never from client input
      // to the auth API); a membership check always follows when it is read.
      additionalFields: { activeOrganiserId: { type: "string", required: false, input: false } },
      expiresIn: 7 * 24 * 3600,
      updateAge: 24 * 3600,
      // Plain getSession calls may read a signed cookie for up to a minute instead of the database.
      // The two that decide access, the Studio session check and core-token issuance, pass
      // disableCookieCache, so a signed-out or revoked session is refused at once there.
      cookieCache: { enabled: true, maxAge: SESSION_COOKIE_CACHE_SECONDS },
    },
    databaseHooks: {
      user: {
        create: {
          // Demo-domain users come only from the demo buttons or the demo mailbox, never from a
          // social provider, so nobody can take over a demo identity with an outside account.
          before: async (user: { email: string }, ctx: { path?: string } | null) => {
            if (isDemoAddress(user.email, demoDomain) && !DEMO_USER_PATHS.has(ctx?.path ?? "")) return false;
          },
        },
      },
      account: {
        create: {
          // Never link a social account to a demo-domain user.
          before: async (
            account: { userId: string; providerId: string },
            ctx: { context: { internalAdapter: { findUserById(id: string): Promise<{ email: string } | null> } } } | null,
          ) => {
            if (account.providerId === "credential") return;
            const user = await ctx?.context.internalAdapter.findUserById(account.userId);
            if (!user || isDemoAddress(user.email, demoDomain)) return false;
          },
        },
      },
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
