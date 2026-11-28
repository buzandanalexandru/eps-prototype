// Trip wallet fixtures: one event with a pass, camping, a table, a padel slot and a bus seat.
import type { TicketItem, Trip, TripEvent } from "@/lib/trip/timeline";

let n = 0;
export function item(over: Partial<TicketItem> = {}): TicketItem {
  n++;
  return {
    id: `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`,
    organiserId: "org",
    eventId: "ev",
    orderId: "ord",
    lineNo: n,
    unitNo: 1,
    productId: "p",
    snapshotVersion: 1,
    generation: 1,
    holderKind: "account",
    state: "valid",
    createdAt: "2026-12-01T10:00:00Z",
    productType: "pass",
    productName: { ro: "Abonament", en: "Pass" },
    needsName: false,
    named: false,
    nameLocked: false,
    ...over,
  };
}

export function event(over: Partial<TripEvent> = {}): TripEvent {
  return {
    eventId: "ev",
    organiserId: "org",
    brandId: "brand",
    name: { ro: "Galaxia 2027", en: "Galaxia 2027" },
    startsAt: "2027-07-15T14:00:00.000Z",
    endsAt: "2027-07-19T00:00:00.000Z",
    timezone: "Europe/Bucharest",
    currency: "RON",
    isSample: false,
    items: [],
    pending: [],
    ...over,
  };
}

export const trip = (...events: TripEvent[]): Trip => ({ serverTime: "2026-12-01T10:00:00Z", events });
