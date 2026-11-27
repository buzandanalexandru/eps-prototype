import type pg from "pg";
import { withTx } from "../db/tx";
import type { MailboxEntry, MailboxStore } from "./mailer";

/** Demo mailbox rows in control.demo_mailbox; only links younger than the link lifetime show. */
export class PgMailboxStore implements MailboxStore {
  constructor(
    private pool: Pick<pg.Pool, "connect">,
    private ttlSeconds: number,
  ) {}

  async put(e: MailboxEntry) {
    await withTx(this.pool, {}, async (c) => {
      await c.query("insert into control.demo_mailbox (email, url, created_at) values ($1, $2, $3)", [
        e.email,
        e.url,
        e.createdAt,
      ]);
      await c.query("delete from control.demo_mailbox where created_at < now() - make_interval(secs => $1)", [
        this.ttlSeconds,
      ]);
    });
  }

  async latest(email: string, limit = 5) {
    return withTx(this.pool, {}, async (c) => {
      const r = await c.query<{ email: string; url: string; created_at: Date }>(
        `select email, url, created_at from control.demo_mailbox
         where email = $1 and created_at > now() - make_interval(secs => $2)
         order by created_at desc limit $3`,
        [email.toLowerCase(), this.ttlSeconds, limit],
      );
      return r.rows.map((x) => ({ email: x.email, url: x.url, createdAt: x.created_at }));
    });
  }
}
