// NIBIRU 2027, rebuilt as sample organiser #1 (sample-data capability; C10). Facts come from the
// teardown (docs/research/nibiru-eps-teardown.md §1, §3, §5, §10). Values the teardown does not
// state are marked ASSUMPTION and can be tuned here only.
// This is SAMPLE DATA: the organiser is flagged is_sample and labelled in Studio.
import type { SampleOrganiser, SampleProduct } from "./types";

const GALAXIA_DAYS = ["2027-07-15", "2027-07-16", "2027-07-17", "2027-07-18"];
const SEASON = { from: "2027-06-24", to: "2027-09-05" }; // 74 days, open daily 17:00 to 03:00
function seasonDays(): string[] {
  const out: string[] = [];
  for (let d = new Date(`${SEASON.from}T00:00:00Z`); d.toISOString().slice(0, 10) <= SEASON.to; d.setUTCDate(d.getUTCDate() + 1)) {
    out.push(d.toISOString().slice(0, 10));
  }
  return out;
}

// Galaxia presale opened before the teardown (27 Nov 2026). ASSUMPTION: 1 Oct 2026.
const PRESALE = "2026-10-01T09:00:00Z";
// "10 RON now locks the price, then 9 monthly instalments" (teardown §5). A plan's count is a
// maximum, capped per buyer so the last payment falls 7 days before the event: at demo time (early
// December 2026) a buyer gets 7, so the sample offers "10 RON now and up to 7 monthly payments".
const NIBIRU_PLAN = { depositMinor: 1000, monthlyCount: 7, dayOfMonth: 1 };

function pass(key: string, ro: string, en: string, zone: string, priceMinor: number, perks: [string, string][], pools: string[], sort: number): SampleProduct {
  return {
    key,
    type: "pass",
    name: { ro, en },
    config: { zone, days: GALAXIA_DAYS },
    perks: perks.map(([r, e]) => ({ ro: r, en: e })),
    named: true,
    salesFrom: PRESALE,
    maxPerOrder: 6,
    sort,
    pools,
    ladder: { mode: "by_date", steps: [{ priceMinor }] },
    plan: NIBIRU_PLAN,
  };
}

const FAST = ["Acces rapid (fast lane)", "Fast lane"] as [string, string];
const TOILETS = ["Toalete VIP", "VIP toilets"] as [string, string];

