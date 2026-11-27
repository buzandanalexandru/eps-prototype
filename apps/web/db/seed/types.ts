// Shape of a sample organiser definition. Everything in it passes through the same zod schemas and
// domain functions as Studio (apply.ts); nothing is inserted raw.
import type { BrandInput } from "@/lib/organisers/domain";
import type { InstalmentInput, LadderInput, PoolInput, ProductInput, RulesInput, VenueInput } from "@/lib/catalogue/schema";
import type { FeePolicy } from "@/lib/pricing/fees";

export type SampleProduct = ProductInput & { key: string; pools: string[]; ladder: LadderInput; plan?: InstalmentInput };

export type SampleEvent = {
  key: string;
  brand: string;
  venue: string;
  slug: string;
  name: { ro: string; en: string };
  description: { ro: string; en: string };
  timezone?: string;
  /** Local wall times in the event's zone, "YYYY-MM-DDTHH:MM". */
  starts: string;
  ends: string;
  /** Every date from `from` to `to`, with the same hours. */
  days: { from: string; to: string; opens: string; closes: string };
  rules?: RulesInput;
  lineup?: { day: string; artist: string; stage: string; starts: string }[];
  pools: (PoolInput & { key: string })[];
  products: SampleProduct[];
};

export type SampleOrganiser = {
  key: string;
  name: string;
  slug: string;
  /** "demo-organiser": the shared demo organiser user (step 0) becomes owner. */
  owner: "demo-organiser" | null;
  feePolicy: FeePolicy;
  brands: (BrandInput & { key: string })[];
  venues: (VenueInput & { key: string })[];
  events: SampleEvent[];
};
