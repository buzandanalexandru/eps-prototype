// Database-backed unit tests run against a migrated Postgres (CI: the web job's postgres:18 service,
// migrated by cmd/migrate; locally: EPS_TEST_DATABASE_URL). They connect as the migrating owner and
// every query still runs inside withTx (SET LOCAL ROLE eps_web), exactly like the app.
// Without the variable they are skipped locally and fail in CI.
import { randomUUID } from "node:crypto";
import pg from "pg";
import { afterAll, describe } from "vitest";
import { withTx, type Scope } from "@/lib/db/tx";
import { createOrganiser, completeOnboarding } from "@/lib/organisers/domain";
import type { ClientBase } from "pg";

export const TEST_DB = process.env.EPS_TEST_DATABASE_URL;
if (!TEST_DB && process.env.CI) throw new Error("EPS_TEST_DATABASE_URL is required in CI");

export const describeDb = TEST_DB ? describe : describe.skip;

let pool: pg.Pool | undefined;
export function testPool(): pg.Pool {
  if (!pool) pool = new pg.Pool({ connectionString: TEST_DB, max: 4 });
  return pool;
}
afterAll(async () => {
  await pool?.end();
  pool = undefined;
});

export function tx<T>(scope: Scope, fn: (c: pg.PoolClient) => Promise<T>): Promise<T> {
  return withTx(testPool(), scope, fn);
}

/** Runs as the owner (no role switch): test setup only, e.g. creating Better Auth users. */
export async function asOwner<T>(fn: (c: ClientBase) => Promise<T>): Promise<T> {
  const c = await testPool().connect();
  try {
    return await fn(c);
  } finally {
    c.release();
  }
}

export function uniq(prefix = "t"): string {
  return `${prefix}-${randomUUID().slice(0, 8)}`;
}

export async function makeUser(email?: string): Promise<{ id: string; email: string }> {
  const id = `u_${randomUUID().replaceAll("-", "").slice(0, 20)}`;
  const e = email ?? `${id}@test.eps.invalid`;
  await asOwner((c) => c.query(`insert into control."user" (id, name, email, "emailVerified") values ($1, $2, $3, true)`, [id, `User ${id.slice(2, 6)}`, e]));
  return { id, email: e };
}

/** A fully onboarded organiser with one brand, owned by a new user. */
export async function makeOrganiser(opts: { languages?: ("ro" | "en")[]; currency?: "RON" | "EUR" } = {}) {
  const user = await makeUser();
  const id = randomUUID();
  const slug = uniq("org");
  await tx({ organiserId: id, userId: user.id }, (c) => createOrganiser(c, { id, userId: user.id, input: { name: `Org ${slug}`, slug } }));
  const { brandId } = await tx({ organiserId: id, userId: user.id }, (c) =>
    completeOnboarding(c, id, { name: `Brand ${slug}`, slug: uniq("brand"), languages: opts.languages ?? ["ro", "en"], currency: opts.currency ?? "RON", template: "festival" }),
  );
  return { organiserId: id, brandId, user, scope: { organiserId: id, userId: user.id } as Scope };
}
