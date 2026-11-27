import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { Button } from "@/components/ui/button";

export default async function Home() {
  const t = await getTranslations("home");
  return (
    <section className="space-y-6">
      <h1 className="text-3xl font-bold">{t("title")}</h1>
      <p className="max-w-prose text-muted-foreground">{t("lead")}</p>
      <div className="flex flex-wrap gap-3">
        <Button asChild>
          <Link href="/studio">{t("openStudio")}</Link>
        </Button>
        <Button asChild variant="outline">
          <Link href="/status">{t("viewStatus")}</Link>
        </Button>
      </div>
    </section>
  );
}
