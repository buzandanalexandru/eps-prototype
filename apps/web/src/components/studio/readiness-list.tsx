import Link from "next/link";
import { getTranslations } from "next-intl/server";
import type { Problem } from "@/lib/catalogue/readiness";
import { Card } from "@/components/ui/field";

/** The readiness checklist (design D1.5): every problem with a link to the field to fix. */
export async function ReadinessList({ problems }: { problems: Problem[] }) {
  const t = await getTranslations("readiness");
  return (
    <Card data-testid="readiness" data-ready={problems.length === 0}>
      <h2 className="font-semibold">{t("title")}</h2>
      {problems.length === 0 ? (
        <p className="text-sm text-emerald-700 dark:text-emerald-400" data-testid="ready">
          {t("ready")}
        </p>
      ) : (
        <>
          <p className="text-sm">{t("notReady", { n: problems.length })}</p>
          <ul className="list-disc space-y-1 pl-5 text-sm" data-testid="problems">
            {problems.map((p, i) => (
              <li key={i} data-code={p.code}>
                <Link href={p.href} className="text-primary hover:underline">
                  {t(`codes.${p.code}`, { detail: p.detail ?? "" })}
                </Link>
              </li>
            ))}
          </ul>
        </>
      )}
    </Card>
  );
}
