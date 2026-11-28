// The sale snapshot (design D3.2, schemaVersion 1): what one publish fixes for one event. The JSON
// Schema in api/schemas/sale-snapshot.v1.json is the contract; services/internal/snapshot mirrors
// these types in Go. Money is integer minor units; every price step carries its precomputed all-in
// amount, so no consumer recomputes fees. No personal data and no draft-only fields.
import type { FeePolicy } from "../pricing/fees";
import type { Currency } from "../pricing/money";
import type { PoolScope, ProductType } from "../catalogue/schema";

export const SNAPSHOT_SCHEMA_VERSION = 1 as const;

export type L10n = { ro: string; en: string };

export type SnapshotPriceStep = {
  index: number;
  mode: "by_date" | "by_sales";
  /** by_date steps after the first: when the step starts (ISO instant); otherwise null. */
  fromAt: string | null;
  /** by_sales steps after the first: units sold after which the step applies; otherwise null. */
  afterSold: number | null;
  faceMinor: number;
  allInMinor: number;
  fanFeeMinor: number;
  organiserNetMinor: number;
};

export type SnapshotInstalments = {
  /** A fixed deposit, or null when the deposit is a share (depositBps) of the all-in price. */
  depositMinor: number | null;
  depositBps: number | null;
  /** The most monthly payments; each buyer gets as many as still fit from the purchase date. */
  monthlyCount: number;
  dayOfMonth: number;
  /** The last local date a monthly payment may fall on (7 days before the event's local start). */
  lastDueBy: string;
};

export type SnapshotProduct = {
  productId: string;
  type: ProductType;
  name: L10n;
  perks: L10n[];
  /** The zone of a pass; null for other types. */
  zone: string | null;
  config: Record<string, unknown>;
  named: boolean;
  salesFrom: string | null;
  salesUntil: string | null;
  visible: boolean;
  maxPerOrder: number;
  sortOrder: number;
  poolIds: string[];
  /** Add-ons: an order must hold at least one of these products (empty: needs nothing). */
  requires: string[];
  priceSteps: SnapshotPriceStep[];
  instalments: SnapshotInstalments | null;
  /** Step 7 (absent in older snapshots): a named product's personalisation deadline (ISO instant). */
  personaliseBy?: string | null;
  /** Step 7: the online check-in deadline (ISO instant); null: no online check-in. */
  checkinBy?: string | null;
  /** Step 7: the late check-in fee after the deadline, priced like a price step. */
  lateCheckin?: { faceMinor: number; allInMinor: number; fanFeeMinor: number; organiserNetMinor: number } | null;
};

export type SnapshotPoolInstance = {
  /** `<poolId>`, `<poolId>@<YYYY-MM-DD>` or `<poolId>@<YYYY-MM-DD>T<HH:MM>` (spec map). */
  instanceId: string;
  date: string | null;
  slotStart: string | null;
  capacity: number;
};

export type SnapshotPool = { poolId: string; name: string; scope: PoolScope; instances: SnapshotPoolInstance[] };

export type SnapshotEvent = {
  slug: string;
  name: L10n;
  description: L10n;
  startsAt: string;
  endsAt: string;
  timezone: string;
  venue: { name: string; address: string; lat: number | null; lng: number | null } | null;
  days: { date: string; opens: string; closes: string }[];
  lineup: { date: string; artist: string; stage: string; starts: string; sample: boolean }[];
};

export type SaleSnapshot = {
  schemaVersion: typeof SNAPSHOT_SCHEMA_VERSION;
  snapshotId: string;
  organiserId: string;
  brandId: string;
  eventId: string;
  version: number;
  publishedAt: string;
  currency: Currency;
  timezone: string;
  isSample: boolean;
  fee: { policy: FeePolicy; bps: number; fixedMinor: number };
  event: SnapshotEvent;
  rules: {
    minAge: number | null;
    consentUnderAge: number | null;
    maxPerAccount: number | null;
    /** Checkout hold length in minutes (5 to 15, default 8; step 4). */
    holdMinutes: number;
    /** The shop's checkout slots (storefront document checkout.slots) at publish; step 4 asks them. */
    checkoutQuestions: { afterContact: string[]; consents: string[] };
  };
  pools: SnapshotPool[];
  products: SnapshotProduct[];
};

/** The sale terms: the document without the fields that change with every version. */
export type SaleTerms = Omit<SaleSnapshot, "snapshotId" | "version" | "publishedAt">;
