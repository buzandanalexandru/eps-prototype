// The deployed Better Auth instance (Postgres, demo mailbox). Server-only.
import "server-only";
import { betterAuth } from "better-auth";
import { demoEmailDomain, demoSignInEnabled } from "../env";
import { getAppPool, getAuthPool } from "../db/pool";
import { DemoMailbox, NoMailer, type MailSender } from "./mailer";
import { PgMailboxStore } from "./mailbox-store";
import { buildAuthOptions, MAGIC_LINK_TTL_SECONDS } from "./options";

function makeMailer(): MailSender {
  // No mail provider is in the decided stack yet (design, Open Questions): on previews and the
  // demo the demo mailbox serves demo-domain addresses; elsewhere magic links are refused.
  if (demoSignInEnabled()) {
    return new DemoMailbox(new PgMailboxStore(getAppPool(), MAGIC_LINK_TTL_SECONDS), demoEmailDomain());
  }
  return new NoMailer();
}

function create() {
  return betterAuth(
    buildAuthOptions({ database: getAuthPool(), mailer: makeMailer(), rateLimitStorage: "database" }),
  );
}

let instance: ReturnType<typeof create> | undefined;

export function getAuth() {
  if (!instance) instance = create();
  return instance;
}

export function getMailbox() {
  return new PgMailboxStore(getAppPool(), MAGIC_LINK_TTL_SECONDS);
}
