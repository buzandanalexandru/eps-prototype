"use client";
// The waiting room island (step 5, design D-2 to D-5). It joins once (with the proof of work) and
// then asks for its place when the gateway says (about every 10 s, with jitter). Before the
// pre-queue opens nothing is taken; in the pre-queue it counts down to the draw (arrival time
// gives no advantage); in line it shows the fans ahead and an estimate from the current rate; a
// paused sale says the place is safe. Once admitted it keeps the signed pass for the event and goes
// on to the checkout. Everything is the fan's own browser state: a reload or a second tab finds
// the same place.
import { useEffect, useState } from "react";
import { fill, type ShopLabels } from "@/lib/storefront/labels";
import type { Locale } from "@/lib/storefront/schema";
import { shopApi, type Problem } from "./shop-api";
import { browserPrint, clearPass, queueToken, savePass, saveQueueToken, sessionSecret, sha256Hex, solvePow, tabId } from "./queue-client";
import type { components } from "@/lib/core/schema";

type QueueStatus = components["schemas"]["QueueStatus"];
type Props = { labels: ShopLabels; locale: Locale; base: string };
type View = { kind: "loading" } | { kind: "status"; status: QueueStatus; skew: number } | { kind: "error"; problem: Problem } | { kind: "retrying" };

const btn = "inline-flex min-h-11 items-center justify-center rounded-[var(--eps-radius)] px-5 py-2 text-sm font-semibold";
const primary = `${btn} bg-[var(--eps-primary)] text-[var(--eps-on-primary)]`;
const box = "rounded-[var(--eps-radius)] border border-[color-mix(in_srgb,var(--eps-on-surface)_15%,transparent)] bg-[color-mix(in_srgb,var(--eps-on-surface)_4%,var(--eps-surface))] p-4";
const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

function placeCode(token: string | null): string {
  try {
    const body = JSON.parse(atob(token!.split(".")[1].replace(/-/g, "+").replace(/_/g, "/"))) as { qid?: string };
    return (body.qid ?? "").slice(0, 6).toUpperCase();
  } catch {
    return "";
  }
}

function clock(ms: number): string {
  const s = Math.max(0, Math.floor(ms / 1000));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const r = String(s % 60).padStart(2, "0");
  return h > 0 ? `${h}:${String(m).padStart(2, "0")}:${r}` : `${m}:${r}`;
}

/** "N fans ahead of you" with the language's plural forms (RO: 1, 2-19, from 20 "de"). */
export function aheadText(n: number, locale: Locale, labels: ShopLabels): string {
  const form = new Intl.PluralRules(locale === "ro" ? "ro-RO" : "en-GB").select(n);
  const label = form === "one" ? labels.queueAheadOne : form === "few" ? labels.queueAheadFew : labels.queueAheadOther;
  return fill(label, { n: count(n, locale) });
}

/** A whole number with the locale's thousands separator (no money here). */
function count(n: number, locale: Locale): string {
  return String(Math.trunc(n)).replace(/\B(?=(\d{3})+(?!\d))/g, locale === "ro" ? "." : ",");
}

type Target = { sale: string; next: string } | null;

