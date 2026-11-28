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
};

export type ShopCatalogue = Record<string, ShopEvent>;

export type ShopBrand = {
  name: string;
  slug: string;
  currency: Currency;
  organiserName: string;
  isSample: boolean;
};
