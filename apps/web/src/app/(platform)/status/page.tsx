import { getTranslations } from "next-intl/server";
import { loadStatus } from "@/lib/status/loader";
import { statusDeps } from "@/lib/status/deps";

export const metadata = { title: "Status" };
export const dynamic = "force-dynamic";

function Row({ label, value, testId }: { label: string; value: React.ReactNode; testId?: string }) {
  return (
    <div className="grid grid-cols-1 gap-1 border-b py-2 sm:grid-cols-3">
      <dt className="text-sm text-muted-foreground">{label}</dt>
      <dd className="break-all font-mono text-sm sm:col-span-2" data-testid={testId}>
        {value}
      </dd>
    </div>
  );
}

export default async function StatusPage() {
  const t = await getTranslations("status");
  const s = await loadStatus(await statusDeps());
  return (
    <section className="space-y-8">
      <div>
        <h1 className="text-2xl font-bold">{t("title")}</h1>
        <p className="text-muted-foreground">{t("lead")}</p>
      </div>

      <div>
        <h2 className="font-semibold">{t("web")}</h2>
        <dl>
          <Row label={t("commit")} value={s.web.commit} testId="web-commit" />
        </dl>
      </div>

      <div>
        <h2 className="font-semibold">{t("database")}</h2>
        <dl>
          <Row
            label={t("database")}
            testId="db-state"
            value={
              s.database.state === "ready"
                ? t("ready")
                : s.database.state === "unconfigured"
                  ? t("unconfigured")
                  : `${t("notReady")}: ${s.database.reason}`
            }
          />
          {s.database.state === "ready" && (
            <>
              <Row label={t("control")} value={s.database.versions.control} testId="schema-control" />
              <Row label={t("sales")} value={s.database.versions.sales} testId="schema-sales" />
            </>
          )}
        </dl>
      </div>

      <div>
        <h2 className="font-semibold">{t("core")}</h2>
        <dl>
          <Row label={t("address")} value={s.core.url ?? "—"} testId="core-url" />
          {s.core.state === "ok" ? (
            <Row label={t("commit")} value={s.core.commit} testId="core-commit" />
          ) : (
            <Row label={t("unreachable")} value={s.core.reason} testId="core-error" />
          )}
        </dl>
      </div>

      <div>
        <h2 className="font-semibold">{t("whoami")}</h2>
        {s.whoami.state === "signed_out" && <p data-testid="whoami-signed-out">{t("signedOut")}</p>}
        {s.whoami.state === "error" && (
          <p className="text-destructive" data-testid="whoami-error">
            {s.whoami.reason}
          </p>
        )}
        {s.whoami.state === "ok" && (
          <dl data-testid="whoami">
            <Row label={t("subject")} value={s.whoami.identity.sub} testId="whoami-sub" />
            <Row label={t("kind")} value={s.whoami.identity.kind} testId="whoami-kind" />
            {s.whoami.identity.org && <Row label={t("organiser")} value={s.whoami.identity.org} />}
            {s.whoami.identity.role && <Row label={t("role")} value={s.whoami.identity.role} />}
            <Row label={t("session")} value={s.whoami.identity.sid} />
            <Row label={t("expires")} value={new Date(s.whoami.identity.exp * 1000).toISOString()} />
          </dl>
        )}
      </div>
    </section>
  );
}
