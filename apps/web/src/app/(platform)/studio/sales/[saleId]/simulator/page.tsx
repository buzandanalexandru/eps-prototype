// Studio's on-sale rehearsal (step 6, E7): the sale moment's rehearsal (create, reset, its waiting
// room), the run settings with their defaults and the environment's limits, Start and Stop, the
// latest run's state and summary, and the honest load panel: measured numbers next to arithmetic.
import Link from "next/link";
import { notFound, redirect } from "next/navigation";
import { getLocale, getTranslations } from "next-intl/server";
import { loadEventGraph } from "@/lib/catalogue/domain";
import { organiserCore } from "@/lib/checkout/studio-core";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { Card } from "@/components/ui/field";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/studio/copy-button";
import { SimulatorPanel } from "@/components/studio/simulator-panel";
import { createRehearsalAction, resetRehearsalAction, type Run, type SaleView } from "../actions";

export const metadata = { title: "On-sale rehearsal" };
export const dynamic = "force-dynamic";

type Limits = { maxFans: number; maxDurationSeconds: number };
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export default async function SimulatorPage({ params, searchParams }: { params: Promise<{ saleId: string }>; searchParams: Promise<Record<string, string | undefined>> }) {
  const { saleId } = await params;
  const sp = await searchParams;
  if (!UUID.test(saleId)) notFound();
  const s = await requireStudio();
  const t = await getTranslations("simulator");
  const locale = await getLocale();
  const sale = await organiserCore<SaleView>("GET", `/v1/organiser/sales/${saleId}`);
  if (!sale.ok) notFound();
  if (sale.data.rehearsal) redirect(`/studio/sales/${sale.data.rehearsal.sourceSaleId}/simulator`);
  const g = await studioTx(s, (c) => loadEventGraph(c, sale.data.eventId));
  if (!g) notFound();
  const rh = await organiserCore<{ rehearsals: SaleView[]; limits: Limits }>("GET", `/v1/organiser/sales/${saleId}/rehearsals`);
  const rehearsal = rh.ok ? rh.data.rehearsals[0] : undefined;
  const limits: Limits = rh.ok ? rh.data.limits : { maxFans: 2000, maxDurationSeconds: 600 };
  const runs = rehearsal ? await organiserCore<{ runs: Run[] }>("GET", `/v1/organiser/sales/${rehearsal.id}/simulations`) : null;
  const latest = runs?.ok ? runs.data.runs[0] : undefined;
  const admin = s.active.role === "owner" || s.active.role === "admin";
  const shopLocale = g.brand.languages.includes(locale as "ro" | "en") ? locale : (g.brand.languages[0] ?? "ro");
  const product = g.products.find((p) => p.visible && p.type === "pass") ?? g.products.find((p) => p.visible);
  const base = `/s/${g.brand.slug}/${shopLocale}`;
  const room =
    rehearsal &&
    `${base}/queue?sale=${rehearsal.id}${product ? `&next=${encodeURIComponent(`${base}/checkout?event=${rehearsal.eventId}&v=1&items=${product.id}:1`)}` : ""}`;
  const fmt = (n: number) => new Intl.NumberFormat(locale === "ro" ? "ro-RO" : "en-GB").format(n);
  return (
    <section className="space-y-6" data-testid="simulator-page">
      <Link href={`/studio/events/${g.event.id}/sales#sale-${saleId}`} className="text-sm underline">
        {t("back")}
      </Link>
      <h2 className="text-xl font-semibold">
        {t("title")}: {sale.data.name}
      </h2>
      <p className="text-sm text-muted-foreground">{t("lead")}</p>
      <p role="note" className="rounded-md border border-dashed p-2 text-sm font-semibold" data-testid="simulator-simulated-note">
        {t("simulatedNote")}
      </p>
      {s.active.is_sample && (
        <p role="note" className="rounded-md border border-dashed p-2 text-sm" data-testid="simulator-sample-note">
          {t("sampleNote")}
        </p>
      )}
      {sp.error && (
        <p role="alert" className="text-sm font-semibold text-destructive" data-testid="simulator-error">
          {t("error", { message: sp.error })}
        </p>
      )}
      <Card>
        {rehearsal ? (
          <div className="space-y-3" data-testid="rehearsal" data-rehearsal-id={rehearsal.id}>
            <h3 className="font-semibold" data-testid="rehearsal-name">
              {rehearsal.name}
            </h3>
            <p className="text-sm">{t("source", { name: sale.data.name, version: rehearsal.rehearsal?.snapshotVersion ?? 1 })}</p>
            {room && (
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <span>{t("waitingRoom")}:</span>
                <a href={room} className="break-all underline" data-testid="rehearsal-room">
                  {`${base}/queue?sale=${rehearsal.id}`}
                </a>
                <CopyButton value={room} />
              </div>
            )}
            <div className="flex flex-wrap gap-2">
              <Link href={`/studio/sales/${rehearsal.id}/live`} className="rounded-md border px-3 py-2 text-sm font-semibold" data-testid="rehearsal-live">
                {t("liveDashboard")}
              </Link>
              {admin && (
                <form action={resetRehearsalAction.bind(null, saleId, rehearsal.id)}>
                  <Button type="submit" variant="outline" data-testid="rehearsal-reset">
                    {t("reset")}
                  </Button>
                </form>
              )}
            </div>
            <p className="text-xs text-muted-foreground">{t("resetHint")}</p>
          </div>
        ) : (
          <div className="space-y-3">
            <p className="text-sm" data-testid="rehearsal-none">
              {t("noRehearsal")}
            </p>
            {admin && (
              <form action={createRehearsalAction.bind(null, saleId)}>
                <Button type="submit" data-testid="rehearsal-create">
                  {t("create")}
                </Button>
              </form>
            )}
          </div>
        )}
      </Card>
      {rehearsal && <SimulatorPanel sourceSaleId={saleId} rehearsalId={rehearsal.id} limits={limits} initial={latest ?? null} admin={admin} />}
      <Card>
        <section className="space-y-2 text-sm" data-testid="simulator-load">
          <h3 className="font-semibold">{t("loadTitle")}</h3>
          <p data-testid="simulator-load-measured">{t("loadMeasuredStep5")}</p>
          <h4 className="font-semibold">{t("loadMathTitle")}</h4>
          <p data-testid="simulator-load-math">{t("loadMath")}</p>
          <p className="text-muted-foreground" data-testid="simulator-load-cap">
            {t("loadCap", { max: fmt(limits.maxFans) })}
          </p>
        </section>
      </Card>
    </section>
  );
}
