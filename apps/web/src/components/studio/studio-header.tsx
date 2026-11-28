import Link from "next/link";
import { getTranslations } from "next-intl/server";
import { switchOrganiserAction } from "@/app/studio/actions";
import { signOut } from "@/lib/session/actions";
import type { Studio } from "@/lib/studio/context";
import { Button } from "@/components/ui/button";

/** The "Sample data" label (sample-data capability), shown on every screen of a sample organiser. */
export async function SampleBadge() {
  const t = await getTranslations("studio");
  return (
    <span
      data-testid="sample-badge"
      className="inline-flex items-center rounded-full border border-amber-500/60 bg-amber-100 px-2 py-0.5 text-xs font-semibold text-amber-900 dark:bg-amber-900/30 dark:text-amber-200"
      title={t("sampleHint")}
    >
      {t("sample")}
    </span>
  );
}

export async function StudioHeader({ studio }: { studio: Studio }) {
  const t = await getTranslations("studio");
  const tc = await getTranslations("common");
  const active = studio.active;
  const ready = active?.onboarding_state === "done";
  const nav = [
    ["/studio", t("nav.overview")],
    ["/studio/brands", t("nav.brands")],
    ["/studio/builder", t("nav.builder")],
    ["/studio/events", t("nav.events")],
    ["/studio/venues", t("nav.venues")],
    ["/studio/team", t("nav.team")],
    ["/studio/fees", t("nav.fees")],
  ] as const;
  return (
    <div className="mb-6 space-y-3 border-b pb-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate font-semibold" data-testid="active-organiser">
            {active ? active.name : t("noOrganiser")}
          </span>
          {active?.is_sample && <SampleBadge />}
          {studio.organisers.length > 1 && (
            <form action={switchOrganiserAction} className="flex items-center gap-1" data-testid="organiser-switcher">
              <label className="sr-only" htmlFor="organiserId">
                {t("switchLabel")}
              </label>
              <select id="organiserId" name="organiserId" defaultValue={active?.id} className="max-w-44 rounded-md border bg-background px-2 py-1 text-sm">
                {studio.organisers.map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.name}
                  </option>
                ))}
              </select>
              <Button type="submit" size="sm" variant="outline">
                {t("switch")}
              </Button>
            </form>
          )}
        </div>
        <div className="flex min-w-0 items-center gap-2 text-sm">
          <span className="truncate text-muted-foreground" data-testid="studio-user">
            {t("welcome", { name: studio.user.name || studio.user.email })}
          </span>
          <form action={signOut}>
            <Button type="submit" variant="outline" size="sm">
              {tc("signOut")}
            </Button>
          </form>
        </div>
      </div>
      {ready && (
        <nav aria-label={t("nav.label")} className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
          {nav.map(([href, label]) => (
            <Link key={href} href={href} className="hover:underline">
              {label}
            </Link>
          ))}
        </nav>
      )}
    </div>
  );
}