export function WaitingRoom({ labels, locale, base }: Props) {
  const [view, setView] = useState<View>({ kind: "loading" });
  const [now, setNow] = useState(() => Date.now());
  const [target, setTarget] = useState<Target>(null);
  const [attempt, setAttempt] = useState(0);
  const fmt = new Intl.DateTimeFormat(locale === "ro" ? "ro-RO" : "en-GB", { dateStyle: "long", timeStyle: "short" });

  // The address names the sale and where to go once admitted (only an address of this shop).
  useEffect(() => {
    const sp = new URLSearchParams(window.location.search);
    const s = sp.get("sale") ?? "";
    const n = sp.get("next") ?? "";
    for (const a of document.querySelectorAll<HTMLAnchorElement>('a[data-testid^="shop-lang-"]')) a.href = `${a.href.split("?")[0]}${window.location.search}`;
    queueMicrotask(() => {
      if (!UUID.test(s)) setView({ kind: "error", problem: { type: "", title: "", status: 404, code: "EPS-SALE-NOT-FOUND", messageKey: "err_sale_not_found", requestId: "" } });
      else setTarget({ sale: s, next: n.startsWith(`${base}/`) && !n.startsWith("//") ? n : base });
    });
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(tick);
  }, [base]);

  // The loop: join once (or find the place again), then ask when the gateway says.
  useEffect(() => {
    if (!target) return;
    const sale = target.sale;
    let timer: ReturnType<typeof setTimeout> | undefined;
    let stopped = false;
    let failures = 0;
    const later = (ms: number, f: () => void) => {
      if (!stopped) timer = setTimeout(f, Math.max(500, ms));
    };
    const show = (st: QueueStatus) => {
      setView({ kind: "status", status: st, skew: Date.parse(st.serverTime) - Date.now() });
      if (st.queueToken) saveQueueToken(st.saleId, st.queueToken);
      if (st.state === "admitted" && st.pass) {
        savePass(st.eventId, st.pass, st.passExpiresAt ?? Date.now() + 600_000);
        later(1200, () => window.location.assign(target.next));
      }
      if (st.state === "ended") {
        clearPass(st.eventId);
        saveQueueToken(st.saleId, null);
      }
    };
    const failed = (problem: Problem, status: number) => {
      if (problem.code === "EPS-SIGN-IN-REQUIRED") {
        setView({ kind: "error", problem });
        return;
      }
      // A sale not found yet (its copy is on its way after a restart) is asked again with a growing
      // wait (review spec M4): never a dead end for a well-formed sale id.
      void status;
      // Unreachable or busy: try again with a growing, jittered wait (never a block).
      failures += 1;
      setView((v) => (v.kind === "status" ? v : { kind: "retrying" }));
      const wait = Math.min(30_000, 2000 * 2 ** Math.min(failures, 4)) * (0.75 + Math.random() * 0.5);
      later(problem.status === 429 ? Math.max(wait, 30_000) : wait, () => void (queueToken(sale) ? poll() : join()));
    };
    const join = async (): Promise<void> => {
      const session = sessionSecret();
      await new Promise((r) => setTimeout(r, 0)); // paint first: the proof of work takes a moment
      const pow = solvePow(sale, sha256Hex(session));
      const r = await shopApi<QueueStatus>(`queue/${sale}/join`, { method: "POST", body: { pow, print: browserPrint() }, headers: { "X-EPS-Session": session, "X-EPS-Tab": tabId() } });
      if (stopped) return;
      if (!r.ok) return failed(r.problem, r.status);
      failures = 0;
      show(r.data);
      if (r.data.state === "before" || r.data.state === "notopen") later(r.data.nextPollMs, () => void join());
      else if (r.data.state !== "admitted" && r.data.state !== "ended") later(r.data.nextPollMs, () => void poll());
    };
    const poll = async (): Promise<void> => {
      const qt = queueToken(sale);
      if (!qt) return join();
      const r = await shopApi<QueueStatus>(`queue/${sale}/status`, { headers: { "X-EPS-Queue-Token": qt, "X-EPS-Session": sessionSecret(), "X-EPS-Tab": tabId() } });
      if (stopped) return;
      if (!r.ok) {
        if (r.status === 401) {
          saveQueueToken(sale, null);
          return join();
        }
        return failed(r.problem, r.status);
      }
      failures = 0;
      show(r.data);
      if (r.data.state !== "admitted" && r.data.state !== "ended") later(r.data.nextPollMs, () => void poll());
    };
    void (queueToken(sale) && attempt === 0 ? poll() : join());
    return () => {
      stopped = true;
      clearTimeout(timer);
    };
  }, [target, attempt]);

  const st = view.kind === "status" ? view.status : null;
  const at = (iso: string) => fmt.format(new Date(iso));
  const serverNow = now + (view.kind === "status" ? view.skew : 0);
  const est = st?.estimateSeconds ?? null;
  return (
    <div className="mx-auto max-w-2xl space-y-4 px-4 py-8" data-testid="waiting-room" data-state={st?.state ?? view.kind}>
      <h1 className="font-[family-name:var(--eps-font-heading)] text-3xl font-bold">{labels.queueTitle}</h1>
      {view.kind === "loading" && <p role="status">{labels.queueLoading}</p>}
      {view.kind === "retrying" && <p role="status">{labels.queueRetrying}</p>}
      {view.kind === "error" && (
        <div className="space-y-3">
          <p role="alert" className="text-xl font-semibold" data-testid="queue-error">
            {view.problem.code === "EPS-SIGN-IN-REQUIRED" ? labels.queueSignIn : view.problem.code === "EPS-SALE-NOT-FOUND" ? labels.queueNoSale : labels.err_generic}
          </p>
          {view.problem.code === "EPS-SIGN-IN-REQUIRED" && (
            <a href={`/sign-in?next=${encodeURIComponent(typeof window !== "undefined" ? window.location.pathname + window.location.search : base)}`} className={primary} data-testid="queue-sign-in">
              {labels.queueSignInLink}
            </a>
          )}
          <a href={base} className="text-sm font-semibold underline">
            {labels.backToShop}
          </a>
        </div>
      )}
      {st && (
        <div className="space-y-4" aria-live="polite">
          {st.paused && (st.state === "waiting" || st.state === "shuffling") && (
            <p role="status" className={`${box} font-semibold`} data-testid="queue-paused">
              {labels.queuePaused}
            </p>
          )}
          {st.state === "before" && (
            <p className={box} data-testid="queue-before">
              {fill(labels.queueBefore, { time: at(st.prequeueOpensAt) })}
            </p>
          )}
          {st.state === "notopen" && (
            <div className={`${box} space-y-2`} data-testid="queue-not-open">
              <p>{fill(labels.queueNotOpen, { time: at(st.opensAt) })}</p>
              <p className="text-2xl font-bold" role="timer" data-testid="queue-countdown">
                {clock(Date.parse(st.opensAt) - serverNow)}
              </p>
            </div>
          )}
          {st.state === "prequeue" && (
            <div className={`${box} space-y-2`} data-testid="queue-prequeue">
              <p>{fill(labels.queuePrequeue, { time: at(st.opensAt) })}</p>
              <p className="text-2xl font-bold" role="timer" data-testid="queue-countdown">
                {fill(labels.queueDrawIn, { time: clock(Date.parse(st.opensAt) - serverNow) })}
              </p>
            </div>
          )}
          {st.state === "shuffling" && (
            <p role="status" className={`${box} font-semibold`} data-testid="queue-shuffling">
              {labels.queueShuffling}
            </p>
          )}
          {st.state === "waiting" && (
            <div className={`${box} space-y-1`} data-testid="queue-waiting">
              <p className="text-2xl font-bold" data-testid="queue-ahead" data-ahead={st.ahead ?? 0}>
                {aheadText(st.ahead ?? 0, locale, labels)}
              </p>
              <p data-testid="queue-estimate">
                {est !== null && est >= 60 ? fill(labels.queueEstimate, { time: fill(labels.queueMinutes, { n: Math.ceil(est / 60) }) }) : labels.queueEstimateSoon}
              </p>
            </div>
          )}
          {st.state === "admitted" && (
            <div className={`${box} space-y-2`} data-testid="queue-admitted">
              <p className="text-xl font-bold">{labels.queueAdmitted}</p>
              {st.passExpiresAt && <p>{fill(labels.queueShoppingTime, { time: clock(st.passExpiresAt - serverNow) })}</p>}
              <a href={target?.next ?? base} className={primary} data-testid="queue-continue">
                {labels.queueToShop}
              </a>
            </div>
          )}
          {st.state === "ended" && (
            <div className={`${box} space-y-2`} data-testid="queue-ended">
              <p className="font-semibold">{labels.queueEnded}</p>
              <button type="button" className={primary} onClick={() => setAttempt((a) => a + 1)} data-testid="queue-join-again">
                {labels.queueJoinAgain}
              </button>
            </div>
          )}
          {st.state !== "admitted" && st.state !== "ended" && st.state !== "before" && (
            <p className="text-sm" data-testid="queue-keep-open">
              {labels.queueKeepOpen}
            </p>
          )}
          {placeCode(queueToken(st.saleId)) && st.state !== "before" && st.state !== "notopen" && (
            <p className="text-xs" data-testid="queue-place" data-place={placeCode(queueToken(st.saleId))}>
              {fill(labels.queuePlace, { code: placeCode(queueToken(st.saleId)) })}
            </p>
          )}
        </div>
      )}
      <a href={base} className="text-sm font-semibold underline">
        {labels.backToShop}
      </a>
    </div>
  );
}
