// The checkout questions editor (step 4, D9 Thin): up to 5 questions and consents in the locked
// checkout's two slots, reaching the shop with the brand's next publish.
import { notFound } from "next/navigation";
import { getTranslations } from "next-intl/server";
import { listQuestions, MAX_QUESTIONS } from "@/lib/checkout/questions";
import { getBrand } from "@/lib/organisers/domain";
import { requireStudio, studioTx } from "@/lib/studio/context";
import { Card } from "@/components/ui/field";
import { Button } from "@/components/ui/button";
import { addQuestionAction, removeQuestionAction } from "./actions";

export const metadata = { title: "Checkout questions" };
export const dynamic = "force-dynamic";

const field = "w-full rounded-md border bg-background px-2 py-1";

export default async function QuestionsPage({ params, searchParams }: { params: Promise<{ id: string }>; searchParams: Promise<Record<string, string | undefined>> }) {
  const { id } = await params;
  const sp = await searchParams;
  const s = await requireStudio();
  const t = await getTranslations("payments");
  const { brand, questions } = await studioTx(s, async (c) => {
    const brand = await getBrand(c, id);
    return { brand, questions: brand ? await listQuestions(c, id) : [] };
  });
  if (!brand) notFound();
  return (
    <section className="max-w-2xl space-y-6">
      <h1 className="text-2xl font-bold">{t("questionsTitle")}</h1>
      <p className="text-sm text-muted-foreground">
        {brand.name} · {t("questionsLead")}
      </p>
      {sp.added && (
        <p role="status" className="text-sm font-semibold" data-testid="question-added">
          {t("added")}
        </p>
      )}
      {sp.error && (
        <p role="alert" className="text-sm font-semibold text-destructive">
          {t("error", { code: sp.error })}
        </p>
      )}
      <Card>
        {questions.length === 0 ? (
          <p className="text-sm">{t("none")}</p>
        ) : (
          <ul className="space-y-2" data-testid="question-list">
            {questions.map((q) => (
              <li key={q.id} className="flex flex-wrap items-center justify-between gap-2 text-sm" data-testid="question-row">
                <span>
                  {q.label.ro} / {q.label.en} · {t(`kind_${q.kind}`)}
                  {q.required ? ` · ${t("required")}` : ""}
                </span>
                <form action={removeQuestionAction.bind(null, brand.id, q.id)}>
                  <Button type="submit" size="sm" variant="outline">
                    {t("remove")}
                  </Button>
                </form>
              </li>
            ))}
          </ul>
        )}
      </Card>
      {questions.length < MAX_QUESTIONS && (
        <Card>
          <form action={addQuestionAction.bind(null, brand.id)} className="space-y-3" data-testid="question-form">
            <label className="grid gap-1 text-sm">
              {t("kind")}
              <select name="kind" className={field} defaultValue="consent">
                <option value="text">{t("kind_text")}</option>
                <option value="choice">{t("kind_choice")}</option>
                <option value="consent">{t("kind_consent")}</option>
              </select>
            </label>
            <label className="grid gap-1 text-sm">
              {t("labelRo")}
              <input name="label_ro" required maxLength={300} className={field} />
            </label>
            <label className="grid gap-1 text-sm">
              {t("labelEn")}
              <input name="label_en" required maxLength={300} className={field} />
            </label>
            <label className="grid gap-1 text-sm">
              {t("options")}
              <textarea name="options" rows={3} className={field} />
            </label>
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" name="required" /> {t("required")}
            </label>
            <Button type="submit">{t("add")}</Button>
          </form>
        </Card>
      )}
    </section>
  );
}
