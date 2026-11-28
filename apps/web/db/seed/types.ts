// Shape of a sample organiser definition. Everything in it passes through the same zod schemas and
// domain functions as Studio (apply.ts); nothing is inserted raw.
import type { BrandInput } from "@/lib/organisers/domain";
import type { InstalmentInput, LadderInput, PoolInput, ProductInput, RulesInput, VenueInput } from "@/lib/catalogue/schema";
import type { FeePolicy } from "@/lib/pricing/fees";
import type { TemplateId } from "@/lib/storefront/templates";

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

/** A brand's sample shop: a starting template with the brand's logo and first event, published once. */
export type SampleStorefront = { template: TemplateId; logo: string; event?: string };

export type SampleOrganiser = {
  key: string;
  name: string;
  slug: string;
  /** "demo-organiser": the shared demo organiser user (step 0) becomes owner. */
  owner: "demo-organiser" | null;
  feePolicy: FeePolicy;
  // hostnames: reserved .test names the shop answers on (local host-routing tests).
  brands: (BrandInput & { key: string; storefront?: SampleStorefront; hostnames?: string[] })[];
  venues: (VenueInput & { key: string })[];
  events: SampleEvent[];
};
