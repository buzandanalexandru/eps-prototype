// The live ticket (F3): its rotating code, actions and the wallet pass preview.
import { getLocale, getTranslations } from "next-intl/server";
import Link from "next/link";
import { notFound } from "next/navigation";
import { LiveTicket } from "@/components/tickets/live-ticket";
import { brandLooks, fanCore } from "@/lib/fan/core";
import type { TripEvent } from "@/lib/trip/timeline";

export const dynamic = "force-dynamic";

export async function generateMetadata() {
  const t = await getTranslations("ticket");
  return { title: t("title") };
}

export default async function TicketPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  if (!/^[0-9a-f-]{36}$/.test(id)) notFound();
  const t = await getTranslations("ticket");
  const locale = (await getLocale()) === "en" ? "en" : "ro";
  const r = await fanCore<TripEvent>("GET", `/v1/me/tickets/${id}`);
  if (!r.ok && r.status === 404) notFound();
  if (!r.ok && r.status === 410) {
    return (
      <section className="space-y-3" data-testid="ticket-moved">
        <h1 className="text-2xl font-bold">{t("moved")}</h1>
        <p>{t("movedLead")}</p>
        <Link href="/wallet" className="font-semibold underline">
          {t("back")}
        </Link>
      </section>
    );
  }
  if (!r.ok) {
    return (
      <p role="alert" data-testid="ticket-unreachable">
        {t("error", { code: r.code })}
      </p>
    );
  }
  const ev = r.data;
  const looks = await brandLooks([ev.brandId]).catch(() => ({}) as Awaited<ReturnType<typeof brandLooks>>);
  const look = looks[ev.brandId];
  return (
    <section className="space-y-4">
      <Link href="/wallet" className="text-sm font-semibold underline print:hidden">
        {t("back")}
      </Link>
      <LiveTicket
        event={ev}
        item={ev.items[0]}
        locale={locale}
        mode="account"
        look={{ brand: look?.name ?? "", primary: look?.primary ?? "#1f2937", onPrimary: look?.onPrimary ?? "#ffffff" }}
        shopBase={look ? `/s/${look.slug}/${locale}` : null}
      />
    </section>
  );
}
