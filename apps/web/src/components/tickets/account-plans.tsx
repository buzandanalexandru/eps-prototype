"use client";
// The fan account's instalment schedules (F8, Thin): every payment with its due date and status, and
// Pay now for the next or overdue payment, through the step 4 payment flow with the plan's saved
// card (simulated provider). The page reloads the plan from the core afterwards.
import { useState } from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { shopApi, type OrderView } from "@/components/storefront/shop-api";
import { formatMoney, type Currency } from "@/lib/pricing";
import { fmtDay } from "@/lib/trip/format";

export function AccountPlans({ orders, locale }: { orders: OrderView[]; locale: "ro" | "en" }) {
  const t = useTranslations("account");
  const router = useRouter();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const withPlans = orders.filter((o) => o.plan && o.plan.state !== "pending");
  if (withPlans.length === 0) return <p className="text-sm text-muted-foreground">{t("noPlans")}</p>;
  const pay = async (o: OrderView, what: "pay-overdue" | "simulate-next") => {
    setBusy(o.id);
    setError(null);
    const r = await shopApi<OrderView>(`orders/${o.id}/instalments/${what}`, { method: "POST", body: {} });
    if (!r.ok) setError(r.problem.code ?? "EPS-UNAVAILABLE");
    // The provider confirms by webhook: give it a moment, then read the plan again.
    window.setTimeout(() => {
      setBusy(null);
      router.refresh();
    }, 2500);
  };
  return (
    <div className="space-y-4">
      {error && (
        <p role="alert" data-testid="plan-error">
          {error}
        </p>
      )}
      {withPlans.map((o) => {
        const plan = o.plan!;
        const money = (m: number) => formatMoney(m, o.currency as Currency, locale);
        const overdue = plan.parts.find((p) => p.state === "overdue");
        const next = plan.parts.find((p) => p.state === "scheduled");
        const paid = plan.parts.filter((p) => p.state === "paid").length;
        return (
          <div key={o.id} className="space-y-2 rounded-xl border p-3" data-testid="plan" data-order-id={o.id} data-plan-state={plan.state} data-paid={paid}>
            <p className="font-semibold break-words">{(o.eventName as { ro?: string; en?: string } | undefined)?.[locale] ?? o.id.slice(0, 8)}</p>
            <p className="text-sm" data-testid="plan-paid-count">
              {t("paidCount", { paid, total: plan.parts.length })}
            </p>
            {overdue && (
              <p className="text-sm font-semibold text-destructive" data-testid="plan-overdue">
                {t("planOverdue", { amount: money(overdue.amountMinor) })}
              </p>
            )}
            <ul className="space-y-1 text-sm">
              {plan.parts.map((p) => (
                <li key={p.n} className="flex flex-wrap justify-between gap-2" data-testid="plan-part" data-state={p.state}>
                  <span>
                    {t("part", { n: p.n })} · {money(p.amountMinor)} · {t("partDue", { date: fmtDay(p.dueOn, locale) })}
                  </span>
                  <span className="font-semibold">
                    {t(`part_${p.state}` as "part_paid")}
                    {p.state === "overdue" && p.graceUntil ? ` · ${t("partGrace", { date: fmtDay(p.graceUntil, locale) })}` : ""}
                  </span>
                </li>
              ))}
            </ul>
            {plan.state === "active" && (overdue || next) && (
              <div className="space-y-1">
                <button
                  type="button"
                  disabled={busy !== null}
                  onClick={() => void pay(o, overdue ? "pay-overdue" : "simulate-next")}
                  className="rounded-md bg-foreground px-3 py-2 text-sm font-semibold text-background disabled:opacity-50"
                  data-testid="plan-pay-now"
                >
                  {busy === o.id ? t("paying") : t("payNow")}
                </button>
                <p className="text-xs text-muted-foreground">{t("payNowNote")}</p>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
