"use client";
// The locked checkout and the order status page (step 4, D2, D3, D7 to D9, D11 to D13). One static
// page: the island reads the address in the browser. With ?event=&v=&items= it asks the core to
// hold the tickets (one idempotency key per hand-off, so a reload or double click holds once) and
// then shows ?order=<id>; with ?order=<id>[&t=<link>] it shows that order's true state, asked of
// the core every 2 seconds while it can still change. Paid is shown only once the core says so.
// Payment happens on the simulated provider's page (no card data here), from a saved card, or
// from the EPS Wallet; every Pay press carries its own idempotency key.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { fill, type ShopLabels } from "@/lib/storefront/labels";
import type { Locale } from "@/lib/storefront/schema";
import type { Currency } from "@/lib/pricing";
import { parseItems } from "@/lib/storefront/picker";
import { money, shopDate } from "./all-in-price";
import { newKey, shopApi, type OrderView, type Problem } from "./shop-api";

type Props = { labels: ShopLabels; locale: Locale; base: string };

const btn = "inline-flex min-h-11 items-center justify-center rounded-[var(--eps-radius)] px-5 py-2 text-sm font-semibold disabled:cursor-not-allowed disabled:opacity-50";
const primary = `${btn} bg-[var(--eps-primary)] text-[var(--eps-on-primary)]`;
const secondary = `${btn} border-2 border-[var(--eps-primary)] text-[var(--eps-primary)]`;
const box = "rounded-[var(--eps-radius)] border border-[color-mix(in_srgb,var(--eps-on-surface)_15%,transparent)] bg-[color-mix(in_srgb,var(--eps-on-surface)_4%,var(--eps-surface))] p-4";
const input = "min-h-11 w-full rounded-[var(--eps-radius)] border border-[color-mix(in_srgb,var(--eps-on-surface)_30%,transparent)] bg-[var(--eps-surface)] px-3 py-2";
const LIVE = ["held", "paying", "payment_failed", "paid"];

const tx = (v: { ro: string; en: string } | undefined, l: Locale) => (v ? (v[l] || v[l === "ro" ? "en" : "ro"] || "").trim() : "");

export function messageFor(labels: ShopLabels, p: Pick<Problem, "code" | "messageKey">): string {
  const key = (p.messageKey ?? "") as keyof ShopLabels;
  const v = key && Object.hasOwn(labels, key) ? labels[key] : null;
  return typeof v === "string" ? v : labels.err_generic;
}

function ErrorBox({ labels, problem, testId = "checkout-error" }: { labels: ShopLabels; problem: Pick<Problem, "code" | "messageKey">; testId?: string }) {
  return (
    <div role="alert" className={`${box} border-[var(--eps-primary)]`} data-testid={testId}>
      <p className="font-semibold">{messageFor(labels, problem)}</p>
      {problem.code.startsWith("EPS-") && (
        <p className="text-sm">
          {labels.errorCode}: <code data-testid="error-code">{problem.code}</code>
        </p>
      )}
    </div>
  );
}

/** Seconds left on the hold, from the server's deadline and clock (not from page load). */
function useHoldLeft(order: OrderView | null, skew: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), 500);
    return () => clearInterval(t);
  }, []);
  if (!order) return 0;
  return Math.max(0, Math.floor((Date.parse(order.holdExpiresAt) - (now + skew)) / 1000));
}

