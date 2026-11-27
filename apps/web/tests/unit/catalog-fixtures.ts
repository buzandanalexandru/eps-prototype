// A small festival built through the domain functions, for the catalogue tests.
import type { ClientBase } from "pg";
import { saveEvent, savePool, saveProduct, saveVenue, setDays, setLadder, setProductPools } from "@/lib/catalogue/domain";
import { zonedToUtc } from "@/lib/catalogue/time";
import { uniq } from "./db";

export const FESTIVAL_DAYS = ["2027-07-15", "2027-07-16", "2027-07-17", "2027-07-18"];

export async function festival(c: ClientBase, brandId: string) {
  const venue = await saveVenue(c, { name: "NIBIRU Arena", stages: ["Main", "Galaxy"] });
  const ev = await saveEvent(c, {
    brandId,
    venueId: venue.id,
    slug: uniq("galaxia-2027"),
    name: { ro: "Galaxia 2027", en: "Galaxia 2027" },
    description: { ro: "Festival pe malul mării", en: "Festival by the sea" },
    timezone: "Europe/Bucharest",
    startsAt: zonedToUtc("2027-07-15T17:00", "Europe/Bucharest")!,
    endsAt: zonedToUtc("2027-07-19T03:00", "Europe/Bucharest")!,
  });
  const days = await setDays(c, ev.id, FESTIVAL_DAYS.map((day) => ({ day, opens: "17:00", closes: "03:00" })));
  const arena = await savePool(c, ev.id, { name: "Arena", capacity: 40000, scope: "event" });
  const ga = await saveProduct(c, ev.id, {
    type: "pass",
    name: { ro: "GA", en: "GA" },
    config: { zone: "Arena", days: FESTIVAL_DAYS },
    named: true,
  });
  await setLadder(c, ga.id, { mode: "by_date", steps: [{ priceMinor: 53900 }] });
  await setProductPools(c, ga.id, [arena.id]);
  return { venue, ev, days, arena, ga };
}
