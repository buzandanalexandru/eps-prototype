// The web app's single transaction helper (design D0.2, U2): every application query runs inside
// it, under the eps_web role, with the organiser scope (and optionally the user) set for this
// transaction only. Nothing else in the web app may run SET ROLE.
import type pg from "pg";

export type Scope = {
  /** Organiser id the transaction may see and write; "" or undefined means no organiser data. */
  organiserId?: string;
  /** Signed-in user id, opening the read-only window on the user's own memberships. */
  userId?: string;
};

export const WEB_ROLE = "eps_web";

export async function withTx<T>(
  pool: Pick<pg.Pool, "connect">,
  scope: Scope,
  fn: (client: pg.PoolClient) => Promise<T>,
): Promise<T> {
  const client = await pool.connect();
  try {
    await client.query("BEGIN");
    await client.query(`SET LOCAL ROLE ${WEB_ROLE}`);
    await client.query(
      "select set_config('app.organiser_id', $1, true), set_config('app.user_id', $2, true)",
      [scope.organiserId ?? "", scope.userId ?? ""],
    );
    const result = await fn(client);
    await client.query("COMMIT");
    return result;
  } catch (err) {
    await client.query("ROLLBACK").catch(() => {});
    throw err;
  } finally {
    client.release();
  }
}
