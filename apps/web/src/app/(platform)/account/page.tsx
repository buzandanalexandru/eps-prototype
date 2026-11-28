// The fan account (F8, Thin): orders with their true states, the tickets held now, instalment
// schedules with Pay now, and "Delete my data".
import { getLocale, getTranslations } from "next-intl/server";
import Link from "next/link";
import { AccountPlans } from "@/components/tickets/account-plans";
import type { OrderView } from "@/components/storefront/shop-api";
import { RetryRefresh } from "@/components/studio/retry-refresh";
import { brandLooks, fanCore } from "@/lib/fan/core";
import { formatMoney, type Currency } from "@/lib/pricing";
import { fmtDate } from "@/lib/trip/format";
import type { Trip } from "@/lib/trip/timeline";
import { deleteMyDataAction } from "./actions";

export const dynamic = "force-dynamic";

export async function generateMetadata() {
  const t = await getTranslations("account");
  return { title: t("title") };
}

export default async function AccountPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  const t = await getTranslations("account");
  const tt = await getTranslations("trip");
  const locale = (await getLocale()) === "en" ? "en" : "ro";
  const sp = await searchParams;
  const [ordersR, tripR] = await Promise.all([fanCore<{ orders: OrderView[] }>("GET", "/v1/shop/orders"), fanCore<Trip>("GET", "/v1/me/trip")]);
  const orders = ordersR.ok ? ordersR.data.orders : [];
  const trip = tripR.ok ? tripR.data : null;
  const looks = trip ? await brandLooks(trip.events.map((e) => e.brandId)).catch(() => ({}) as Awaited<ReturnType<typeof brandLooks>>) : {};
  const shopOf = (eventId: string) => {
    const e = trip?.events.find((x) => x.eventId === eventId);
    const l = e ? looks[e.brandId] : undefined;
    return l ? `/s/${l.slug}/${locale}` : null;
  };
  const errorText =
    sp.error === "member" ? t("deleteMember") : sp.error === "EPS-ERASE-LIVE-ORDER" ? t("deleteLive") : sp.error === "confirm" ? t("deleteConfirm") : sp.error ? t("deleteFailed", { code: sp.error }) : null;
  return (
    <section className="space-y-8">
      <div className="space-y-1">
        <h1 className="text-2xl font-bold">{t("title")}</h1>
        <p className="text-sm text-muted-foreground">{t("lead")}</p>
      </div>
      {(!ordersR.ok || !tripR.ok) && (
        <p role="alert" data-testid="account-unreachable">
          {t("unreachable")}
          <RetryRefresh />
        </p>
      )}

      <div className="space-y-3">
        <h2 className="text-xl font-semibold">{t("orders")}</h2>
        {orders.length === 0 && <p className="text-sm text-muted-foreground">{t("noOrders")}</p>}
        <ul className="space-y-2">
          {orders.map((o) => {
            const name = (o.eventName as { ro?: string; en?: string } | undefined)?.[locale] ?? "";
            const shop = shopOf(o.eventId);
            return (
              <li key={o.id} className="space-y-1 rounded-xl border p-3" data-testid="account-order" data-order-id={o.id} data-state={o.state}>
                <div className="flex flex-wrap justify-between gap-2">
                  <p className="font-semibold break-words">{name || t("order", { id: o.id.slice(0, 8) })}</p>
                  <span className="text-sm font-semibold" data-testid="account-order-state">
                    {t(`state_${o.state}` as "state_paid")}
                  </span>
                </div>
                <p className="text-sm">
                  {t("order", { id: o.id.slice(0, 8).toUpperCase() })} · {t("total", { amount: formatMoney(o.totalMinor, o.currency as Currency, locale) })} · {fmtDate(o.createdAt, locale)}
                </p>
                <div className="flex flex-wrap gap-3 text-sm">
                  {o.state === "issued" && (
                    <Link href="/wallet" className="font-semibold underline" data-testid="account-open-tickets">
                      {t("openTickets")}
                    </Link>
                  )}
                  {shop && (
                    <a href={`${shop}/checkout?order=${o.id}`} className="underline">
                      {t("openOrder")}
                    </a>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
      </div>

      <div className="space-y-3">
        <h2 className="text-xl font-semibold">{t("tickets")}</h2>
        {(!trip || trip.events.every((e) => e.items.length === 0)) && <p className="text-sm text-muted-foreground">{t("noTickets")}</p>}
        <ul className="space-y-2">
          {trip?.events.flatMap((e) =>
            e.items.map((i) => (
              <li key={i.id} data-testid="account-ticket" data-state={i.state}>
                <Link href={`/wallet/tickets/${i.id}`} className="block rounded-xl border p-3 hover:bg-muted">
                  <span className="block font-semibold break-words">
                    {i.productName[locale] || i.productName.en} · {e.name[locale] || e.name.en}
                  </span>
                  <span className="text-sm">
                    {i.state === "suspended" ? t("suspendedTicket") : i.state === "cancelled" ? tt("stateCancelled") : i.named && i.needsName ? tt("stateNeedsName") : i.checkedInAt ? tt("stateCheckedIn") : tt("stateValid")}
                  </span>
                </Link>
              </li>
            )),
          )}
        </ul>
      </div>

      <div className="space-y-3">
        <h2 className="text-xl font-semibold">{t("plans")}</h2>
        <AccountPlans orders={orders} locale={locale} />
      </div>

      <form action={deleteMyDataAction} className="space-y-2 rounded-xl border border-destructive/50 p-3" data-testid="delete-my-data">
        <h2 className="text-xl font-semibold">{t("deleteTitle")}</h2>
        <p className="text-sm">{t("deleteLead")}</p>
        {errorText && (
          <p role="alert" className="text-sm font-semibold text-destructive" data-testid="delete-error">
            {errorText}
          </p>
        )}
        <label className="flex items-start gap-2 text-sm">
          <input type="checkbox" name="confirm" value="yes" required className="mt-1" data-testid="delete-confirm" />
          {t("deleteConfirm")}
        </label>
        <button type="submit" className="rounded-md bg-destructive px-3 py-2 text-sm font-semibold text-white" data-testid="delete-submit">
          {t("deleteButton")}
        </button>
      </form>
    </section>
  );
}
