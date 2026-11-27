import Link from "next/link";
import { getLocale, getTranslations } from "next-intl/server";
import { listEvents } from "@/lib/catalogue/domain";
import { localDate } from "@/lib/catalogue/time";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { lt } from "@/lib/studio/text";
import { Card } from "@/components/ui/field";

export const metadata = { title: "Events" };

export default async function EventsPage() {
  const s = await requireStudio();
  const t = await getTranslations("events");
  const locale = await getLocale();
  const events = await studioTx(s, (c) => listEvents(c));
  return (
    <section className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h1 className="text-2xl font-bold">{t("title")}</h1>
        <Link href="/studio/events/new" className="rounded-lg bg-primary px-3 py-1.5 text-sm text-primary-foreground" data-testid="new-event">
          {t("new")}
        </Link>
      </div>
      {events.length === 0 && <p className="text-muted-foreground">{t("none")}</p>}
      <ul className="space-y-3" data-testid="event-list">
        {events.map((e) => (
          <li key={e.id}>
            <Card className="space-y-1">
              <Link href={`/studio/events/${e.id}`} className="font-semibold break-words hover:underline">
                {lt(e.name, locale) || e.slug}
              </Link>
              <p className="text-sm text-muted-foreground">
                {e.brand_name} · {localDate(e.starts_at, e.timezone)} → {localDate(e.ends_at, e.timezone)} · {t(`status.${e.status}`)} · {t("productCount", { n: e.products })}
                {e.edition_of && ` · ${t("edition")}`}
              </p>
            </Card>
          </li>
        ))}
      </ul>
    </section>
  );
}
