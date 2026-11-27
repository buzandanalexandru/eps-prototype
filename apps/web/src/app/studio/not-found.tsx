import Link from "next/link";
import { getTranslations } from "next-intl/server";

export default async function StudioNotFound() {
  const t = await getTranslations("studio");
  return (
    <section className="space-y-3" data-testid="not-found">
      <h1 className="text-2xl font-bold">{t("notFound")}</h1>
      <p className="text-muted-foreground">{t("notFoundLead")}</p>
      <Link href="/studio" className="text-primary hover:underline">
        {t("backToStudio")}
      </Link>
    </section>
  );
}
