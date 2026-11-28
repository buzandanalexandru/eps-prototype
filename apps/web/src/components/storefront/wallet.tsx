"use client";
// The EPS Wallet island (step 4, D12, O6): balance, season credit, the code a stand types in, top-ups
// through the simulated providers (the balance rises only once the provider confirms) and the
// history. Every screen says the balance is simulated and no real money is held.
import { useCallback, useEffect, useState } from "react";
import { fill, type ShopLabels } from "@/lib/storefront/labels";
import type { Locale } from "@/lib/storefront/schema";
import type { Currency } from "@/lib/pricing";
import { money, shopDate } from "./all-in-price";
import { messageFor } from "./checkout";
import { newKey, shopApi, type Attempt, type Problem, type Wallet } from "./shop-api";

type Props = { labels: ShopLabels; locale: Locale; base: string; organiserId: string; currency: string };

const btn = "inline-flex min-h-11 items-center justify-center rounded-[var(--eps-radius)] px-5 py-2 text-sm font-semibold disabled:opacity-50";
const primary = `${btn} bg-[var(--eps-primary)] text-[var(--eps-on-primary)]`;
const box = "rounded-[var(--eps-radius)] border border-[color-mix(in_srgb,var(--eps-on-surface)_15%,transparent)] bg-[color-mix(in_srgb,var(--eps-on-surface)_4%,var(--eps-surface))] p-4";
const input = "min-h-11 rounded-[var(--eps-radius)] border border-[color-mix(in_srgb,var(--eps-on-surface)_30%,transparent)] bg-[var(--eps-surface)] px-3 py-2";

export function WalletNotice({ labels }: { labels: ShopLabels }) {
  return (
    <p role="note" className="rounded-[var(--eps-radius)] bg-[var(--eps-accent)] px-3 py-2 text-sm font-semibold text-[var(--eps-on-accent,var(--eps-on-surface))]" data-testid="wallet-notice">
      {labels.walletNotice}
    </p>
  );
}

type State = { state: "loading" } | { state: "signin" } | { state: "ready"; wallet: Wallet } | { state: "error"; problem: Problem };

