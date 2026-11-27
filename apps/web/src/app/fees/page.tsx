import { getLocale, getTranslations } from "next-intl/server";
import { FeeExample, percent } from "@/components/fee-example";
import { publishedFee } from "@/lib/fees/published";
import { FEE_POLICIES, formatMoney } from "@/lib/pricing";

export const dynamic = "force-dynamic";

export async function generateMetadata() {
  const t = await getTranslations("fees");
  return { title: t("title") };
}

export default async function FeesPage() {
  const t = await getTranslations("fees");
  const locale = await getLocale();
  const { fee } = await publishedFee();
  return (
    <article className="max-w-2xl space-y-6">
      <h1 className="text-3xl font-bold">{t("title")}</h1>
      <p className="text-muted-foreground">{t("lead")}</p>
      <section className="rounded-lg border p-4" data-testid="current-fee">
        <h2 className="font-semibold">{t("current")}</h2>
        <p className="text-2xl font-bold" data-testid="fee-percent">
          {percent(fee.percentBps, locale)}
        </p>
        <p className="text-sm" data-testid="fee-fixed">
          {fee.fixedMinor ? t("fixed", { amount: formatMoney(fee.fixedMinor, "RON", locale) }) : t("noFixed")}
        </p>
        <p className="mt-2 text-sm text-muted-foreground">{t("perTicket")}</p>
      </section>
      <section className="space-y-2">
        <h2 className="text-xl font-semibold">{t("policiesTitle")}</h2>
        <ul className="space-y-2">
          {FEE_POLICIES.map((p) => (
            <li key={p} data-testid={`policy-${p}`}>
              <span className="font-medium">{t(`policies.${p}.name`)}</span>: {t(`policies.${p}.lead`)}
            </li>
          ))}
        </ul>
      </section>
      <section className="space-y-2">
        <h2 className="text-xl font-semibold">{t("exampleTitle")}</h2>
        <p className="text-sm text-muted-foreground">{t("exampleLead", { price: formatMoney(53900, "RON", locale) })}</p>
        <FeeExample fee={fee} />
      </section>
      <p className="text-sm text-muted-foreground">{t("allIn")}</p>
    </article>
  );
}
