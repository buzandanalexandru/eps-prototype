// The trip wallet (F2): everything the signed-in fan holds, across organisers, by event.
import { getLocale, getTranslations } from "next-intl/server";
import Link from "next/link";
import { TripWallet } from "@/components/tickets/trip-wallet";
import { brandLooks, fanCore } from "@/lib/fan/core";
import type { Trip } from "@/lib/trip/timeline";

export const dynamic = "force-dynamic";

export async function generateMetadata() {
  const t = await getTranslations("trip");
  return { title: t("title") };
}

export default async function WalletPage() {
  const t = await getTranslations("trip");
  const locale = (await getLocale()) === "en" ? "en" : "ro";
  const r = await fanCore<Trip>("GET", "/v1/me/trip");
  const trip = r.ok ? r.data : null;
  const looks = trip ? await brandLooks(trip.events.map((e) => e.brandId)).catch(() => ({})) : {};
  return (
    <section className="space-y-4">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h1 className="text-2xl font-bold">{t("title")}</h1>
        <Link href="/account" className="text-sm font-semibold underline">
          {t("accountLink")}
        </Link>
      </div>
      <p className="text-sm text-muted-foreground">{t("lead")}</p>
      <TripWallet initial={trip} looks={looks} locale={locale} unreachable={!r.ok} />
    </section>
  );
}
