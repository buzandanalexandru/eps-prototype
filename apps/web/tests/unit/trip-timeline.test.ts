// Trip wallet spec "Mixed items for one event": one event group with the bus seat first, then the
// pass and camping at the event start, then the table and the padel slot in time order; events by
// date; cancelled items leave the wallet.
import { expect, it } from "vitest";
import { timeline } from "@/lib/trip/timeline";
import { event, item, trip } from "./trip-fixtures";

it("orders one event's items by their own time, items without one at the event start", () => {
  const padel = item({ productType: "time_slot", productName: { ro: "Padel", en: "Padel" }, startsAt: "2027-07-17T08:00:00Z" });
  const table = item({ productType: "table", productName: { ro: "Masă", en: "Table" }, startsAt: "2027-07-16T17:00:00Z" });
  const camping = item({ productType: "add_on", productName: { ro: "Camping", en: "Camping" } });
  const pass = item({ productType: "pass" });
  const bus = item({ productType: "time_slot", productName: { ro: "Autobuz", en: "Bus" }, startsAt: "2027-07-14T16:00:00Z" });
  const [g] = timeline(trip(event({ items: [padel, table, camping, pass, bus] })));
  expect(g.items.map((i) => i.productName.en)).toEqual(["Bus", "Pass", "Camping", "Table", "Padel"]);
});

it("groups events by start date and drops cancelled items and empty events", () => {
  const later = event({ eventId: "b", startsAt: "2027-08-01T10:00:00Z", endsAt: "2027-08-02T10:00:00Z", items: [item()] });
  const sooner = event({ eventId: "a", organiserId: "other", items: [item(), item({ state: "cancelled" })] });
  const gone = event({ eventId: "c", items: [item({ state: "cancelled" })] });
  const t = timeline(trip(later, gone, sooner));
  expect(t.map((g) => g.event.eventId)).toEqual(["a", "b"]);
  expect(t[0].items).toHaveLength(1);
});
