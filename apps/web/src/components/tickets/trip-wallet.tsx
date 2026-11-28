"use client";
// The trip wallet (F2, design D9): one timeline of everything the fan holds, by event, each event in
// its brand's colours, each item with its one next action and the header with the most urgent one.
// Paid orders whose tickets are being issued show "Issuing your tickets" and the page asks again
// every 3 seconds until they appear.
import { useEffect, useState } from "react";
import Link from "next/link";
import { useTranslations } from "next-intl";
import type { BrandLook } from "@/lib/fan/core";
import { fmtDate, fmtDay } from "@/lib/trip/format";
import { headerAction, nextAction, timeline, type NextAction, type TicketItem, type Trip, type TripEvent } from "@/lib/trip/timeline";
import { call } from "./api";

const FALLBACK: BrandLook = { brandId: "", slug: "", name: "", primary: "#1f2937", onPrimary: "#ffffff", surface: "#ffffff", onSurface: "#111827", logo: null };

export function actionLabel(t: ReturnType<typeof useTranslations>, a: NextAction, locale: string, tz: string): string {
  const date = a.deadline ? fmtDate(a.deadline, locale, tz) : "";
  switch (a.kind) {
    case "pay_overdue":
      return t("actionPay");
    case "name":
      return date ? t("actionNameBy", { date }) : t("actionName");
    case "pick_day":
      return date ? t("actionPickBy", { date }) : t("actionPick");
    case "check_in":
      return date ? t("actionCheckInBy", { date }) : t("actionCheckIn");
    default:
      return t("actionShow");
  }
}

function stateLabel(t: ReturnType<typeof useTranslations>, i: TicketItem): string {
  if (i.state === "suspended") return t("stateSuspended");
  if (i.state === "cancelled") return t("stateCancelled");
  if (i.named && i.needsName) return t("stateNeedsName");
  if (i.checkedInAt) return t("stateCheckedIn");
  return t("stateValid");
}

export function TripWallet({ initial, looks, locale, unreachable }: { initial: Trip | null; looks: Record<string, BrandLook>; locale: "ro" | "en"; unreachable: boolean }) {
  const t = useTranslations("trip");
  const [trip, setTrip] = useState<Trip | null>(initial);
  const [down, setDown] = useState(unreachable);
  const [now, setNow] = useState(() => Date.now());
  const pending = !trip || trip.events.some((e) => e.pending.length > 0);

  useEffect(() => {
    if (!pending && !down) return;
    const id = window.setInterval(async () => {
      const r = await call<Trip>("GET", "/api/fan/me/trip");
      if (r.status === 200 && Array.isArray(r.body.events)) {
        setTrip(r.body);
        setDown(false);
      } else setDown(true);
      setNow(Date.now());
    }, down ? 5000 : 3000);
    return () => window.clearInterval(id);
  }, [pending, down]);

  if (!trip) {
    return (
      <p role="alert" data-testid="trip-unreachable">
        {t("unreachable")}
      </p>
    );
  }
  const groups = timeline(trip);
  const head = headerAction(trip, now);
  const tzOf = (eventId: string) => trip.events.find((e) => e.eventId === eventId)?.timezone ?? "Europe/Bucharest";
  return (
    <div className="space-y-6" data-testid="trip-wallet">
      {down && (
        <p role="alert" className="text-sm" data-testid="trip-unreachable">
          {t("unreachable")}
        </p>
      )}
      {head && (
        <Link href={`/wallet/tickets/${head.ticketId}`} className="block rounded-xl border-2 border-foreground p-3" data-testid="trip-header-action" data-kind={head.kind}>
          <span className="text-xs font-semibold uppercase">{t("next")}</span>
          <span className="block font-semibold">{actionLabel(t, head, locale, tzOf(head.eventId))}</span>
        </Link>
      )}
      {groups.length === 0 && (
        <div className="space-y-2 rounded-xl border p-6 text-center" data-testid="trip-empty">
          <p className="text-lg font-semibold">{t("emptyTitle")}</p>
          <p className="text-sm text-muted-foreground">{t("emptyLead")}</p>
        </div>
      )}
      {groups.map(({ event, items }) => (
        <EventGroup key={event.eventId} event={event} items={items} look={looks[event.brandId] ?? FALLBACK} locale={locale} now={now} />
      ))}
    </div>
  );
}

function EventGroup({ event, items, look, locale, now }: { event: TripEvent; items: TicketItem[]; look: BrandLook; locale: "ro" | "en"; now: number }) {
  const t = useTranslations("trip");
  const tz = event.timezone || "Europe/Bucharest";
  return (
    <section className="overflow-hidden rounded-2xl border" data-testid="trip-event" data-event-id={event.eventId} data-brand={look.slug} style={{ background: look.surface, color: look.onSurface }}>
      <header className="flex items-center gap-3 p-4" style={{ background: look.primary, color: look.onPrimary }} data-testid="trip-event-header" data-primary={look.primary}>
        {look.logo && (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={look.logo} alt="" className="h-8 w-auto max-w-24 shrink-0 object-contain" />
        )}
        <div className="min-w-0">
          <p className="text-xs uppercase tracking-wide opacity-80 break-words">{look.name}</p>
          <h2 className="text-lg font-bold break-words">{event.name[locale] || event.name.en}</h2>
          <p className="text-sm">
            {fmtDate(event.startsAt, locale, tz)} – {fmtDate(event.endsAt, locale, tz)}
          </p>
        </div>
        {event.isSample && <span className="ml-auto shrink-0 rounded bg-white/80 px-2 py-0.5 text-xs font-semibold text-black">{t("sample")}</span>}
      </header>
      <ul className="divide-y">
        {event.pending.map((p) => (
          <li key={p.orderId} className="p-4" data-testid="issuing" data-order-id={p.orderId}>
            <p className="font-semibold">{t("issuing")}</p>
            <p className="text-sm">{t("issuingLead")}</p>
            <p className="text-sm">{p.lines.map((l) => `${l.qty} × ${l.productName[locale] || l.productName.en}`).join(", ")}</p>
          </li>
        ))}
        {items.map((i) => {
          const a = nextAction(event, i, now);
          return (
            <li key={i.id} className="space-y-1 p-4" data-testid="trip-item" data-ticket-id={i.id} data-type={i.productType} data-state={i.state}>
              <div className="flex flex-wrap items-baseline justify-between gap-2">
                <p className="font-semibold break-words">{i.productName[locale] || i.productName.en}</p>
                <span className="text-xs font-semibold" data-testid="trip-item-state">
                  {stateLabel(t, i)}
                </span>
              </div>
              <p className="text-sm">
                {t(`type_${i.productType}` as "type_pass")} · {i.startsAt ? fmtDate(i.startsAt, locale, tz, true) : i.day ? fmtDay(i.day, locale) : fmtDate(event.startsAt, locale, tz)}
              </p>
              {i.holderName && <p className="text-sm">{t("holder", { name: i.holderName })}</p>}
              <div className="flex flex-wrap items-center gap-3 pt-1">
                {a && (
                  <Link href={`/wallet/tickets/${i.id}`} className="rounded-md px-3 py-1.5 text-sm font-semibold" style={{ background: look.primary, color: look.onPrimary }} data-testid="trip-item-action" data-kind={a.kind}>
                    {actionLabel(t, a, locale, tz)}
                  </Link>
                )}
                <Link href={`/wallet/tickets/${i.id}`} className="text-sm font-semibold underline" data-testid="trip-item-open">
                  {t("open")}
                </Link>
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
