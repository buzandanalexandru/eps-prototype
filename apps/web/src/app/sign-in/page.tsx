import { getTranslations } from "next-intl/server";
import { demoEmailDomain, demoSignInEnabled, googleConfigured } from "@/lib/env";
import { safeNext } from "@/lib/auth/guard";
import { isDemoAddress } from "@/lib/auth/mailer";
import { getMailbox } from "@/lib/auth/server";
import { SignInForm } from "./sign-in-form";

export const metadata = { title: "Sign in" };

type Search = Promise<{ next?: string; error?: string; sent?: string }>;

export default async function SignInPage({ searchParams }: { searchParams: Search }) {
  const sp = await searchParams;
  const t = await getTranslations("signIn");
  const next = safeNext(sp.next);
  const demo = demoSignInEnabled();
  const domain = demoEmailDomain();
  const sent = sp.sent?.toLowerCase();

  let mailbox: { url: string; createdAt: Date }[] | null = null;
  if (demo && sent && isDemoAddress(sent, domain) && process.env.DATABASE_URL) {
    mailbox = await getMailbox()
      .latest(sent, 3)
      .catch(() => []);
  }

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-bold">{t("title")}</h1>
      <p className="max-w-prose text-muted-foreground">{t("lead")}</p>
      {sp.error && (
        <p role="alert" className="rounded-md border border-destructive/40 p-3 text-sm text-destructive" data-testid="link-error">
          {t("linkInvalid")}
        </p>
      )}
      {sent && <p role="status">{t("sent", { email: sent })}</p>}
      {mailbox && (
        <section className="space-y-2 rounded-lg border p-4" data-testid="demo-mailbox">
          <h2 className="font-semibold">{t("mailboxTitle")}</h2>
          <p className="text-sm text-muted-foreground">{t("mailboxLead", { domain: `@${domain}` })}</p>
          {mailbox.length === 0 ? (
            <p className="text-sm">{t("mailboxEmpty")}</p>
          ) : (
            <ul className="space-y-1">
              {mailbox.map((m) => (
                <li key={m.url}>
                  <a className="break-all text-primary underline" href={m.url}>
                    {t("openLink")}
                  </a>{" "}
                  <span className="text-xs text-muted-foreground">{m.createdAt.toISOString()}</span>
                </li>
              ))}
            </ul>
          )}
        </section>
      )}
      <SignInForm next={next} google={googleConfigured()} demo={demo} initialEmail={sent ?? ""} />
    </section>
  );
}
