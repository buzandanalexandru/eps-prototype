import { getFormatter, getTranslations } from "next-intl/server";
import type { PublicationState } from "@/lib/snapshot/publish";

/** An event's live sale snapshot and whether the catalogue has unpublished changes (design D3.1). */
export async function PublicationStateCard({ state }: { state: PublicationState }) {
  const t = await getTranslations("publication");
  const f = await getFormatter();
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border p-3 text-sm" data-testid="publication-state" data-live={state.liveVersion ?? ""} data-changed={state.changed}>
      {state.liveVersion === null ? (
        <span>{t("notPublished")}</span>
      ) : (
        <span data-testid="live-snapshot">
          {t("live", { version: state.liveVersion, date: state.publishedAt ? f.dateTime(new Date(state.publishedAt), { dateStyle: "medium", timeStyle: "short" }) : "" })}
        </span>
      )}
      {state.changed && (
        <span className="rounded-full bg-amber-100 px-2 py-0.5 text-xs font-semibold text-amber-900" data-testid="unpublished-changes">
          {t("changed")}
        </span>
      )}
      {state.liveVersion !== null && <span className="w-full text-xs text-muted-foreground">{t("lead")}</span>}
    </div>
  );
}
