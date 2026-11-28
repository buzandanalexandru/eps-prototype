// A second, unrelated sample organiser (sample-data capability), so the demo shows two organisers
// and their separation. Entirely invented: SAMPLE DATA. It has no members.
import type { SampleOrganiser } from "./types";

export const sunwave: SampleOrganiser = {
  key: "sunwave",
  name: "Sunwave",
  slug: "sunwave",
  owner: null,
  feePolicy: "split",
  brands: [
    {
      key: "festival",
      name: "Sunwave Festival",
      slug: "sunwave-festival",
      languages: ["ro", "en"],
      currency: "EUR",
      template: "club",
      storefront: { template: "club", logo: "sample:logo-sunwave", event: "beach-day-2027" },
    },
  ],
  venues: [{ key: "beach", name: "Sunwave Beach", address: "Vama Veche, Constanța, România", lat: 43.7531, lng: 28.5753, stages: ["Beach Stage"] }],
  events: [
    {
      key: "beach-day-2027",
      brand: "festival",
      venue: "beach",
      slug: "sunwave-beach-day-2027",
      name: { ro: "Sunwave Beach Day 2027", en: "Sunwave Beach Day 2027" },
      description: { ro: "O zi de muzică pe plajă, de la prânz până la miezul nopții.", en: "A day of music on the beach, from noon to midnight." },
      starts: "2027-08-21T12:00",
      ends: "2027-08-21T23:59",
      days: { from: "2027-08-21", to: "2027-08-21", opens: "12:00", closes: "23:59" },
      pools: [{ key: "beach", name: "Beach", capacity: 2000, scope: "event" }],
      products: [
        {
          key: "day",
          type: "day_ticket",
          name: { ro: "Bilet de o zi", en: "Day ticket" },
          config: { day: "2027-08-21" },
          maxPerOrder: 6,
          sort: 1,
          pools: ["beach"],
          ladder: { mode: "by_date", steps: [{ priceMinor: 3500 }, { fromAt: "2027-06-01T00:00:00+03:00", priceMinor: 4500 }] },
        },
      ],
    },
  ],
};
