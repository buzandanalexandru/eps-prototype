// The platform fee and fee policies (design D1.3, A6). One calculation for the whole platform:
// Studio, the fees page, the seed and (step 3) the snapshot compiler all call feeBreakdown, so a
// price can never disagree between screens. Computed per unit.
import { divRoundHalfUp, isMinor } from "./money";

export type FeePolicy = "absorb" | "pass_on" | "split";
export const FEE_POLICIES: readonly FeePolicy[] = ["absorb", "pass_on", "split"];
export const DEFAULT_FEE_POLICY: FeePolicy = "pass_on";

export type PlatformFee = { percentBps: number; fixedMinor: number };

export type FeeBreakdown = {
  faceMinor: number;
  /** The whole platform fee for one unit. */
  feeMinor: number;
  /** The part of the fee the fan pays on top of the face price. */
  fanFeeMinor: number;
  /** What the fan pays: the all-in price, the only fan-facing number. */
  allInMinor: number;
  /** What the organiser receives. */
  organiserNetMinor: number;
};

export function isFeePolicy(v: unknown): v is FeePolicy {
  return v === "absorb" || v === "pass_on" || v === "split";
}

/** The fee for one unit: round_half_up(face × bps / 10000) + fixed; a free unit pays no fee. */
export function platformFeeFor(faceMinor: number, fee: PlatformFee): number {
  if (!isMinor(faceMinor) || faceMinor < 0) throw new Error("feeBreakdown: face price must be a non-negative integer");
  if (faceMinor === 0) return 0;
  return divRoundHalfUp(faceMinor * fee.percentBps, 10_000) + fee.fixedMinor;
}

/**
 * absorb: fan pays face, organiser gets face − fee. pass on: fan pays face + fee, organiser gets
 * face. split: fan pays face + floor(fee / 2), organiser gets face − ceil(fee / 2). Fan and
 * organiser amounts always differ by exactly the fee.
 */
export function feeBreakdown(faceMinor: number, policy: FeePolicy, fee: PlatformFee): FeeBreakdown {
  const feeMinor = platformFeeFor(faceMinor, fee);
  const fanFeeMinor = policy === "pass_on" ? feeMinor : policy === "split" ? Math.floor(feeMinor / 2) : 0;
  const allInMinor = faceMinor + fanFeeMinor;
  return { faceMinor, feeMinor, fanFeeMinor, allInMinor, organiserNetMinor: allInMinor - feeMinor };
}

/** The policy in force for an event: its override, else the organiser's, else pass on. */
export function effectivePolicy(organiserPolicy: FeePolicy | null | undefined, eventOverride: FeePolicy | null | undefined): FeePolicy {
  return eventOverride ?? organiserPolicy ?? DEFAULT_FEE_POLICY;
}
