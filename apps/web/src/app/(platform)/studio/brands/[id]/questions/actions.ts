"use server";
// The checkout questions editor's actions (step 4, D9 Thin).
import { redirect } from "next/navigation";
import { addQuestion, removeQuestion } from "@/lib/checkout/questions";
import { getBrand } from "@/lib/organisers/domain";
import { DomainError } from "@/lib/errors";
import { requireActive, studioTx } from "@/lib/studio/context";

export async function addQuestionAction(brandId: string, fd: FormData) {
  const s = await requireActive();
  const kind = String(fd.get("kind") ?? "text");
  const options = String(fd.get("options") ?? "")
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean)
    .map((l) => {
      const [ro, en] = l.split("|").map((x) => x.trim());
      return { ro, en: en || ro };
    });
  let error = "";
  try {
    await studioTx(s, async (c) => {
      if (!(await getBrand(c, brandId))) throw new DomainError("not_found");
      await addQuestion(c, s.active.id, brandId, s.user.id, {
        kind,
        label: { ro: String(fd.get("label_ro") ?? ""), en: String(fd.get("label_en") ?? "") },
        options: kind === "choice" ? options : [],
        required: fd.get("required") === "on",
      });
    });
  } catch (e) {
    error = e instanceof DomainError ? e.code : "invalid";
  }
  redirect(`/studio/brands/${brandId}/questions?${error ? `error=${error}` : "added=1"}`);
}

export async function removeQuestionAction(brandId: string, id: string) {
  const s = await requireActive();
  await studioTx(s, async (c) => {
    if (!(await getBrand(c, brandId))) throw new DomainError("not_found");
    await removeQuestion(c, brandId, id);
  });
  redirect(`/studio/brands/${brandId}/questions`);
}