export const nibiru: SampleOrganiser = {
  key: "nibiru",
  name: "NIBIRU",
  slug: "nibiru",
  owner: "demo-organiser",
  feePolicy: "pass_on",
  brands: [
    { key: "festivals", name: "NIBIRU Festivals", slug: "nibiru", languages: ["ro", "en"], currency: "RON", template: "festival" },
    { key: "resort", name: "NIBIRU Resort", slug: "nibiru-resort", languages: ["ro", "en"], currency: "RON", template: "resort" },
  ],
  venues: [
    { key: "arena", name: "NIBIRU Arena", address: "Costinești – Tuzla, Constanța, România", lat: 43.9295, lng: 28.6312, stages: ["Main Stage", "Galaxy Stage"] },
    { key: "promenada", name: "Promenada", address: "Costinești – Tuzla, Constanța, România", lat: 43.931, lng: 28.633, stages: [] },
    { key: "center-stage", name: "Center Stage", address: "Promenada NIBIRU", lat: 43.9312, lng: 28.6335, stages: ["Center Stage"] },
    { key: "beer-garden", name: "Berăria NIBIRU (Beer Garden)", address: "Promenada NIBIRU", lat: 43.9305, lng: 28.6322, stages: ["Beer Garden"] },
    { key: "the-fool", name: "The Fool", address: "Promenada NIBIRU", lat: 43.9301, lng: 28.6318, stages: ["Amfiteatru"] },
    { key: "padel", name: "Terenuri de padel", address: "NIBIRU Resort", lat: 43.9299, lng: 28.6327, stages: [] },
  ],
  events: [
    {
      key: "galaxia-2027",
      brand: "festivals",
      venue: "arena",
      slug: "galaxia-2027",
      name: { ro: "Galaxia 2027", en: "Galaxia 2027" },
      description: {
        ro: "Patru nopți de muzică electronică pe NIBIRU Arena, la malul mării. Bilete nominale; abonamente pe zone.",
        en: "Four nights of electronic music at NIBIRU Arena by the sea. Named tickets; passes by zone.",
      },
      starts: "2027-07-15T17:00",
      ends: "2027-07-19T03:00",
      days: { from: "2027-07-15", to: "2027-07-18", opens: "17:00", closes: "03:00" },
      // ASSUMPTION: from 14, parental consent under 18, at most 6 per account.
      rules: { minAge: 14, consentUnderAge: 18, maxPerAccount: 6 },
      // ASSUMPTION: the real line-up is not announced; placeholder sample artists.
      lineup: [
        { day: "2027-07-15", artist: "Nova Drift (artist de exemplu)", stage: "Main Stage", starts: "22:00" },
        { day: "2027-07-15", artist: "Lumen Kids (artist de exemplu)", stage: "Galaxy Stage", starts: "19:00" },
        { day: "2027-07-15", artist: "Orbital Sea (artist de exemplu)", stage: "Main Stage", starts: "00:30" },
        { day: "2027-07-16", artist: "Kepler Nine (artist de exemplu)", stage: "Main Stage", starts: "23:00" },
        { day: "2027-07-16", artist: "Tidal Static (artist de exemplu)", stage: "Galaxy Stage", starts: "20:00" },
        { day: "2027-07-17", artist: "Solar Choir (artist de exemplu)", stage: "Main Stage", starts: "23:30" },
        { day: "2027-07-18", artist: "Last Light (artist de exemplu)", stage: "Main Stage", starts: "22:30" },
      ],
      pools: [
        // ASSUMPTION: pool sizes (the teardown claims 282k+ visits over the 2026 edition).
        { key: "arena", name: "Arena", capacity: 40000, scope: "event" },
        { key: "golden-circle", name: "Golden Circle", capacity: 6000, scope: "event" },
        { key: "vip-platform", name: "VIP platform", capacity: 2000, scope: "event" },
        { key: "ultra-vip", name: "Ultra VIP", capacity: 300, scope: "event" },
        { key: "camping", name: "Camping", capacity: 3000, scope: "event" },
      ],
      products: [
        {
          // ASSUMPTION: a sales ladder, 539 then 599 RON after 10,000 sold.
          ...pass("ga", "General Access", "General Access", "Arena", 53900, [], ["arena"], 1),
          ladder: { mode: "by_sales", steps: [{ priceMinor: 53900 }, { afterSold: 10000, priceMinor: 59900 }] },
        },
        pass("ga-plus", "General Access Plus", "General Access Plus", "Arena", 65900, [FAST], ["arena"], 2),
        pass("golden-circle", "Golden Circle", "Golden Circle", "Golden Circle", 83900, [FAST, TOILETS], ["golden-circle"], 3),
        pass("vip", "VIP", "VIP", "VIP platform", 93900, [FAST, TOILETS, ["Rezervare de masă", "Table reservation"]], ["vip-platform", "arena"], 4),
        pass("ultra-vip", "Ultra VIP", "Ultra VIP", "Ultra VIP", 123900, [FAST, TOILETS, ["Masă Ultra VIP", "Ultra VIP table"]], ["ultra-vip"], 5),
        {
          key: "camping",
          type: "add_on",
          name: { ro: "Camping Galaxia", en: "Galaxia camping" },
          perks: [{ ro: "Loc de cort pentru 4 nopți", en: "Tent pitch for 4 nights" }],
          config: { requires: "any_ticket" },
          named: false,
          salesFrom: PRESALE,
          maxPerOrder: 4,
          sort: 6,
          pools: ["camping"],
          // ASSUMPTION: camping price (empty for 2027 in the teardown).
          ladder: { mode: "by_date", steps: [{ priceMinor: 19900 }] },
        },
      ],
    },
    {
      key: "promenada-2027",
      brand: "resort",
      venue: "promenada",
      slug: "promenada-2027",
      name: { ro: "Acces Promenada 2027", en: "Promenada access 2027" },
      description: {
        ro: "Intrare pe Promenada NIBIRU, deschisă zilnic între 17:00 și 03:00, 24 iunie – 5 septembrie. Copiii sub 5 ani intră gratuit.",
        en: "Entry to the NIBIRU Promenada, open daily 17:00 to 03:00, 24 June to 5 September. Children under 5 go free.",
      },
      starts: "2027-06-24T17:00",
      ends: "2027-09-06T03:00",
      days: { ...SEASON, opens: "17:00", closes: "03:00" },
      pools: [
        { key: "promenada", name: "Promenada (pe zi)", capacity: 8000, scope: "day" },
        // Any-Day tickets draw on an event pool (the day is picked later). ASSUMPTION: size.
        { key: "any-day", name: "Any-Day (sezon)", capacity: 20000, scope: "event" },
      ],
      products: [
        {
          key: "pick-a-day",
          type: "day_ticket",
          name: { ro: "Promenada · alegi ziua", en: "Promenada · pick a day" },
          perks: [],
          config: { day: null },
          maxPerOrder: 10,
          sort: 1,
          pools: ["promenada"],
          // 10 → 11 → 12 → 13 (1 Jan) → … → 35 (24 Jul), from the teardown; ASSUMPTION: the
          // intermediate dates not shown on the site.
          ladder: {
            mode: "by_date",
            steps: [
              { priceMinor: 1000 },
              { fromAt: "2026-12-01T00:00:00+02:00", priceMinor: 1100 },
              { fromAt: "2026-12-20T00:00:00+02:00", priceMinor: 1200 },
              { fromAt: "2027-01-01T00:00:00+02:00", priceMinor: 1300 },
              { fromAt: "2027-03-01T00:00:00+02:00", priceMinor: 1500 },
              { fromAt: "2027-05-01T00:00:00+03:00", priceMinor: 2000 },
              { fromAt: "2027-06-24T00:00:00+03:00", priceMinor: 2500 },
              { fromAt: "2027-07-24T00:00:00+03:00", priceMinor: 3500 },
            ],
          },
        },
        {
          key: "any-day",
          type: "flex_day",
          name: { ro: "Promenada Any-Day", en: "Promenada Any-Day" },
          perks: [{ ro: "Alegi ziua mai târziu", en: "Pick the day later" }],
          config: { days: seasonDays() },
          maxPerOrder: 10,
          sort: 2,
          pools: ["any-day"],
          // From 15 RON (teardown); ASSUMPTION: the later steps.
          ladder: {
            mode: "by_date",
            steps: [
              { priceMinor: 1500 },
              { fromAt: "2027-03-01T00:00:00+02:00", priceMinor: 2000 },
              { fromAt: "2027-07-24T00:00:00+03:00", priceMinor: 3500 },
            ],
          },
        },
      ],
    },
    {
      key: "center-stage-2027",
      brand: "resort",
      venue: "center-stage",
      slug: "center-stage-2027",
      name: { ro: "Seri la Center Stage 2027", en: "Center Stage evenings 2027" },
      description: {
        ro: "Concerte gratuite pe Promenada. Biletul de seară este un voucher de consumație de 25 RON.",
        en: "Free concerts on the Promenada. The evening ticket is a 25 RON drinks voucher.",
      },
      starts: "2027-06-24T17:00",
      ends: "2027-09-06T03:00",
      days: { ...SEASON, opens: "17:00", closes: "03:00" },
      pools: [{ key: "center-stage", name: "Center Stage (pe seară)", capacity: 3000, scope: "day" }], // ASSUMPTION: size
      products: [
        {
          key: "drink-voucher",
          type: "voucher",
          name: { ro: "Seară Center Stage · voucher băuturi", en: "Center Stage evening · drinks voucher" },
          perks: [{ ro: "Consumație de 25 RON inclusă", en: "25 RON to spend at the bars" }],
          config: { spendValueMinor: 2500 },
          maxPerOrder: 10,
          sort: 1,
          pools: ["center-stage"],
          ladder: { mode: "by_date", steps: [{ priceMinor: 2500 }] },
        },
      ],
    },
    {
      key: "beer-garden-bdlp",
      brand: "resort",
      venue: "beer-garden",
      slug: "bdlp-beer-garden-2027",
      name: { ro: "BDLP live la Berăria NIBIRU", en: "BDLP live at the Beer Garden" },
      description: {
        ro: "Concert cu loc garantat la masă. Categoria mesei dă zona și vizibilitatea scenei.",
        en: "A concert with a guaranteed table seat. The table category sets the zone and the view of the stage.",
      },
      starts: "2027-07-10T20:00",
      ends: "2027-07-11T02:00",
      days: { from: "2027-07-10", to: "2027-07-10", opens: "20:00", closes: "02:00" },
      pools: [
        // ASSUMPTION: 1,000 seats as tables of 4 across three categories.
        { key: "cat-a", name: "Mese categoria A", capacity: 40, scope: "event" },
        { key: "cat-b", name: "Mese categoria B", capacity: 80, scope: "event" },
        { key: "cat-c", name: "Mese categoria C", capacity: 130, scope: "event" },
      ],
      products: (
        [
          ["c", "C", 7900, 3],
          ["b", "B", 9900, 2],
          ["a", "A", 14900, 1],
        ] as const
      ).map(([k, cat, price, sort]) => ({
        key: `table-${k}`,
        type: "table" as const,
        name: { ro: `Masă categoria ${cat}`, en: `Category ${cat} table` },
        perks: [{ ro: "Loc garantat la masă", en: "Guaranteed seat at a table" }],
        config: { category: cat, seats: 4 },
        salesFrom: "2026-11-01T09:00:00Z",
        maxPerOrder: 2,
        sort,
        pools: [`cat-${k}`],
        ladder: { mode: "by_date" as const, steps: [{ priceMinor: price }] },
        // "Berăria payable in up to 4 equal monthly instalments" (teardown §10).
        plan: { depositMinor: 0, monthlyCount: 4, dayOfMonth: 1 },
      })),
    },
    {
      key: "padel-2027",
      brand: "resort",
      venue: "padel",
      slug: "padel-2027",
      name: { ro: "Padel la NIBIRU 2027", en: "Padel at NIBIRU 2027" },
      description: {
        ro: "Rezervi terenul pe intervale de 30 de minute, până la 2 ore la rând.",
        en: "Book a court in 30-minute slots, up to 2 hours in a row.",
      },
      starts: "2027-06-24T17:00",
      ends: "2027-09-06T03:00",
      days: { ...SEASON, opens: "17:00", closes: "03:00" },
      pools: [{ key: "courts", name: "Terenuri (pe interval)", capacity: 4, scope: "slot" }], // ASSUMPTION: 4 courts
      products: [
        {
          key: "padel-slot",
          type: "time_slot",
          name: { ro: "Teren de padel · 30 de minute", en: "Padel court · 30 minutes" },
          perks: [{ ro: "Rachete incluse", en: "Rackets included" }],
          config: { slotMinutes: 30, firstStart: "17:00", lastStart: "02:30", maxConsecutiveSlots: 4 },
          maxPerOrder: 4,
          sort: 1,
          pools: ["courts"],
          ladder: { mode: "by_date", steps: [{ priceMinor: 5000 }] },
        },
      ],
    },
    {
      key: "stand-up-the-fool",
      brand: "resort",
      venue: "the-fool",
      slug: "stand-up-the-fool-2027-07-08",
      name: { ro: "Stand-up la The Fool", en: "Stand-up at The Fool" },
      description: {
        ro: "O seară de stand-up în amfiteatrul în aer liber. Locuri la mese comune, în ordinea sosirii. Accesul sub 16 ani nu este permis.",
        en: "A stand-up night in the open-air amphitheatre. Shared tables, first come first seated. No entry under 16.",
      },
      starts: "2027-07-08T21:00",
      ends: "2027-07-08T23:30",
      days: { from: "2027-07-08", to: "2027-07-08", opens: "20:30", closes: "23:30" },
      rules: { minAge: 16, maxPerAccount: 8 },
      pools: [
        { key: "general", name: "Arena generală", capacity: 450, scope: "event" }, // ASSUMPTION
        { key: "front", name: "Front Stage", capacity: 80, scope: "event" }, // ASSUMPTION
      ],
      products: [
        {
          key: "general",
          type: "day_ticket",
          name: { ro: "Arena generală", en: "General Arena" },
          config: { day: "2027-07-08" },
          maxPerOrder: 8,
          sort: 1,
          pools: ["general"],
          ladder: { mode: "by_date", steps: [{ priceMinor: 4250 }] },
        },
        {
          key: "front",
          type: "day_ticket",
          name: { ro: "Front Stage", en: "Front Stage" },
          config: { day: "2027-07-08" },
          maxPerOrder: 8,
          sort: 2,
          pools: ["front"],
          ladder: { mode: "by_date", steps: [{ priceMinor: 4750 }] },
        },
      ],
    },
  ],
};
