import Link from "next/link";
import { getLocale, getTranslations } from "next-intl/server";
import type { EventGraph } from "@/lib/catalogue/domain";
import { lt } from "@/lib/studio/text";

export async function EventNav({ g, current }: { g: EventGraph; current: "details" | "products" | "pools" | "sales" }) {
  const t = await getTranslations("events");
  const locale = await getLocale();
  const base = `/studio/events/${g.event.id}`;
  const tabs = [
    ["details", base, t("tabs.details")],
    ["products", `${base}/products`, t("tabs.products")],
    ["pools", `${base}/pools`, t("tabs.pools")],
    ["sales", `${base}/sales`, t("tabs.sales")],
  ] as const;
  return (
    <div className="space-y-2">
      <p className="text-sm text-muted-foreground">
        <Link href="/studio/events" className="hover:underline">
          {t("title")}
        </Link>{" "}
        · {g.brand.name}
      </p>
      <h1 className="text-2xl font-bold break-words" data-testid="event-title">
        {lt(g.event.name, locale) || g.event.slug}
      </h1>
      <nav className="flex flex-wrap gap-4 border-b text-sm" aria-label={t("tabs.label")}>
        {tabs.map(([k, href, label]) => (
          <Link key={k} href={href} aria-current={current === k ? "page" : undefined} className={`-mb-px border-b-2 pb-1 ${current === k ? "border-primary font-semibold" : "border-transparent"}`}>
            {label}
          </Link>
        ))}
      </nav>
    </div>
  );
}
