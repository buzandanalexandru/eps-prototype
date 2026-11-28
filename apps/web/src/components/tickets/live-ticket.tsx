"use client";
// The live ticket (F3, design D4/D5): a QR that changes every 15 seconds from a buffer of signed
// codes kept in memory only (never storage or a cache), picked by the page's clock corrected to the
// server's. When the buffer runs out without a network it says so instead of showing a stale code.
// Also the ticket's actions: name it, check in (late: the fee from the EPS Wallet), pick the day,
// and the wallet pass preview.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { formatMoney, type Currency } from "@/lib/pricing";
import { qrSvgPath } from "@/lib/tickets/qr";
import { fmtDate, fmtDay } from "@/lib/trip/format";
import type { TicketItem, TripEvent } from "@/lib/trip/timeline";
import { call } from "./api";
import { PassPreview, type PassLook } from "./pass-preview";

type Code = { window: number; validAt: string; expiresAt: string; code: string };
type Codes = { serverTime: string; codes: Code[]; generation: number };

const REFETCH_BELOW_MS = 5 * 60_000;

function QR({ text }: { text: string }) {
  const { path, viewBox } = useMemo(() => qrSvgPath(text), [text]);
  return (
    <svg viewBox={`0 0 ${viewBox} ${viewBox}`} className="h-auto w-full max-w-72" role="img" aria-hidden="true" shapeRendering="crispEdges">
      <rect width={viewBox} height={viewBox} fill="#fff" />
      <path d={path} fill="#000" />
    </svg>
  );
}

