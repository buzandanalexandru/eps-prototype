// "Continue as demo organiser / demo fan" (design D0.4). Only registered when EPS_DEMO_SIGNIN=on,
// so with the flag off the endpoint does not exist and the auth handler answers 404.
import type { BetterAuthPlugin } from "better-auth";
import { createAuthEndpoint } from "better-auth/api";
import { setSessionCookie } from "better-auth/cookies";
import * as z from "zod";

export const DEMO_SIGN_IN_PATH = "/demo/sign-in";

export function demoOrganiserEmail(domain: string) {
  return `demo-organiser@${domain}`;
}

function randomSuffix() {
  const bytes = new Uint8Array(6);
  crypto.getRandomValues(bytes);
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

export function demoSignIn(opts: { domain: string }): BetterAuthPlugin {
  return {
    id: "eps-demo-sign-in",
    endpoints: {
      demoSignIn: createAuthEndpoint(
        DEMO_SIGN_IN_PATH,
        {
          method: "POST",
          body: z.object({ as: z.enum(["organiser", "fan"]) }),
        },
        async (ctx) => {
          const adapter = ctx.context.internalAdapter;
          let user;
          if (ctx.body.as === "organiser") {
            // One shared demo organiser user; step 1's seed makes it owner of the sample organiser.
            const email = demoOrganiserEmail(opts.domain);
            user = (await adapter.findUserByEmail(email))?.user;
            if (!user) {
              user = await adapter.createUser({ email, name: "Demo organiser", emailVerified: true }, { method: "demo" });
            }
          } else {
            // A fresh fan per click, so two visitors never share a cart.
            const id = randomSuffix();
            user = await adapter.createUser({
              email: `demo-fan-${id}@${opts.domain}`,
              name: `Demo fan ${id.slice(0, 4)}`,
              emailVerified: true,
            }, { method: "demo" });
          }
          const session = await adapter.createSession(user.id);
          await setSessionCookie(ctx, { session, user });
          return ctx.json({ userId: user.id, email: user.email, as: ctx.body.as });
        },
      ),
    },
    rateLimit: [
      {
        pathMatcher: (path: string) => path === DEMO_SIGN_IN_PATH,
        window: 60,
        max: 10,
      },
    ],
  };
}
