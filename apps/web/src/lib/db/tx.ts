// The web app's single transaction helper (design D0.2, U2): every application query runs inside
// it, under the eps_web role, with the organiser scope (and optionally the user) set for this
// transaction only. Nothing else in the web app may run SET LOCAL ROLE.
import type pg from "pg";

export type Scope = {
  /** Organiser id the transaction may see and write; "" or undefined means no organiser data. */
  organiserId?: string;
  /** Signed-in user id, opening the read-only window on the user's own memberships. */
  userId?: string;
};

export const WEB_ROLE = "eps_web";

// Scope values are uuids and Better Auth ids: letters, digits, "-" and "_". Anything else is refused
// before it reaches SQL, which lets the setup go out as one simple-protocol statement.
const SCOPE_VALUE = /^[A-Za-z0-9_-]{0,128}$/;

function literal(v: string | undefined): string {
  const s = v ?? "";
  if (!SCOPE_VALUE.test(s)) throw new Error("withTx: scope ids may only hold letters, digits, '-' and '_'");
  return `'${s}'`;
}

/** The statement that opens every application transaction: one round trip. */
export function txSetup(scope: Scope): string {
  return (
    `BEGIN; SET LOCAL ROLE ${WEB_ROLE}; ` +
    `SELECT set_config('app.organiser_id', ${literal(scope.organiserId)}, true), ` +
    `set_config('app.user_id', ${literal(scope.userId)}, true)`
  );
}

export async function withTx<T>(
  pool: Pick<pg.Pool, "connect">,
  scope: Scope,
  fn: (client: pg.PoolClient) => Promise<T>,
): Promise<T> {
  const setup = txSetup(scope); // validates before a connection is taken
  const client = await pool.connect();
  let broken = false;
  try {
    await client.query(setup);
    const result = await fn(client);
    await client.query("COMMIT");
    return result;
  } catch (err) {
    await client.query("ROLLBACK").catch(() => {
      broken = true; // the connection state is unknown: do not hand it to the next caller
    });
    throw err;
  } finally {
    client.release(broken || undefined);
  }
}
