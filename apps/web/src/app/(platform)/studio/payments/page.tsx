// Studio's payments page (step 4): the organiser's simulated providers A and B (breaker state and
// live knobs: outage, failures, declines, the bank code, latency, lost webhooks) and the EPS
// Wallet's season close. No real money anywhere.
import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { organiserCore, type ProviderState } from "@/lib/checkout/studio-core";
import { requireStudio } from "@/lib/studio/context";
import { Card } from "@/components/ui/field";
import { Button } from "@/components/ui/button";
import { closeSeasonAction, setKnobsAction } from "./actions";

export const metadata = { title: "Payments" };
export const dynamic = "force-dynamic";

type Search = Promise<Record<string, string | undefined>>;

const field = "w-full rounded-md border bg-background px-2 py-1 text-sm";

export default async function StudioPaymentsPage({ searchParams }: { searchParams: Search }) {
  const s = await requireStudio();
  const t = await getTranslations("payments");
  const sp = await searchParams;
  const r = await organiserCore<{ providers: ProviderState[] }>("GET", "/v1/organiser/payments/providers");
  const admin = s.active.role === "owner" || s.active.role === "admin";
  const providers = r.ok ? r.data.providers : [];
  const year = new Date().getUTCFullYear() + 1;
  return (
    <section className="max-w-3xl space-y-6">
      <h1 className="text-2xl font-bold">{t("title")}</h1>
      <p className="text-sm text-muted-foreground">{t("lead")}</p>
      {sp.saved && (
        <p role="status" className="text-sm font-semibold" data-testid="knobs-saved">
          {t("saved", { id: sp.saved.toUpperCase() })}
        </p>
      )}
      {sp.error && (
        <p role="alert" className="text-sm font-semibold text-destructive" data-testid="payments-error">
          {t("error", { code: sp.error })}
        </p>
      )}
      {!r.ok && <p role="alert">{t("error", { code: r.code })}</p>}
      <div className="grid gap-4 sm:grid-cols-2">
        {providers.map((p) => {
          const k = p.knobs ?? {};
          return (
            <Card key={p.provider}>
              <div data-testid={`provider-${p.provider}`} data-breaker={p.state} className="space-y-3">
                <h2 className="font-semibold">{t("provider", { id: p.provider.toUpperCase() })}</h2>
                <p className="text-sm" data-testid="breaker-state">
                  {t(`state_${p.state}`)}
                </p>
                <p className="text-xs text-muted-foreground">{t("calls", { calls: p.calls ?? 0, failures: p.failures ?? 0 })}</p>
                {!p.reachable && <p className="text-sm text-destructive">{t("unreachable")}</p>}
                {admin && p.reachable && (
                  <form action={setKnobsAction} className="space-y-2" data-testid={`knobs-${p.provider}`}>
                    <input type="hidden" name="provider" value={p.provider} />
                    <label className="flex items-center gap-2 text-sm">
                      <input type="checkbox" name="outage" defaultChecked={!!k.outage} data-testid="knob-outage" /> {t("outage")}
                    </label>
                    <label className="grid gap-1 text-sm">
                      {t("outageMode")}
                      <select name="outage_mode" defaultValue={k.outage_mode === "hang" ? "hang" : "503"} className={field}>
                        <option value="503">{t("mode503")}</option>
                        <option value="hang">{t("modeHang")}</option>
                      </select>
                    </label>
                    {(
                      [
                        ["failure_rate", t("failureRate"), k.failure_rate],
                        ["decline_rate", t("declineRate"), k.decline_rate],
                        ["challenge_rate", t("challengeRate"), k.challenge_rate],
                        ["timeout_rate", t("timeoutRate"), k.timeout_rate],
                        ["webhook_drop_rate", t("webhookDropRate"), k.webhook_drop_rate],
                        ["latency_ms", t("latency"), k.latency_ms],
                        ["latency_jitter_ms", t("jitter"), k.latency_jitter_ms],
                        ["webhook_delay_ms", t("webhookDelay"), k.webhook_delay_ms],
                      ] as const
                    ).map(([name, label, v]) => (
                      <label key={name} className="grid gap-1 text-sm">
                        {label}
                        <input name={name} defaultValue={String(v ?? 0)} inputMode="decimal" className={field} data-testid={`knob-${name}`} />
                      </label>
                    ))}
                    <Button type="submit" size="sm" data-testid="knobs-save">
                      {t("save", { id: p.provider.toUpperCase() })}
                    </Button>
                  </form>
                )}
              </div>
            </Card>
          );
        })}
      </div>
      <Card>
        <h2 className="font-semibold">{t("walletTitle")}</h2>
        <p className="text-sm text-muted-foreground">{t("walletLead")}</p>
        {sp.closed && (
          <p role="status" className="text-sm font-semibold" data-testid="season-closed">
            {t("closed", { fans: sp.closed })}
          </p>
        )}
        {admin && (
          <form action={closeSeasonAction} className="flex flex-wrap items-end gap-2" data-testid="season-close">
            <label className="grid gap-1 text-sm">
              {t("expires")}
              <input type="date" name="expires" defaultValue={`${year}-12-31`} className={field} />
            </label>
            <Button type="submit" size="sm" variant="outline">
              {t("closeSeason")}
            </Button>
          </form>
        )}
        <Link href="/studio/stand" className="text-sm text-primary hover:underline">
          {t("standLink")}
        </Link>
      </Card>
    </section>
  );
}
