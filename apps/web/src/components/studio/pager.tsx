import Link from "next/link";
import { getTranslations } from "next-intl/server";

export function pageNumber(raw: string | undefined): number {
  const n = Number(raw);
  return Number.isInteger(n) && n > 0 && n < 10_000 ? n - 1 : 0;
}

export async function Pager({ path, page, more }: { path: string; page: number; more: boolean }) {
  const t = await getTranslations("studio.common");
  if (page === 0 && !more) return null;
  return (
    <nav className="flex gap-4 text-sm" aria-label={t("pages")}>
      {page > 0 && <Link href={`${path}?page=${page}`}>{t("previous")}</Link>}
      {more && <Link href={`${path}?page=${page + 2}`}>{t("next")}</Link>}
    </nav>
  );
}
