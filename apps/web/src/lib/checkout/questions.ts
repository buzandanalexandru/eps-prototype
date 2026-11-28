// Checkout questions (step 4, D9 Thin, design D-13): an organiser's questions and consents for a
// brand's locked checkout. A question is stored once and never changed (control.checkout_questions);
// its id goes into the storefront draft's checkout slots (afterContact for text and single choice,
// consents for checkboxes), so it reaches fans with the brand's next publish and every snapshot
// asks exactly the questions it was published with. At most 5 per checkout.
import { randomUUID } from "node:crypto";
import type { ClientBase } from "pg";
import * as z from "zod";
import { DomainError } from "../errors";
import { getStorefrontByBrand, ensureStorefront, saveDraft } from "../storefront/domain";

export const MAX_QUESTIONS = 5;

const l10n = z.object({ ro: z.string().trim().min(1).max(300), en: z.string().trim().min(1).max(300) });

export const questionInput = z
  .object({
    kind: z.enum(["text", "choice", "consent"]),
    label: l10n,
    options: z.array(l10n).max(10).default([]),
    required: z.boolean().default(false),
  })
  .refine((q) => (q.kind === "choice" ? q.options.length >= 2 : q.options.length === 0), { message: "options", path: ["options"] });

export type QuestionInput = z.infer<typeof questionInput>;

export type QuestionRow = { id: string; kind: "text" | "choice" | "consent"; label: { ro: string; en: string }; options: { ro: string; en: string }[]; required: boolean; slot: "afterContact" | "consents"; inDraft: boolean };

/** The brand's questions, in the draft's slot order (questions no longer in the draft are left out). */
export async function listQuestions(c: ClientBase, brandId: string): Promise<QuestionRow[]> {
  const sf = await getStorefrontByBrand(c, brandId);
  const slots = sf?.document.checkout.slots ?? { afterContact: [], consents: [] };
  const ids = [...slots.afterContact, ...slots.consents];
  if (!ids.length) return [];
  const rows = (
    await c.query<Omit<QuestionRow, "slot" | "inDraft">>("select id, kind, label, options, required from control.checkout_questions where brand_id = $1 and id = any($2)", [brandId, ids])
  ).rows;
  const byId = new Map(rows.map((r) => [r.id, r]));
  return ids.flatMap((id) => {
    const r = byId.get(id);
    return r ? [{ ...r, slot: slots.consents.includes(id) ? ("consents" as const) : ("afterContact" as const), inDraft: true }] : [];
  });
}

/** Adds a question to the brand's checkout draft (published with the brand's next publish). */
export async function addQuestion(c: ClientBase, organiserId: string, brandId: string, userId: string | null, input: unknown): Promise<string> {
  const q = questionInput.parse(input);
  const sf = await ensureStorefront(c, brandId, userId);
  const slots = sf.document.checkout.slots;
  if (slots.afterContact.length + slots.consents.length >= MAX_QUESTIONS) throw new DomainError("too_many_questions");
  const id = `q-${randomUUID().slice(0, 8)}`;
  await c.query("insert into control.checkout_questions (organiser_id, brand_id, id, kind, label, options, required) values ($1, $2, $3, $4, $5, $6, $7)", [
    organiserId,
    brandId,
    id,
    q.kind,
    JSON.stringify(q.label),
    JSON.stringify(q.options),
    q.required,
  ]);
  const doc = structuredClone(sf.document);
  if (q.kind === "consent") doc.checkout.slots.consents.push(id);
  else doc.checkout.slots.afterContact.push(id);
  await saveDraft(c, sf.id, { revision: sf.draft_revision, document: doc });
  return id;
}

/** Takes a question out of the brand's checkout draft (published snapshots keep asking it). */
export async function removeQuestion(c: ClientBase, brandId: string, id: string): Promise<void> {
  const sf = await getStorefrontByBrand(c, brandId);
  if (!sf) throw new DomainError("not_found");
  const doc = structuredClone(sf.document);
  doc.checkout.slots.afterContact = doc.checkout.slots.afterContact.filter((x) => x !== id);
  doc.checkout.slots.consents = doc.checkout.slots.consents.filter((x) => x !== id);
  await saveDraft(c, sf.id, { revision: sf.draft_revision, document: doc });
}
