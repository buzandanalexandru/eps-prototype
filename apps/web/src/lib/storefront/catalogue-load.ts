// Loads the catalogue view of the events a document refers to, inside the organiser's transaction
// (row-level security: another organiser's event is simply absent). A fixed number of queries for
// any number of events (loadEventGraphs).
import type { ClientBase } from "pg";
import { isUuid, loadEventGraphs, stepsOf } from "../catalogue/domain";
import { priceSides } from "../catalogue/display";
import { activePriceStep } from "../pricing/ladder";
import type { ShopCatalogue, ShopEvent } from "./catalogue";

const MAX_EVENTS = 60;

export async function loadShopCatalogue(c: ClientBase, eventIds: string[], now = new Date()): Promise<ShopCatalogue> {
  const ids = [...new Set(eventIds.filter(isUuid))].slice(0, MAX_EVENTS);
  if (!ids.length) return {};
  const graphs = await loadEventGraphs(c, ids);
  const venueIds = [...new Set(graphs.map((g) => g.venue?.id).filter((v): v is string => !!v))];
  const venues = venueIds.length
    ? (await c.query<{ id: string; name: string; address: string; lat: number | null; lng: number | null }>(
        "select id, name, address, lat, lng from control.venues where id = any($1::uuid[])",
        [venueIds],
      )).rows
    : [];
  const out: ShopCatalogue = {};
  for (const g of graphs) {
    const dayOf = new Map(g.days.map((d) => [d.id, d.day]));
    const products = g.products
      .filter((p) => p.visible)
      .sort((a, b) => a.sort - b.sort || a.id.localeCompare(b.id))
      .flatMap((p) => {
        const steps = stepsOf(g.steps, p.id);
        if (!steps.length) return [];
        const sides = priceSides(g, activePriceStep({ steps, at: now }).priceMinor);
        return [{ id: p.id, type: p.type, name: p.name, perks: p.perks ?? [], allInMinor: sides.allInMinor, fanFeeMinor: sides.fanFeeMinor }];
      });
    const v = venues.find((x) => x.id === g.venue?.id);
    const ev: ShopEvent = {
      id: g.event.id,
      slug: g.event.slug,
      name: g.event.name,
      startsAt: g.event.starts_at.toISOString(),
      endsAt: g.event.ends_at.toISOString(),
      timezone: g.event.timezone,
      currency: g.brand.currency,
      venue: v ? { name: v.name, address: v.address, lat: v.lat, lng: v.lng } : null,
      days: g.days.map((d) => ({ date: d.day, opens: d.opens.slice(0, 5), closes: d.closes.slice(0, 5) })).sort((a, b) => a.date.localeCompare(b.date)),
      lineup: g.lineup
        .map((l) => ({ date: dayOf.get(l.day_id) ?? "", artist: l.artist, stage: l.stage, starts: l.starts.slice(0, 5), sort: l.sort }))
        .sort((a, b) => a.date.localeCompare(b.date) || a.sort - b.sort)
        .map(({ sort: _s, ...l }) => l), // eslint-disable-line @typescript-eslint/no-unused-vars
      products,
    };
    out[ev.id] = ev;
  }
  return out;
}

/** The organiser's events for the builder's pickers (id, name, slug), newest first, bounded. */
export async function eventChoices(c: ClientBase, brandId: string): Promise<{ id: string; slug: string; name: { ro: string; en: string } }[]> {
  return (
    await c.query("select id, slug, name from control.events where brand_id = $1 order by starts_at desc, id limit 100", [brandId])
  ).rows;
}
