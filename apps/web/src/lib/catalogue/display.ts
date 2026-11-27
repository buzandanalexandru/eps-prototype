// What Studio shows for a price: "Fan pays" (all-in) and "You receive", under the policy in force.
import { effectivePolicy, feeBreakdown, type FeeBreakdown } from "../pricing/fees";
import type { EventGraph } from "./domain";

export function priceSides(g: Pick<EventGraph, "organiserPolicy" | "fee" | "event">, faceMinor: number): FeeBreakdown {
  return feeBreakdown(faceMinor, effectivePolicy(g.organiserPolicy, g.event.fee_policy), g.fee);
}

/** One-line explanation key per product type (studio.productTypes.<type>.how). */
export const PRODUCT_TYPE_ORDER = ["pass", "day_ticket", "flex_day", "add_on", "voucher", "table", "time_slot"] as const;
