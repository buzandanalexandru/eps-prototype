// A guest's tickets from the signed link in their order page or email (F5, design D10): order-scoped,
// no sign-in, live codes only (a printed page carries no entry code).
import { getLocale, getTranslations } from "next-intl/server";
import Link from "next/link";
import { LiveTicket } from "@/components/tickets/live-ticket";
import { RetryRefresh } from "@/components/studio/retry-refresh";
import { coreBaseUrl } from "@/lib/core/url";
import { brandLooks } from "@/lib/fan/core";
import type { TripEvent } from "@/lib/trip/timeline";

export const dynamic = "force-dynamic";

export async function generateMetadata() {
  const t = await getTranslations("ticket");
  return { title: t("linkTitle"), robots: { index: false, follow: false } };
}

async function load(token: string): Promise<{ status: number; event?: TripEvent }> {
  const base = coreBaseUrl();
  if (!base || !/^[A-Za-z0-9_.-]{20,400}$/.test(token)) return { status: 404 };
  try {
    const res = await fetch(`${base}/v1/ticket-links/${token}`, { cache: "no-store", signal: AbortSignal.timeout(8000) });
    if (!res.ok) return { status: res.status };
    return { status: 200, event: (await res.json()) as TripEvent };
  } catch {
    return { status: 503 };
  }
}

export default async function TicketLinkPage({ params }: { params: Promise<{ token: string }> }) {
  const { token } = await params;
  const t = await getTranslations("ticket");
  const tt = await getTranslations("trip");
  const locale = (await getLocale()) === "en" ? "en" : "ro";
  const r = await load(token);
  if (r.status === 410) {
    return (
      <section className="space-y-3" data-testid="link-claimed">
        <h1 className="text-2xl font-bold">{t("linkTitle")}</h1>
        <p>{t("linkClaimed")}</p>
        <Link href="/sign-in?next=/wallet" className="font-semibold underline">
          {t("signIn")}
        </Link>
      </section>
    );
  }
  if (r.status !== 200 || !r.event) {
    return (
      <section className="space-y-2" data-testid="link-invalid">
        <h1 className="text-2xl font-bold">{r.status === 503 ? t("error", { code: "EPS-UNAVAILABLE" }) : t("linkInvalid")}</h1>
        {r.status !== 503 && <p>{t("linkInvalidLead")}</p>}
        {r.status === 503 && <RetryRefresh />}
      </section>
    );
  }
  const ev = r.event;
  const looks = await brandLooks([ev.brandId]).catch(() => ({}) as Awaited<ReturnType<typeof brandLooks>>);
  const look = looks[ev.brandId];
  return (
    <section className="space-y-4" data-testid="ticket-link">
      <h1 className="text-2xl font-bold">{t("linkTitle")}</h1>
      {ev.pending.length > 0 && (
        <p role="status" data-testid="issuing">
          {tt("issuing")}
          <RetryRefresh everyMs={3000} />
        </p>
      )}
      {ev.items.map((item) => (
        <LiveTicket
          key={item.id}
          event={ev}
          item={item}
          locale={locale}
          mode="link"
          linkToken={token}
          look={{ brand: look?.name ?? "", primary: look?.primary ?? "#1f2937", onPrimary: look?.onPrimary ?? "#ffffff" }}
          shopBase={look ? `/s/${look.slug}/${locale}` : null}
        />
      ))}
      <div className="space-y-1 rounded-xl border p-3 text-sm print:hidden" data-testid="claim-offer">
        <p>{t("claimLead")}</p>
        <Link href="/sign-in?next=/wallet" className="font-semibold underline">
          {t("signIn")}
        </Link>
      </div>
    </section>
  );
}
