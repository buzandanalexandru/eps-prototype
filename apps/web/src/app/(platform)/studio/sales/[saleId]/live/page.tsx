// Studio's live on-sale dashboard (step 6, E8): the browser connects straight to the environment's
// gateway for the sale's server-sent events (not through a Vercel function), with the organiser's
// own short-lived core token. The live controls act through the core.
import Link from "next/link";
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { organiserCore } from "@/lib/checkout/studio-core";
import { gatewayBaseUrl } from "@/lib/core/url";
import { requireStudio } from "@/lib/studio/context";
import { LiveDashboard } from "@/components/studio/live-dashboard";
import type { SaleView } from "../actions";

export const metadata = { title: "Live on-sale" };
export const dynamic = "force-dynamic";

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export default async function LivePage({ params }: { params: Promise<{ saleId: string }> }) {
  const { saleId } = await params;
  if (!UUID.test(saleId)) notFound();
  const s = await requireStudio();
  const t = await getTranslations("live");
  const sale = await organiserCore<SaleView>("GET", `/v1/organiser/sales/${saleId}`);
  if (!sale.ok) notFound();
  const gateway = gatewayBaseUrl();
  const admin = s.active.role === "owner" || s.active.role === "admin";
  const simulatorOf = sale.data.rehearsal?.sourceSaleId ?? saleId;
  return (
    <section className="space-y-4" data-testid="live-page">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="text-xl font-semibold">
          {t("title")}: {sale.data.name}
        </h2>
        <Link href={`/studio/sales/${simulatorOf}/simulator`} className="text-sm underline" data-testid="live-simulator-link">
          {t("simulator")}
        </Link>
      </div>
      <p className="text-sm text-muted-foreground">{t("lead")}</p>
      {sale.data.rehearsal && (
        <p role="note" className="rounded-md border border-dashed p-2 text-sm font-semibold" data-testid="live-rehearsal">
          {t("rehearsal")}
        </p>
      )}
      <LiveDashboard saleId={saleId} gateway={gateway ?? ""} admin={admin} initialRate={sale.data.settings.ratePerMinute} initialPaused={sale.data.settings.paused} initialDegraded={!!sale.data.degraded} />
    </section>
  );
}
