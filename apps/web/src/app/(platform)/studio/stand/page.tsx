// The stand screen (step 4, D12, simulated): a stand charges a fan's EPS Wallet by the code the fan
// shows on their wallet page; the ledger records a stand sale.
import { randomUUID } from "node:crypto";
import { getTranslations } from "next-intl/server";
import { formatMoney, type Currency } from "@/lib/pricing";
import { requireStudio } from "@/lib/studio/context";
import { Card } from "@/components/ui/field";
import { Button } from "@/components/ui/button";
import { standPaymentAction } from "../payments/actions";

export const metadata = { title: "Stand" };
export const dynamic = "force-dynamic";

const field = "w-full rounded-md border bg-background px-2 py-1";

export default async function StandPage({ searchParams }: { searchParams: Promise<Record<string, string | undefined>> }) {
  await requireStudio();
  const t = await getTranslations("payments");
  const sp = await searchParams;
  const cur = (sp.currency === "EUR" ? "EUR" : "RON") as Currency;
  return (
    <section className="max-w-md space-y-4">
      <h1 className="text-2xl font-bold">{t("standTitle")}</h1>
      <p className="text-sm text-muted-foreground">{t("standLead")}</p>
      {sp.charged && (
        <p role="status" className="font-semibold" data-testid="stand-charged">
          {t("charged", { left: formatMoney(Number(sp.left ?? 0), cur, "ro") })}
        </p>
      )}
      {sp.error && (
        <p role="alert" className="font-semibold text-destructive" data-testid="stand-error">
          {t("error", { code: sp.error })}
        </p>
      )}
      <Card>
        <form action={standPaymentAction} className="space-y-3" data-testid="stand-form">
          <label className="grid gap-1 text-sm">
            {t("code")}
            <input name="code" required pattern="[A-Za-z0-9]{4}-[A-Za-z0-9]{4}" className={field} autoComplete="off" data-testid="stand-code" />
          </label>
          <label className="grid gap-1 text-sm">
            {t("payCode")}
            <input name="payCode" required pattern="[0-9]{6}" inputMode="numeric" maxLength={6} className={field} autoComplete="off" data-testid="stand-pay-code" />
          </label>
          <input type="hidden" name="key" value={randomUUID()} />
          <label className="grid gap-1 text-sm">
            {t("amount")} (RON)
            <input name="amount" required inputMode="decimal" className={field} data-testid="stand-amount" />
          </label>
          <label className="grid gap-1 text-sm">
            {t("label")}
            <input name="label" maxLength={80} className={field} />
          </label>
          <Button type="submit">{t("charge")}</Button>
        </form>
      </Card>
    </section>
  );
}