export function WalletView({ labels, locale, organiserId, currency }: Props) {
  const cur = currency as Currency;
  const [st, setSt] = useState<State>({ state: "loading" });
  const [amount, setAmount] = useState("100");
  const [method, setMethod] = useState<"hosted_card" | "apple_pay" | "google_pay">("hosted_card");
  const [busy, setBusy] = useState(false);
  const [topup, setTopup] = useState<{ attempt: Attempt } | null>(null);
  const [error, setError] = useState<Problem | null>(null);

  const load = useCallback(async () => {
    const r = await shopApi<Wallet>(`wallets/${organiserId}?currency=${cur}`);
    if (r.ok) setSt({ state: "ready", wallet: r.data });
    else if (r.status === 403 || r.status === 401) setSt({ state: "signin" });
    else setSt({ state: "error", problem: r.problem });
  }, [organiserId, cur]);

  useEffect(() => {
    const id = new URLSearchParams(window.location.search).get("topup");
    (async () => {
      await load();
      if (id && /^[0-9a-f-]{36}$/.test(id)) {
        // Back from the provider: wait for its confirmation.
        for (let i = 0; i < 30; i++) {
          const r = await shopApi<{ attempt: Attempt; wallet?: Wallet }>(`wallets/${organiserId}/topups/${id}`);
          if (!r.ok) break;
          setTopup({ attempt: r.data.attempt });
          if (r.data.wallet) setSt({ state: "ready", wallet: r.data.wallet });
          if (!["created", "submitted", "challenge_required", "unknown"].includes(r.data.attempt.state)) break;
          await new Promise((res) => setTimeout(res, 1000));
        }
      }
    })();
  }, [load, organiserId]);

  async function topUp(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError(null);
    const minor = Math.round(Number(amount.replace(",", ".")) * 100);
    const back = `${window.location.origin}${window.location.pathname}`;
    const r = await shopApi<{ attempt: Attempt; actionUrl?: string }>(`wallets/${organiserId}/topups`, {
      method: "POST",
      key: newKey(),
      body: { amountMinor: Number.isFinite(minor) ? minor : 0, currency: cur, method, locale, returnUrl: back },
    });
    if (!r.ok) {
      setError(r.problem);
      setBusy(false);
      return;
    }
    if (r.data.actionUrl) {
      // The provider sends the fan back here; the page then waits for the confirmation.
      const u = new URL(r.data.actionUrl);
      sessionStorage.setItem("eps-topup", r.data.attempt.id);
      window.location.assign(u.toString());
      return;
    }
    setBusy(false);
  }

  // The provider returns to the plain page: pick the pending top-up up from this tab.
  useEffect(() => {
    let id: string | null = null;
    try {
      id = sessionStorage.getItem("eps-topup");
      sessionStorage.removeItem("eps-topup");
    } catch {}
    if (id && !new URLSearchParams(window.location.search).get("topup")) {
      window.history.replaceState(null, "", `${window.location.pathname}?topup=${id}`);
      window.location.reload();
    }
  }, []);

  return (
    <div className="mx-auto max-w-3xl space-y-4 px-4 py-8" data-testid="wallet" data-state={st.state}>
      <h1 className="font-[family-name:var(--eps-font-heading)] text-3xl font-bold">{labels.walletTitle}</h1>
      <WalletNotice labels={labels} />
      {st.state === "signin" && (
        <div className={box} data-testid="wallet-signin">
          <p className="font-semibold">{labels.walletSignIn}</p>
          <a className={`${primary} mt-2`} href={`/sign-in?next=${encodeURIComponent(typeof window === "undefined" ? "/" : window.location.pathname)}`}>
            {labels.signIn}
          </a>
        </div>
      )}
      {st.state === "error" && <p role="alert">{messageFor(labels, st.problem)}</p>}
      {st.state === "ready" && (
        <>
          <div className={box}>
            <p className="text-sm">{labels.balance}</p>
            <p className="text-3xl font-bold" data-testid="wallet-balance">
              {money(st.wallet.balanceMinor, cur, locale)}
            </p>
            {st.wallet.creditMinor > 0 && (
              <p className="text-sm" data-testid="wallet-credit">
                {fill(labels.seasonCredit, { date: st.wallet.creditExpiresAt ? shopDate(st.wallet.creditExpiresAt, locale, "Europe/Bucharest") : "" })}: {money(st.wallet.creditMinor, cur, locale)}
              </p>
            )}
            {st.wallet.code && (
              <p className="mt-2 text-sm">
                {labels.walletCode}: <code className="font-bold" data-testid="wallet-code">{st.wallet.code}</code>
              </p>
            )}
            <PayCode labels={labels} organiserId={organiserId} currency={cur} />
          </div>
          {topup && (
            <p role="status" data-testid="topup-status" data-state={topup.attempt.state}>
              {topup.attempt.state === "succeeded" ? labels.topupDone : topup.attempt.state === "declined" ? `${labels.topupDeclined} ${labels.errorCode}: ${topup.attempt.errorCode ?? ""}` : labels.topupWaiting}
            </p>
          )}
          <form className={`${box} space-y-3`} onSubmit={topUp} data-testid="topup-form">
            <p className="font-semibold">{labels.topUp}</p>
            <label className="grid gap-1 text-sm">
              {labels.amount} ({cur})
              <input className={input} inputMode="decimal" value={amount} onChange={(e) => setAmount(e.target.value)} data-testid="topup-amount" />
            </label>
            <fieldset className="space-y-1">
              <legend className="text-sm font-semibold">{labels.method}</legend>
              {(
                [
                  ["hosted_card", labels.methodCard],
                  ["apple_pay", labels.methodApple],
                  ["google_pay", labels.methodGoogle],
                ] as const
              ).map(([m, l]) => (
                <label key={m} className="flex min-h-11 items-center gap-2">
                  <input type="radio" name="topup-method" checked={method === m} onChange={() => setMethod(m)} data-testid={`topup-method-${m}`} /> {l}
                </label>
              ))}
            </fieldset>
            {error && (
              <p role="alert" data-testid="topup-error">
                {messageFor(labels, error)} {error.code.startsWith("EPS-") ? `${labels.errorCode}: ${error.code}` : ""}
              </p>
            )}
            <button className={primary} disabled={busy} data-testid="topup-submit">
              {busy ? labels.sending : labels.topUp}
            </button>
          </form>
          <div className={box}>
            <p className="font-semibold">{labels.history}</p>
            {st.wallet.history.length === 0 ? (
              <p className="text-sm">{labels.noHistory}</p>
            ) : (
              <ul className="mt-2 space-y-1 text-sm" data-testid="wallet-history">
                {st.wallet.history.map((m, i) => (
                  <li key={i} className="flex flex-wrap justify-between gap-2" data-testid="wallet-move" data-kind={m.kind}>
                    <span>
                      {labels[`move_${m.kind}` as keyof ShopLabels] as string} · {shopDate(m.at, locale, "Europe/Bucharest")}
                    </span>
                    <span>{money(m.amountMinor, cur, locale)}</span>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </>
      )}
    </div>
  );
}

/** A 6-digit code for one stand payment, valid 2 minutes (review D-d). */
function PayCode({ labels, organiserId, currency }: { labels: ShopLabels; organiserId: string; currency: string }) {
  const [code, setCode] = useState<{ payCode: string; expiresAt: string } | null>(null);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!code) return;
    const t = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(t);
  }, [code]);
  const left = code ? Math.max(0, Math.floor((Date.parse(code.expiresAt) - now) / 1000)) : 0;
  return (
    <div className="mt-2 space-y-1 text-sm">
      <button
        type="button"
        className="font-semibold underline"
        data-testid="pay-code-show"
        onClick={async () => {
          const r = await shopApi<{ payCode: string; expiresAt: string }>(`wallets/${organiserId}/pay-code?currency=${currency}`, { method: "POST" });
          if (r.ok) {
            setCode(r.data);
            setNow(Date.now());
          }
        }}
      >
        {labels.payCodeShow}
      </button>
      {code &&
        (left > 0 ? (
          <p>
            {labels.payCode}: <code className="text-lg font-bold" data-testid="pay-code">{code.payCode}</code> · {Math.floor(left / 60)}:{String(left % 60).padStart(2, "0")}
          </p>
        ) : (
          <p data-testid="pay-code-expired">{labels.payCodeExpired}</p>
        ))}
    </div>
  );
}
