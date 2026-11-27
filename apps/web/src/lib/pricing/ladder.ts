// Price ladders by date or by sales (design D1.4, C5). The price in force depends only on the time
// and the quantity sold, never on the buyer.
export type LadderMode = "by_date" | "by_sales";

export type PriceStep =
  | { mode: "by_date"; fromAt: Date | null; priceMinor: number }
  | { mode: "by_sales"; afterSold: number | null; priceMinor: number };

export type LadderInput = { steps: PriceStep[]; at: Date; sold?: number };

function reached(step: PriceStep, at: Date, sold: number): boolean {
  if (step.mode === "by_date") return step.fromAt === null || step.fromAt.getTime() <= at.getTime();
  return step.afterSold === null || sold >= step.afterSold;
}

/** Index of the step in force: the last step whose threshold is reached (the first has none). */
export function activePriceStepIndex({ steps, at, sold = 0 }: LadderInput): number {
  if (steps.length === 0) throw new Error("activePriceStep: empty ladder");
  let idx = 0;
  for (let i = 1; i < steps.length; i++) if (reached(steps[i], at, sold)) idx = i;
  return idx;
}

export function activePriceStep(input: LadderInput): PriceStep {
  return input.steps[activePriceStepIndex(input)];
}

/** The next step after the one in force, or null at the top of the ladder. */
export function nextPriceStep(input: LadderInput): PriceStep | null {
  const i = activePriceStepIndex(input);
  return input.steps[i + 1] ?? null;
}

/** What is wrong with a ladder (empty, mixed modes, a threshold on the first step, not strictly increasing). */
export function ladderProblems(steps: PriceStep[]): ("ladder_empty" | "ladder_mixed" | "ladder_unordered" | "ladder_first_threshold")[] {
  if (steps.length === 0) return ["ladder_empty"];
  const out: ("ladder_mixed" | "ladder_unordered" | "ladder_first_threshold")[] = [];
  const mode = steps[0].mode;
  if (steps.some((s) => s.mode !== mode)) return ["ladder_mixed"];
  const first = steps[0];
  if ((first.mode === "by_date" && first.fromAt !== null) || (first.mode === "by_sales" && first.afterSold !== null)) {
    out.push("ladder_first_threshold");
  }
  for (let i = 1; i < steps.length; i++) {
    const a = steps[i - 1];
    const b = steps[i];
    const av = a.mode === "by_date" ? a.fromAt?.getTime() ?? -Infinity : a.afterSold ?? 0;
    const bv = b.mode === "by_date" ? b.fromAt?.getTime() : b.afterSold;
    if (bv === null || bv === undefined || !(bv > av)) {
      out.push("ladder_unordered");
      break;
    }
  }
  return out;
}
