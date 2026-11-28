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

/**
 * What the published shop's ticket picker and price notes need about one product (design D3.7,
 * D3.8). Only facts from the snapshot: everything that depends on the time (the step in force, the
 * sales status, instalments for a purchase now) is worked out in the browser (sale-view.ts), so the
 * page itself can be cached until the next publish.
 */
export type ShopSaleProduct = {
  steps: { mode: "by_date" | "by_sales"; fromAt: string | null; afterSold: number | null; allInMinor: number; fanFeeMinor: number }[];
  salesFrom: string | null;
  salesUntil: string | null;
  maxPerOrder: number;
  /** Add-ons: at least one of these (visible) products must be in the order. */
  requires: string[];
  isTicket: boolean;
  poolIds: string[];
  /** What the buyer picks: nothing, a day (day pools), or a day and a start time (slot pools). */
  choice: "none" | "day" | "slot";
  days: string[];
  slots: string[];
  firstStart: string | null;
  plan: { depositMinor: number | null; depositBps: number | null; monthlyCount: number; dayOfMonth: number } | null;
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
  lineup: { date: string; artist: string; stage: string; starts: string; sample?: boolean }[];
  products: ShopProduct[];
  /** Published shops only: the live snapshot this view was built from, and its pools' capacities. */
  sale?: {
    snapshotId: string;
    version: number;
    maxPerAccount: number | null;
    pools: { poolId: string; scope: "event" | "day" | "slot"; capacity: number }[];
    /** When the page was rendered (cached pages show it until the browser works out the present). */
    renderedAt: string;
  };
};

export type ShopCatalogue = Record<string, ShopEvent>;

export type ShopBrand = {
  name: string;
  slug: string;
  currency: Currency;
  organiserName: string;
  isSample: boolean;
};
