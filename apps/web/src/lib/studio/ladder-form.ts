// The price-ladder editor's form → a ladder input (validated afterwards by ladderSchema).
import * as z from "zod";
import { DomainError } from "../errors";
import type { LadderInput } from "../catalogue/schema";
import { zonedToUtc } from "../catalogue/time";
import { parseMajor } from "../pricing/money";
import { formObject, indexed } from "./form";

const ladderRow = z.object({ threshold: z.string().max(40).optional(), price: z.string().max(20).optional() });
const ladderForm = z.object({ mode: z.string().max(10).optional(), steps: z.record(z.string().regex(/^\d+$/), ladderRow).optional() });

export function ladderFromForm(fd: FormData, tz: string): LadderInput {
  const o = ladderForm.parse(formObject(fd));
  const mode = o.mode === "by_sales" ? "by_sales" : "by_date";
  const rows = indexed<z.infer<typeof ladderRow>>(o.steps).map((x, i) => ({ ...x, i }));
  // The first step is the price from the start of sales: it cannot be left empty while later
  // steps have prices (that would silently promote a later step to the first one).
  if ((rows[0]?.price ?? "").trim() === "") throw new DomainError("price_required", "steps.0.price");
  const steps = rows
    .filter((x) => (x.price ?? "").trim() !== "")
    .map((x) => {
      const priceMinor = parseMajor(x.price ?? "");
      if (priceMinor === null) throw new DomainError("price_invalid", `steps.${x.i}.price`);
      const th = (x.threshold ?? "").trim();
      if (x.i === 0) return { priceMinor, fromAt: null, afterSold: null };
      if (!th) throw new DomainError("threshold_required", "steps");
      if (mode === "by_sales") return { priceMinor, fromAt: null, afterSold: th };
      const at = zonedToUtc(th, tz);
      if (!at) throw new DomainError("date_invalid", `steps.${x.i}.threshold`);
      return { priceMinor, fromAt: at, afterSold: null };
    });
  return { mode, steps } as LadderInput;
}
