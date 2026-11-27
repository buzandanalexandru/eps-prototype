import Link from "next/link";
import { getLocale, getTranslations } from "next-intl/server";
import { listBrands } from "@/lib/organisers/domain";
import { listEvents, loadEventGraph } from "@/lib/catalogue/domain";
import { eventReadiness } from "@/lib/catalogue/readiness";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { lt } from "@/lib/studio/text";
import { Card } from "@/components/ui/field";

export const metadata = { title: "Studio" };

export default async function StudioPage() {
  const s = await requireStudio();
  const t = await getTranslations("studio");
  const locale = await getLocale();
  const { brands, events } = await studioTx(s, async (c) => {
    const brands = await listBrands(c);
    const evs = await listEvents(c);
    const events = [];
    for (const e of evs) events.push({ ...e, problems: eventReadiness((await loadEventGraph(c, e.id))!).length });
    return { brands, events };
  });
  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-bold">{t("title")}</h1>
      <Card>
        <h2 className="font-semibold">{t("nav.brands")}</h2>
        <ul className="space-y-1" data-testid="overview-brands">
          {brands.map((b) => (
            <li key={b.id} className="flex flex-wrap gap-2">
              <Link className="font-medium hover:underline" href={`/studio/brands/${b.id}`}>
                {b.name}
              </Link>
              <span className="text-sm text-muted-foreground">/s/{b.slug}</span>
            </li>
          ))}
        </ul>
      </Card>
      <Card>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 className="font-semibold">{t("nav.events")}</h2>
          <Link href="/studio/events/new" className="text-sm text-primary hover:underline">
            {t("events.new")}
          </Link>
        </div>
        {events.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t("events.none")}</p>
        ) : (
          <ul className="divide-y" data-testid="overview-events">
            {events.map((e) => (
              <li key={e.id} className="flex flex-wrap items-center justify-between gap-2 py-2">
                <Link className="min-w-0 font-medium break-words hover:underline" href={`/studio/events/${e.id}`}>
                  {lt(e.name, locale) || e.slug}
                </Link>
                <span className="text-sm text-muted-foreground">
                  {e.brand_name} · {e.problems === 0 ? t("readiness.ready") : t("readiness.count", { n: e.problems })}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </section>
  );
}