export function LiveTicket(props: {
  event: TripEvent;
  item: TicketItem;
  locale: string;
  mode: "account" | "link";
  linkToken?: string;
  look: PassLook;
  /** The shop's base address (/s/<brand>/<locale>) for paying an overdue instalment or topping up. */
  shopBase: string | null;
}) {
  const { event, locale, mode, linkToken, look, shopBase } = props;
  const t = useTranslations("ticket");
  const [item, setItem] = useState(props.item);
  const base = mode === "account" ? `/api/fan/me/tickets/${item.id}` : `/api/fan/ticket-links/${linkToken}/tickets/${item.id}`;
  const name = event.name[locale === "ro" ? "ro" : "en"] || event.name.en;
  const product = item.productName[locale === "ro" ? "ro" : "en"] || item.productName.en;
  const tz = event.timezone || "Europe/Bucharest";
  const canShow = item.state === "valid" && !(item.named && item.needsName);

  // ---- codes: a buffer in memory, the clock offset to the server
  const [buf, setBuf] = useState<{ offset: number; codes: Code[] } | null>(null);
  const fetching = useRef(false);
  const lastTry = useRef(0);
  const [now, setNow] = useState(() => Date.now());
  const [problem, setProblem] = useState<string | null>(null);
  const [loaded, setLoaded] = useState(false);

  const fetchCodes = useCallback(async () => {
    if (fetching.current) return;
    fetching.current = true;
    lastTry.current = Date.now();
    const sent = Date.now();
    const r = await call<Codes>("GET", `${base}/codes`);
    const got = Date.now();
    fetching.current = false;
    if (r.status === 200 && Array.isArray(r.body.codes)) {
      setBuf({ offset: Date.parse(r.body.serverTime) - (sent + got) / 2, codes: r.body.codes });
      setProblem(null);
      setLoaded(true);
    } else if (r.status !== 0 && r.status < 500 && r.status !== 429) {
      setBuf(null);
      setProblem(r.body.code ?? "EPS-UNAVAILABLE");
      setLoaded(true);
    } else {
      setLoaded(true);
    }
  }, [base]);

  useEffect(() => {
    if (!canShow) return;
    const first = window.setTimeout(() => void fetchCodes(), 0);
    const id = window.setInterval(() => setNow(Date.now()), 250);
    return () => {
      window.clearTimeout(first);
      window.clearInterval(id);
    };
  }, [canShow, fetchCodes]);

  const b = buf;
  const corrected = now + (b?.offset ?? 0);
  const current = b?.codes.find((c) => Date.parse(c.validAt) <= corrected && corrected < Date.parse(c.expiresAt)) ?? null;
  const lastEnd = b && b.codes.length ? Date.parse(b.codes[b.codes.length - 1].expiresAt) : 0;
  useEffect(() => {
    if (!canShow || problem) return;
    // Refetch while fewer than 5 minutes remain, at most every 20 seconds while failing.
    if (lastEnd - corrected < REFETCH_BELOW_MS && Date.now() - lastTry.current > 20_000) void fetchCodes();
  }, [canShow, problem, lastEnd, corrected, fetchCodes]);
  const seconds = current ? Math.max(1, Math.ceil((Date.parse(current.expiresAt) - corrected) / 1000)) : 0;

  // ---- actions
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [nameInput, setNameInput] = useState("");
  const [day, setDay] = useState(item.pickDays?.[0] ?? "");
  const act = async (method: "PUT" | "POST", path: string, body: unknown) => {
    setBusy(true);
    setError(null);
    const r = await call<TripEvent>(method, `${base}/${path}`, body);
    setBusy(false);
    if (r.status === 200 && r.body.items?.[0]) {
      const fresh = r.body.items[0];
      if (fresh.generation !== item.generation) {
        // A reissue (a picked day): the old generation's codes are revoked.
        setBuf(null);
        lastTry.current = 0;
      }
      setItem(fresh);
      return;
    }
    setError(r.body.code ?? "EPS-UNAVAILABLE");
  };

  const when = item.startsAt ? fmtDate(item.startsAt, locale, tz, true) : fmtDate(event.startsAt, locale, tz);
  const money = (m: number) => formatMoney(m, event.currency as Currency, locale === "ro" ? "ro" : "en");

  return (
    <article className="space-y-4 rounded-2xl border p-4" data-testid="live-ticket" data-ticket-id={item.id} data-state={item.state}>
      <header className="space-y-1 rounded-xl p-3" style={{ background: look.primary, color: look.onPrimary }}>
        <p className="text-xs uppercase tracking-wide opacity-80">{look.brand}</p>
        <h2 className="text-xl font-bold break-words">{name}</h2>
        <p className="font-semibold break-words" data-testid="ticket-product">
          {product}
        </p>
        <p className="text-sm">{when}</p>
        {item.day && <p className="text-sm">{t("pickedDay", { date: fmtDay(item.day, locale) })}</p>}
        {item.holderName && (
          <p className="text-sm" data-testid="ticket-holder">
            {t("holderLabel")}: {item.holderName}
          </p>
        )}
      </header>

      {item.state === "cancelled" && (
        <p role="status" className="font-semibold" data-testid="ticket-cancelled">
          {t("cancelled")}
        </p>
      )}

      {item.state === "suspended" && (
        <div className="space-y-2 rounded-xl border border-amber-500 p-3" data-testid="ticket-overdue">
          <p className="font-semibold">{t("overdueTitle")}</p>
          {item.overdue && <p className="text-sm">{t("overdueLead", { amount: money(item.overdue.amountMinor) })}</p>}
          {shopBase && (
            <a className="inline-block rounded-md bg-foreground px-3 py-2 text-sm font-semibold text-background" href={`${shopBase}/checkout?order=${item.orderId}`} data-testid="pay-overdue">
              {t("payNow")}
            </a>
          )}
        </div>
      )}

      {item.state === "valid" && item.named && item.needsName && (
        <form
          className="space-y-2 rounded-xl border p-3"
          data-testid="name-form"
          onSubmit={(e) => {
            e.preventDefault();
            void act("PUT", "name", { name: nameInput });
          }}
        >
          <p className="font-semibold">{t("nameTitle")}</p>
          {item.nameLocked ? (
            <p className="text-sm" data-testid="name-locked">
              {t("nameLocked")}
            </p>
          ) : (
            <>
              <p className="text-sm">{t("nameLead")}</p>
              {item.personaliseBy && <p className="text-sm text-muted-foreground">{t("nameBy", { date: fmtDate(item.personaliseBy, locale, tz) })}</p>}
              <label className="grid gap-1 text-sm">
                {t("nameLabel")}
                <input className="w-full rounded-md border bg-background px-2 py-2" value={nameInput} onChange={(e) => setNameInput(e.target.value)} required maxLength={120} autoComplete="name" data-testid="name-input" />
              </label>
              <button type="submit" disabled={busy || !nameInput.trim()} className="rounded-md bg-foreground px-3 py-2 text-sm font-semibold text-background disabled:opacity-50" data-testid="name-save">
                {t("nameSave")}
              </button>
            </>
          )}
        </form>
      )}

      {canShow && (
        <div className="grid justify-items-center gap-2 text-center print:hidden">
          <p className="text-sm font-semibold">{t("codeLabel")}</p>
          {current ? (
            <div className="w-full max-w-72" data-testid="live-code" data-code={current.code} data-window={current.window} aria-label={t("codeLabel")}>
              <QR text={current.code} />
            </div>
          ) : loaded && problem === null ? (
            <div className="space-y-1 p-6" data-testid="reconnect">
              <p className="font-semibold">{t("reconnect")}</p>
              <p className="text-sm text-muted-foreground">{t("reconnectLead")}</p>
            </div>
          ) : problem ? (
            <p role="alert" data-testid="code-problem">
              {t("error", { code: problem })}
            </p>
          ) : (
            <p className="p-6 text-sm" data-testid="code-loading">
              {t("loading")}
            </p>
          )}
          {current && (
            <p className="text-sm tabular-nums" data-testid="countdown">
              {t("changesIn", { seconds })}
            </p>
          )}
          <p className="text-xs text-muted-foreground print:hidden">{t("screenshotNote")}</p>
        </div>
      )}
      <p className="hidden text-sm font-semibold print:block">{t("printNote")}</p>

      {mode === "account" && item.state === "valid" && item.pickDays && item.pickDays.length > 0 && (
        <form
          className="space-y-2 rounded-xl border p-3"
          data-testid="pick-day"
          onSubmit={(e) => {
            e.preventDefault();
            void act("POST", "pick-day", { day });
          }}
        >
          <p className="font-semibold">{t("pickTitle")}</p>
          <p className="text-sm">{t("pickLead")}</p>
          <label className="grid gap-1 text-sm">
            {t("pickLabel")}
            <select className="w-full rounded-md border bg-background px-2 py-2" value={day} onChange={(e) => setDay(e.target.value)} data-testid="pick-day-select">
              {item.pickDays.map((d) => (
                <option key={d} value={d}>
                  {fmtDay(d, locale)}
                </option>
              ))}
            </select>
          </label>
          <button type="submit" disabled={busy || !day} className="rounded-md bg-foreground px-3 py-2 text-sm font-semibold text-background" data-testid="pick-day-save">
            {t("pickSave")}
          </button>
        </form>
      )}

      {item.checkinBy && item.state !== "cancelled" && (
        <div className="space-y-2 rounded-xl border p-3" data-testid="check-in">
          <p className="font-semibold">{t("checkInTitle")}</p>
          {item.checkedInAt ? (
            <p className="text-sm" data-testid="checked-in">
              {t("checkedIn", { date: fmtDate(item.checkedInAt, locale, tz) })}
            </p>
          ) : now < Date.parse(item.checkinBy) || !item.lateCheckinFeeMinor ? (
            <>
              <p className="text-sm">{t("checkInBy", { date: fmtDate(item.checkinBy, locale, tz) })}</p>
              {mode === "account" && (
                <button type="button" disabled={busy || item.state !== "valid"} onClick={() => void act("POST", "check-in", {})} className="rounded-md bg-foreground px-3 py-2 text-sm font-semibold text-background" data-testid="check-in-button">
                  {t("checkIn")}
                </button>
              )}
            </>
          ) : (
            <>
              <p className="text-sm font-semibold">{t("lateTitle")}</p>
              <p className="text-sm">{t("lateLead", { fee: money(item.lateCheckinFeeMinor) })}</p>
              {mode === "account" ? (
                <button type="button" disabled={busy} onClick={() => void act("POST", "check-in", { payLate: true })} className="rounded-md bg-foreground px-3 py-2 text-sm font-semibold text-background" data-testid="late-check-in">
                  {t("latePay", { fee: money(item.lateCheckinFeeMinor) })}
                </button>
              ) : (
                <p className="text-sm">{t("lateSignIn")}</p>
              )}
            </>
          )}
        </div>
      )}

      {error && (
        <div role="alert" className="space-y-1 text-sm" data-testid="ticket-error">
          {error === "EPS-WALLET-INSUFFICIENT" ? (
            <>
              <p>{t("lateInsufficient")}</p>
              {shopBase && (
                <a className="underline" href={`${shopBase}/wallet`}>
                  {t("topUp")}
                </a>
              )}
            </>
          ) : error === "EPS-TICKET-NAME-LOCKED" ? (
            <p>{t("nameLocked")}</p>
          ) : (
            <p>{t("error", { code: error })}</p>
          )}
        </div>
      )}

      {item.state === "valid" && <PassPreview look={look} event={name} product={product} holder={item.holderName ?? null} when={when} />}
    </article>
  );
}
