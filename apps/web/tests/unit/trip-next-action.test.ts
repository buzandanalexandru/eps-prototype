// Trip wallet spec "Next action with its deadline".
import { expect, it } from "vitest";
import { headerAction, nextAction } from "@/lib/trip/timeline";
import { event, item, trip } from "./trip-fixtures";

const DEC = Date.parse("2026-12-01T10:00:00Z");

it("Personalise before check-in: the earliest deadline first", () => {
  const i = item({ named: true, needsName: true, personaliseBy: "2027-06-01T21:00:00Z", checkinBy: "2027-07-10T21:00:00Z" });
  expect(nextAction(event({ items: [i] }), i, DEC)).toMatchObject({ kind: "name", deadline: "2027-06-01T21:00:00Z" });
  const named = { ...i, needsName: false, holderName: "Ana" };
  expect(nextAction(event(), named, DEC)).toMatchObject({ kind: "check_in", deadline: "2027-07-10T21:00:00Z" });
});

it("Overdue instalment comes first, in the item and in the header", () => {
  const i = item({ state: "suspended", overdue: { amountMinor: 7000 }, named: true, needsName: true, personaliseBy: "2026-12-05T00:00:00Z" });
  const other = item({ named: true, needsName: true, personaliseBy: "2026-12-02T00:00:00Z" });
  const e = event({ items: [other, i] });
  expect(nextAction(e, i, DEC)?.kind).toBe("pay_overdue");
  expect(headerAction(trip(e), DEC)).toMatchObject({ kind: "pay_overdue", ticketId: i.id });
});

it("Event day: with everything done, show the code", () => {
  const i = item({ checkinBy: "2027-07-10T21:00:00Z", checkedInAt: "2027-07-01T10:00:00Z" });
  const e = event({ items: [i] });
  expect(nextAction(e, i, DEC)).toBeNull();
  expect(nextAction(e, i, Date.parse("2027-07-15T16:00:00Z"))).toMatchObject({ kind: "show_code" });
  expect(headerAction(trip(e), Date.parse("2027-07-15T16:00:00Z"))?.kind).toBe("show_code");
});

it("a flexible ticket asks for its day; a locked name is not an action", () => {
  const flex = item({ productType: "flex_day", pickDays: ["2027-07-16", "2027-07-17"] });
  expect(nextAction(event(), flex, DEC)?.kind).toBe("pick_day");
  const locked = item({ named: true, needsName: true, nameLocked: true });
  expect(nextAction(event(), locked, DEC)).toBeNull();
});
