// Magic-link mail goes through a sender interface (design D0.4). There is no mail provider in the
// decided stack yet, so the only sender is the demo mailbox: it keeps links for addresses under the
// demo domain and shows them on screen; it never sends anything and never logs the link.

export type MagicLinkMessage = { email: string; url: string };

export interface MailSender {
  sendMagicLink(msg: MagicLinkMessage): Promise<void>;
}

export class MailNotConfiguredError extends Error {
  constructor() {
    super("magic links to real addresses are disabled until a mail provider is configured");
    this.name = "MailNotConfiguredError";
  }
}

export type MailboxEntry = { email: string; url: string; createdAt: Date };

/** Storage for demo mailbox entries; Postgres in deployments, memory in tests. */
export interface MailboxStore {
  put(entry: MailboxEntry): Promise<void>;
  latest(email: string, limit?: number): Promise<MailboxEntry[]>;
}

export class MemoryMailboxStore implements MailboxStore {
  entries: MailboxEntry[] = [];
  async put(entry: MailboxEntry) {
    this.entries.push(entry);
  }
  async latest(email: string, limit = 5) {
    return this.entries
      .filter((e) => e.email === email.toLowerCase())
      .sort((a, b) => b.createdAt.getTime() - a.createdAt.getTime())
      .slice(0, limit);
  }
}

export function isDemoAddress(email: string, domain: string): boolean {
  const at = email.lastIndexOf("@");
  return at > 0 && email.slice(at + 1).toLowerCase() === domain.toLowerCase();
}

export class DemoMailbox implements MailSender {
  constructor(
    private store: MailboxStore,
    private domain: string,
  ) {}

  async sendMagicLink({ email, url }: MagicLinkMessage) {
    if (!isDemoAddress(email, this.domain)) throw new MailNotConfiguredError();
    await this.store.put({ email: email.toLowerCase(), url, createdAt: new Date() });
  }
}

/** Used when neither a provider nor the demo mailbox is available: every request is refused. */
export class NoMailer implements MailSender {
  async sendMagicLink(): Promise<void> {
    throw new MailNotConfiguredError();
  }
}
