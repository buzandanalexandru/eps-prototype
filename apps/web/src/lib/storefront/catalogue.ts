// What catalogue sections show (design D2.2): a view of each referenced event built from the live
// catalogue at render time, with all-in prices from lib/pricing. The document never holds prices.
import type { Localized } from "../catalogue/schema";
import type { Currency } from "../pricing/money";

export type ShopProduct = {
  id: string;
  type: string;
  name: Localized;
  perks: Localized[];
  allInMinor: number;
  /** The platform fee inside allInMinor that the fan pays (pass on or split); 0 when absorbed. */
  fanFeeMinor: number;
  /** Published shops only (step 3): the sale terms from the live snapshot, for the ticket picker. */
  sale?: ShopSaleProduct;
};

/** What the published shop's ticket picker and price notes need about one product (design D3.7, D3.8). */
export type ShopSaleProduct = {
  stepIndex: number;
  /** All-in amount and fee of every price step, in ladder order. */
  steps: { allInMinor: number; fanFeeMinor: number }[];
  mode: "by_date" | "by_sales";
  /** A date ladder's next step after the one in force now (all-in, and when it starts). */
  next: { allInMinor: number; fromAt: string } | null;
  salesFrom: string | null;
  salesUntil: string | null;
  maxPerOrder: number;
  /** Add-ons: at least one of these products must be in the order. */
  requires: string[];
  isTicket: boolean;
  /** What the buyer picks: nothing, a day (day pools), or a day and a start time (slot pools). */
  choice: "none" | "day" | "slot";
  days: string[];
  slots: string[];
  firstStart: string | null;
  /** Instalments for a purchase now, capped per buyer: deposit, then `count` payments (the last may differ). */
  instalments: { depositMinor: number; count: number; eachMinor: number; lastMinor: number; totalMinor: number } | null;
};

export type ShopEvent = {
  id: string;
  slug: string;
  name: Localized;
  startsAt: string;
  endsAt: string;
  timezone: string;
  currency: Currency;
  /** `map`: the stored static map image (/assets/maps/...), or null when none is stored yet. */
  venue: { name: string; address: string; lat: number | null; lng: number | null; map: string | null } | null;
  days: { date: string; opens: string; closes: string }[];
  lineup: { date: string; artist: string; stage: string; starts: string }[];
  products: ShopProduct[];
  /** Published shops only: the live snapshot this view was built from. */
  sale?: { snapshotId: string; version: number; maxPerAccount: number | null };
};

export type ShopCatalogue = Record<string, ShopEvent>;

export type ShopBrand = {
  name: string;
  slug: string;
  currency: Currency;
  organiserName: string;
  isSample: boolean;
};