function clock(s: number): string {
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, "0")}`;
}

/** The language links keep the address (the order and its link) on this static page. */
function keepQueryOnLanguageLinks() {
  for (const a of document.querySelectorAll<HTMLAnchorElement>('a[data-testid^="shop-lang-"]')) {
    a.href = `${a.href.split("?")[0]}${window.location.search}`;
  }
}

type Load = { state: "loading" } | { state: "ready"; order: OrderView } | { state: "error"; problem: Problem };

export function Checkout({ labels, locale, base }: Props) {
  const [load, setLoad] = useState<Load>({ state: "loading" });
  const [skew, setSkew] = useState(0);
  const [orderId, setOrderId] = useState<string | null>(null);
  const [link, setLink] = useState<string | undefined>(undefined);

  const show = useCallback((o: OrderView) => {
    setSkew(Date.parse(o.serverTime) - Date.now());
    setLoad({ state: "ready", order: o });
    keepQueryOnLanguageLinks();
  }, []);

  const refresh = useCallback(async () => {
    if (!orderId) return;
    const r = await shopApi<OrderView>(`orders/${orderId}`, { orderToken: link });
    if (r.ok) show(r.data);
    else if (r.status !== 0) setLoad({ state: "error", problem: r.problem });
  }, [orderId, link, show]);

  // The address decides: an order to show, or a hand-off to hold.
  useEffect(() => {
    keepQueryOnLanguageLinks();
    const sp = new URLSearchParams(window.location.search);
    const id = sp.get("order");
    const t = sp.get("t") ?? undefined;
    if (id && /^[0-9a-f-]{36}$/.test(id)) {
      setLink(t);
      setOrderId(id);
      return;
    }
    const event = sp.get("event") ?? "";
    const v = Number(sp.get("v"));
    const items = parseItems(sp.get("items"));
    if (!/^[0-9a-f-]{36}$/.test(event) || !Number.isInteger(v) || v < 1 || !items) {
      setLoad({ state: "error", problem: { type: "", title: "", status: 422, code: "EPS-HOLD-INVALID", messageKey: "err_hold_invalid", requestId: "" } });
      return;
    }
    // One key per hand-off: a reload or a double click on Continue holds once.
    const handoff = `eps-hold:${event}:${v}:${sp.get("items")}`;
    let key: string | null = null;
    try {
      key = sessionStorage.getItem(handoff);
      if (!key) {
        key = newKey();
        sessionStorage.setItem(handoff, key);
      }
    } catch {
      key = newKey();
    }
    (async () => {
      const r = await shopApi<OrderView>("orders", {
        method: "POST",
        key: key!,
        body: { eventId: event, version: v, locale, items: items.map((l) => ({ productId: l.productId, qty: l.qty, ...(l.instance ? { instance: l.instance } : {}) })) },
      });
      if (!r.ok) {
        setLoad({ state: "error", problem: r.problem });
        return;
      }
      window.history.replaceState(null, "", `${window.location.pathname}?order=${r.data.id}`);
      setOrderId(r.data.id);
      show(r.data);
    })();
  }, [locale, show]);

  useEffect(() => {
    if (!orderId) return;
    void refresh();
  }, [orderId, refresh]);

  // Poll every 2 seconds while the order can still change.
  const state = load.state === "ready" ? load.order.state : null;
  useEffect(() => {
    if (!orderId || !state || !LIVE.includes(state)) return;
    const t = setInterval(() => void refresh(), 2000);
    return () => clearInterval(t);
  }, [orderId, state, refresh]);

  return (
    <div className="mx-auto max-w-3xl space-y-4 px-4 py-8" data-testid="checkout" data-state={state ?? load.state}>
      <h1 className="font-[family-name:var(--eps-font-heading)] text-3xl font-bold">{labels.checkoutTitle}</h1>
      <p className="text-sm" role="note" data-testid="simulated-payments">
        {labels.simulatedPayments}
      </p>
      {load.state === "loading" && (
        <p role="status" data-testid="checkout-loading">
          {labels.holding}
        </p>
      )}
      {load.state === "error" && <Failed labels={labels} problem={load.problem} base={base} />}
      {load.state === "ready" && <OrderScreen order={load.order} labels={labels} locale={locale} base={base} skew={skew} link={link} onChange={show} refresh={refresh} />}
    </div>
  );
}

function Failed({ labels, problem, base }: { labels: ShopLabels; problem: Problem; base: string }) {
  const notFound = problem.code === "EPS-ORDER-NOT-FOUND" || problem.status === 404;
  const invalid = problem.code === "EPS-LINK-INVALID";
  return (
    <div className="space-y-3">
      {notFound ? (
        <p className="text-xl font-semibold" data-testid="order-not-found">
          {labels.orderNotFound}
        </p>
      ) : invalid ? (
        <p className="text-xl font-semibold" data-testid="link-invalid">
          {labels.linkInvalid}
        </p>
      ) : (
        <ErrorBox labels={labels} problem={problem} />
      )}
      <a href={base} className="text-sm font-semibold underline">
        {labels.backToShop}
      </a>
    </div>
  );
}

function Lines({ order, labels, locale }: { order: OrderView; labels: ShopLabels; locale: Locale }) {
  const cur = order.currency as Currency;
  return (
    <div className={box}>
      <p className="font-semibold" data-testid="order-event">
        {tx(order.eventName, locale)}
      </p>
      <ul className="mt-2 space-y-1" data-testid="checkout-lines">
        {order.lines.map((l) => (
          <li key={l.lineNo} className="flex flex-wrap justify-between gap-2" data-testid="checkout-line">
            <span>
              {l.qty} × {tx(l.name, locale)}
              {l.instance ? ` · ${l.instance.includes("T") ? `${shopDate(`${l.instance.slice(0, 10)}T12:00:00Z`, locale, "UTC")}, ${l.instance.slice(11)}` : shopDate(`${l.instance}T12:00:00Z`, locale, "UTC")}` : ""}
            </span>
            <span>{money(l.qty * l.unitAllInMinor, cur, locale)}</span>
          </li>
        ))}
      </ul>
      <p className="mt-2 flex justify-between border-t border-[color-mix(in_srgb,var(--eps-on-surface)_15%,transparent)] pt-2 text-lg font-bold">
        <span>{labels.total}</span>
        <span data-testid="checkout-total">{money(order.totalMinor, cur, locale)}</span>
      </p>
    </div>
  );
}

function Copy({ text, labels, testId }: { text: string; labels: ShopLabels; testId: string }) {
  const [done, setDone] = useState(false);
  return (
    <div className="flex flex-wrap items-center gap-2">
      <input readOnly value={text} className={`${input} min-w-0 flex-1 text-xs`} data-testid={testId} aria-label={testId} onFocus={(e) => e.currentTarget.select()} />
      <button
        type="button"
        className={secondary}
        onClick={async () => {
          try {
            await navigator.clipboard.writeText(text);
            setDone(true);
          } catch {
            setDone(false);
          }
        }}
      >
        {done ? labels.copied : labels.copy}
      </button>
    </div>
  );
}

type ScreenProps = { order: OrderView; labels: ShopLabels; locale: Locale; base: string; skew: number; link?: string; onChange: (o: OrderView) => void; refresh: () => Promise<void> };

function OrderScreen(props: ScreenProps) {
  const { order, labels, locale, base, skew } = props;
  const left = useHoldLeft(order, skew);
  const here = typeof window !== "undefined" ? window.location.pathname : `${base}/checkout`;
  const resumeHref = order.resumeToken && typeof window !== "undefined" ? `${window.location.origin}${here}?order=${order.id}&t=${encodeURIComponent(order.resumeToken)}` : null;
  const viewHref = order.viewToken && typeof window !== "undefined" ? `${window.location.origin}${here}?order=${order.id}&t=${encodeURIComponent(order.viewToken)}` : null;
  const ended = (order.state === "held" || order.state === "payment_failed") && left <= 0;
  const lastCode = order.lastErrorCode ?? undefined;

  return (
    <div className="space-y-4" data-testid="order" data-order-id={order.id}>
      <p className="text-sm">
        {labels.orderState}: <span data-testid="order-state">{order.state}</span>
      </p>
      <Lines order={order} labels={labels} locale={locale} />
      {(order.state === "held" || order.state === "payment_failed" || order.state === "paying") && !ended && (
        <p className="text-lg font-semibold" role="timer" data-testid="hold-timer" data-seconds={left}>
          {fill(labels.holdTimer, { time: clock(left) })}
        </p>
      )}
      {order.state === "payment_failed" && lastCode && <ErrorBox labels={labels} problem={{ code: lastCode, messageKey: `err_${lastCode.slice(4).toLowerCase().replaceAll("-", "_")}` }} testId="payment-error" />}
      {(order.state === "expired" || ended || (order.access === "expired_link" && (order.state === "held" || order.state === "payment_failed"))) && (
        <div className="space-y-2" data-testid="hold-ended">
          <p className="font-semibold">{labels.holdEnded}</p>
          <a href={base} className={primary} data-testid="start-new">
            {labels.startNew}
          </a>
        </div>
      )}
      {order.canPay && !ended && <PayForm {...props} />}
      {order.state === "paying" && <Waiting order={order} labels={labels} />}
      {(order.state === "paid" || order.state === "issued") && (
        <div className={box} data-testid="order-paid">
          <p className="text-xl font-bold">{order.state === "issued" ? labels.issuedTitle : labels.paidTitle}</p>
          <p>{labels.paidLead}</p>
        </div>
      )}
      {order.plan && order.plan.state !== "pending" && <PlanView {...props} />}
      {order.state === "cancelled" && (
        <p className="text-xl font-semibold" data-testid="order-cancelled">
          {labels.cancelledTitle}
        </p>
      )}
      {order.state === "refunded" && (
        <div className="space-y-2" data-testid="order-refunded">
          <p className="text-xl font-semibold">{labels.refundedTitle}</p>
          {lastCode && <ErrorBox labels={labels} problem={{ code: lastCode, messageKey: `err_${lastCode.slice(4).toLowerCase().replaceAll("-", "_")}` }} />}
        </div>
      )}
      {resumeHref && (order.state === "held" || order.state === "payment_failed") && (
        <div className="space-y-1" data-testid="resume">
          <p className="text-sm font-semibold">{labels.resumeLink}</p>
          <Copy text={resumeHref} labels={labels} testId="resume-link" />
        </div>
      )}
      {viewHref && <GuestLinks order={order} labels={labels} viewHref={viewHref} />}
      <a href={base} className="text-sm font-semibold underline">
        {labels.backToShop}
      </a>
    </div>
  );
}

function GuestLinks({ order, labels, viewHref }: { order: OrderView; labels: ShopLabels; viewHref: string }) {
  const [claim, setClaim] = useState<"idle" | "done" | Problem>("idle");
  useEffect(() => {
    // Keep the order link in the address, so the page still opens it after signing in.
    const u = new URL(window.location.href);
    if (!u.searchParams.get("t")) window.history.replaceState(null, "", viewHref.slice(u.origin.length));
  }, [viewHref]);
  return (
    <div className="space-y-2" data-testid="guest-links">
      <p className="text-sm font-semibold">{labels.orderLink}</p>
      <Copy text={viewHref} labels={labels} testId="order-link" />
      {(order.state === "paid" || order.state === "issued") &&
        (claim === "done" ? (
          <p data-testid="claimed">{labels.addedToAccount}</p>
        ) : (
          <>
            <button
              type="button"
              className={secondary}
              data-testid="add-to-account"
              onClick={async () => {
                const r = await shopApi<{ claimed: string[] }>("claims", { method: "POST", body: { orderId: order.id } });
                if (r.ok) setClaim("done");
                else if (r.problem.code === "EPS-FORBIDDEN") {
                  window.location.assign(`/sign-in?next=${encodeURIComponent(window.location.pathname + window.location.search)}`);
                } else setClaim(r.problem);
              }}
            >
              {labels.addToAccount}
            </button>
            {claim !== "idle" && typeof claim === "object" && <ErrorBox labels={labels} problem={claim} />}
          </>
        ))}
    </div>
  );
}

function Waiting({ order, labels }: { order: OrderView; labels: ShopLabels }) {
  const action = order.payment?.state === "challenge_required" || order.payment?.state === "submitted" ? order.payment.actionUrl : undefined;
  return (
    <div className={box} role="status" data-testid="waiting-provider">
      <p className="font-semibold">{labels.waitingProvider}</p>
      {action && (
        <a href={action} className={`${secondary} mt-2`} data-testid="continue-at-bank">
          {labels.continueAtBank}
        </a>
      )}
    </div>
  );
}

type Method = { id: string; method: "hosted_card" | "apple_pay" | "google_pay" | "saved_card" | "wallet"; label: string; savedCardId?: string };

function PayForm({ order, labels, locale, link, onChange, refresh }: ScreenProps) {
  const cur = order.currency as Currency;
  const [name, setName] = useState(order.buyerName ?? "");
  const [email, setEmail] = useState(order.buyerEmail ?? "");
  const [answers, setAnswers] = useState<Record<string, unknown>>(() => ({ ...(order.answers ?? {}) }));
  const [plan, setPlan] = useState<"full" | "instalments">(order.payPlan === "instalments" ? "instalments" : "full");
  const [missing, setMissing] = useState<string[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<Problem | null>(null);
  const [saveCard, setSaveCard] = useState(false);
  const press = useRef<string | null>(null);
  const walletCovers = !!order.wallet && order.wallet.balanceMinor + order.wallet.creditMinor >= order.totalMinor && order.access === "buyer";
  const methods: Method[] = useMemo(() => {
    const out: Method[] = [];
    if (plan === "full" && walletCovers) out.push({ id: "wallet", method: "wallet", label: fill(labels.methodWallet, { balance: money(order.wallet!.balanceMinor + order.wallet!.creditMinor, cur, locale) }) });
    for (const c of order.savedCards) out.push({ id: `card-${c.id}`, method: "saved_card", label: fill(labels.savedCard, { label: c.label }), savedCardId: c.id });
    out.push({ id: "hosted_card", method: "hosted_card", label: labels.methodCard });
    if (plan === "full") {
      out.push({ id: "apple_pay", method: "apple_pay", label: labels.methodApple });
      out.push({ id: "google_pay", method: "google_pay", label: labels.methodGoogle });
    }
    return out;
  }, [plan, walletCovers, order.wallet, order.savedCards, labels, cur, locale]);
  const [choice, setChoice] = useState<string>("hosted_card");
  const chosen = methods.find((m) => m.id === choice) ?? methods.find((m) => m.id === "hosted_card")!;
  const offer = order.instalmentOffer;
  const amount = plan === "instalments" && offer ? offer.depositMinor : order.totalMinor;

  async function pay(m: Method) {
    setBusy(true);
    setError(null);
    // Required questions and consents first: payment does not start without them.
    const req = order.questions.filter((q) => q.required && (q.kind === "consent" ? answers[q.id] !== true : answers[q.id] === undefined || answers[q.id] === ""));
    const miss = [...(name.trim() ? [] : ["name"]), ...(email.trim() ? [] : ["email"]), ...req.map((q) => q.id)];
    setMissing(miss);
    if (miss.length) {
      setBusy(false);
      return;
    }
    const d = await shopApi<OrderView>(`orders/${order.id}/details`, { method: "PUT", orderToken: link, body: { name, email, answers } });
    if (!d.ok) {
      const m2 = (d.problem.data as { missing?: string[] } | undefined)?.missing;
      if (m2) setMissing(m2);
      setError(d.problem);
      setBusy(false);
      return;
    }
    // One key per Pay press, kept for retries of the same press.
    if (!press.current) press.current = newKey();
    const here = `${window.location.origin}${window.location.pathname}?order=${order.id}${link ? `&t=${encodeURIComponent(link)}` : ""}`;
    const r = await shopApi<{ order: OrderView; actionUrl?: string }>(`orders/${order.id}/payments`, {
      method: "POST",
      key: press.current,
      orderToken: link,
      body: { method: m.method, ...(m.savedCardId ? { savedCardId: m.savedCardId } : {}), ...(saveCard && m.method === "hosted_card" ? { saveCard: true } : {}), plan, returnUrl: here },
    });
    if (!r.ok) {
      if (!r.problem.retryable) press.current = null;
      setError(r.problem);
      setBusy(false);
      void refresh();
      return;
    }
    press.current = null;
    if (r.data.actionUrl) {
      window.location.assign(r.data.actionUrl);
      return;
    }
    onChange(r.data.order);
    setBusy(false);
  }

  async function cancel() {
    setBusy(true);
    const r = await shopApi<OrderView>(`orders/${order.id}/cancel`, { method: "POST", orderToken: link });
    if (r.ok) onChange(r.data);
    else setError(r.problem);
    setBusy(false);
  }

  const afterContact = order.questions.filter((q) => q.slot === "afterContact");
  const consents = order.questions.filter((q) => q.slot === "consents");
  const need = (id: string) => missing.includes(id);

  return (
    <form
      className="space-y-4"
      data-testid="pay-form"
      onSubmit={(e) => {
        e.preventDefault();
        void pay(chosen);
      }}
      noValidate
    >
      <fieldset className="space-y-3">
        <legend className="font-semibold">{labels.contact}</legend>
        <label className="grid gap-1 text-sm">
          {labels.name}
          <input className={input} name="name" autoComplete="name" value={name} onChange={(e) => setName(e.target.value)} aria-invalid={need("name")} data-testid="contact-name" />
        </label>
        <label className="grid gap-1 text-sm">
          {labels.email}
          <input className={input} name="email" type="email" autoComplete="email" value={email} onChange={(e) => setEmail(e.target.value)} aria-invalid={need("email")} data-testid="contact-email" />
        </label>
      </fieldset>
      {afterContact.length > 0 && (
        <fieldset className="space-y-3" data-testid="questions">
          <legend className="font-semibold">{labels.slotQuestions}</legend>
          {afterContact.map((q) => (
            <label key={q.id} className="grid gap-1 text-sm" data-testid={`question-${q.id}`}>
              <span>
                {tx(q.label, locale)} <span className="text-xs">({q.required ? labels.required : labels.optional})</span>
              </span>
              {q.kind === "choice" ? (
                <select className={input} value={typeof answers[q.id] === "number" ? String(answers[q.id]) : ""} onChange={(e) => setAnswers({ ...answers, [q.id]: e.target.value === "" ? undefined : Number(e.target.value) })} aria-invalid={need(q.id)}>
                  <option value="">{labels.choose}</option>
                  {q.options.map((o, i) => (
                    <option key={i} value={i}>
                      {tx(o, locale)}
                    </option>
                  ))}
                </select>
              ) : (
                <input className={input} value={typeof answers[q.id] === "string" ? (answers[q.id] as string) : ""} maxLength={500} onChange={(e) => setAnswers({ ...answers, [q.id]: e.target.value })} aria-invalid={need(q.id)} />
              )}
            </label>
          ))}
        </fieldset>
      )}
      {consents.length > 0 && (
        <fieldset className="space-y-2" data-testid="consents">
          <legend className="font-semibold">{labels.slotConsents}</legend>
          {consents.map((q) => (
            <div key={q.id}>
              <label className="flex items-start gap-2 text-sm" data-testid={`consent-${q.id}`}>
                <input type="checkbox" className="mt-1 h-5 w-5" checked={answers[q.id] === true} onChange={(e) => setAnswers({ ...answers, [q.id]: e.target.checked ? true : undefined })} aria-invalid={need(q.id)} />
                <span>
                  {tx(q.label, locale)} <span className="text-xs">({q.required ? labels.required : labels.optional})</span>
                </span>
              </label>
              {need(q.id) && (
                <p className="text-sm font-semibold" role="alert" data-testid="consent-needed">
                  {labels.consentNeeded}
                </p>
              )}
            </div>
          ))}
        </fieldset>
      )}
      {offer && (
        <fieldset className="space-y-2" data-testid="plan-choice">
          <legend className="font-semibold">{labels.payment}</legend>
          <label className="flex items-center gap-2">
            <input type="radio" name="plan" checked={plan === "full"} onChange={() => setPlan("full")} data-testid="plan-full" /> {labels.payInFull}
          </label>
          <label className="flex items-center gap-2">
            <input
              type="radio"
              name="plan"
              checked={plan === "instalments"}
              onChange={() => {
                setPlan("instalments");
                setChoice("hosted_card");
              }}
              data-testid="plan-instalments"
            />{" "}
            {labels.payInstalments}
          </label>
          {plan === "instalments" && (
            <div className={box} data-testid="schedule">
              <p className="font-semibold">{labels.scheduleTitle}</p>
              <ul className="text-sm">
                <li className="flex justify-between gap-2" data-testid="schedule-deposit">
                  <span>{labels.depositNow}</span>
                  <span>{money(offer.depositMinor, cur, locale)}</span>
                </li>
                {offer.payments.map((p) => (
                  <li key={p.n} className="flex justify-between gap-2" data-testid="schedule-part">
                    <span>{shopDate(`${p.dueOn}T12:00:00Z`, locale, "UTC")}</span>
                    <span>{money(p.amountMinor, cur, locale)}</span>
                  </li>
                ))}
                <li className="flex justify-between gap-2 font-semibold">
                  <span>{labels.total}</span>
                  <span data-testid="schedule-total">{money(offer.totalMinor, cur, locale)}</span>
                </li>
              </ul>
            </div>
          )}
        </fieldset>
      )}
      <fieldset className="space-y-2" data-testid="methods">
        <legend className="font-semibold">{labels.method}</legend>
        {methods.map((m) => (
          <label key={m.id} className="flex min-h-11 items-center gap-2" data-testid={`method-${m.method}`}>
            <input type="radio" name="method" value={m.id} checked={chosen.id === m.id} onChange={() => setChoice(m.id)} /> {m.label}
          </label>
        ))}
        {chosen.method === "hosted_card" && order.buyerKind === "account" && order.access === "buyer" && plan === "full" && (
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={saveCard} onChange={(e) => setSaveCard(e.target.checked)} data-testid="save-card" /> {labels.saveCard}
          </label>
        )}
      </fieldset>
      {error && <ErrorBox labels={labels} problem={error} />}
      <div className="flex flex-wrap gap-2">
        <button type="submit" className={primary} disabled={busy} data-testid="pay">
          {busy ? labels.sending : order.state === "payment_failed" ? labels.tryAgain : fill(labels.payAmount, { amount: money(amount, cur, locale) })}
        </button>
        <button type="button" className={secondary} disabled={busy} onClick={() => void cancel()} data-testid="cancel-order">
          {labels.cancelOrder}
        </button>
      </div>
    </form>
  );
}

function PlanView({ order, labels, locale, link, onChange }: ScreenProps) {
  const plan = order.plan!;
  const cur = order.currency as Currency;
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<Problem | null>(null);
  const act = async (path: string) => {
    setBusy(true);
    setError(null);
    const r = await shopApi<OrderView>(`orders/${order.id}/${path}`, { method: "POST", orderToken: link });
    if (r.ok) onChange(r.data);
    else setError(r.problem);
    setBusy(false);
  };
  const stateLabel = (p: (typeof plan.parts)[number]) =>
    p.state === "paid" ? labels.partPaid : p.state === "overdue" ? fill(labels.partOverdue, { date: p.graceUntil ? shopDate(`${p.graceUntil}T12:00:00Z`, locale, "UTC") : "" }) : p.state === "charging" ? labels.partCharging : p.state === "cancelled" ? labels.partCancelled : labels.partScheduled;
  const overdue = plan.parts.some((p) => p.state === "overdue");
  return (
    <div className={box} data-testid="plan" data-plan-state={plan.state}>
      <p className="font-semibold">{labels.planTitle}</p>
      {plan.savedCardLabel && <p className="text-sm" data-testid="plan-card">{fill(labels.cardOnFile, { label: plan.savedCardLabel })}</p>}
      <ul className="mt-2 space-y-1 text-sm">
        <li className="flex justify-between gap-2">
          <span>{labels.depositNow}</span>
          <span>
            {money(plan.depositMinor, cur, locale)} · {labels.partPaid}
          </span>
        </li>
        {plan.parts.map((p) => (
          <li key={p.n} className="flex flex-wrap justify-between gap-2" data-testid="plan-part" data-part-state={p.state}>
            <span>{shopDate(`${p.dueOn}T12:00:00Z`, locale, "UTC")}</span>
            <span>
              {money(p.amountMinor, cur, locale)} · {stateLabel(p)}
            </span>
          </li>
        ))}
      </ul>
      {order.access === "buyer" && plan.state === "active" && (
        <div className="mt-3 flex flex-wrap gap-2">
          <button type="button" className={secondary} disabled={busy} onClick={() => void act("instalments/simulate-next")} data-testid="simulate-next">
            {labels.simulateNext}
          </button>
          {overdue && (
            <>
              <button type="button" className={primary} disabled={busy} onClick={() => void act("instalments/pay-overdue")} data-testid="pay-overdue">
                {labels.payOverdue}
              </button>
              <button type="button" className={secondary} disabled title={labels.changeCardLater} data-testid="change-card">
                {labels.changeCard}
              </button>
            </>
          )}
        </div>
      )}
      {error && <ErrorBox labels={labels} problem={error} />}
    </div>
  );
}
