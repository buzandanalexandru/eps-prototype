import { getTranslations } from "next-intl/server";

export async function generateMetadata() {
  const t = await getTranslations("terms");
  return { title: t("title") };
}

/** Platform terms placeholder linked from every storefront footer (prototype: no real sales). */
export default async function TermsPage() {
  const t = await getTranslations("terms");
  return (
    <section className="max-w-2xl space-y-3">
      <h1 className="text-2xl font-bold">{t("title")}</h1>
      <p>{t("lead")}</p>
      <p data-testid="terms-privacy">{t("privacy")}</p>
      <p className="text-sm text-muted-foreground">{t("prototype")}</p>
    </section>
  );
}
