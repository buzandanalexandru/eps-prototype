// The trip wallet's timeline and next actions (step 7, design D9): pure functions over the core's
// GET /v1/me/trip answer, shared by the wallet, the live ticket and the fan account.
import type { components } from "../core/schema";

export type Trip = components["schemas"]["Trip"];
export type TripEvent = components["schemas"]["TripEvent"];
export type TicketItem = components["schemas"]["TicketItem"];

/** Items the holder holds now (a cancelled ticket stays in the account's list, not in the wallet). */
export function liveItems(e: TripEvent): TicketItem[] {
  return e.items.filter((i) => i.state !== "cancelled");
}

const TYPE_ORDER: Record<string, number> = { pass: 0, day_ticket: 1, flex_day: 2, voucher: 3, add_on: 4, table: 5, time_slot: 6 };

/** When an item happens: its own time, else its event's start. */
export function itemTime(e: TripEvent, i: TicketItem): number {
  return Date.parse(i.startsAt ?? e.startsAt);
}

/** Events in start order, each with its items in time order (items without a time at the start). */
export function timeline(trip: Trip): { event: TripEvent; items: TicketItem[] }[] {
  return [...trip.events]
    .filter((e) => liveItems(e).length > 0 || e.pending.length > 0)
    .sort((a, b) => Date.parse(a.startsAt) - Date.parse(b.startsAt) || a.eventId.localeCompare(b.eventId))
    .map((event) => ({
      event,
      items: liveItems(event).sort(
        (a, b) =>
          itemTime(event, a) - itemTime(event, b) ||
          (TYPE_ORDER[a.productType] ?? 9) - (TYPE_ORDER[b.productType] ?? 9) ||
          a.orderId.localeCompare(b.orderId) ||
          a.lineNo - b.lineNo ||
          a.unitNo - b.unitNo,
      ),
    }));
}

export type ActionKind = "pay_overdue" | "name" | "pick_day" | "check_in" | "show_code";
export type NextAction = { kind: ActionKind; ticketId: string; eventId: string; orderId: string; deadline: string | null };

const PRIORITY: Record<ActionKind, number> = { pay_overdue: 0, name: 1, pick_day: 2, check_in: 3, show_code: 4 };

/** Whether `now` is on the event's days (from 12 hours before its start to its end). */
export function isEventDay(e: TripEvent, now: number): boolean {
  return now >= Date.parse(e.startsAt) - 12 * 3600_000 && now <= Date.parse(e.endsAt);
}

/** Everything still to do for an item, most urgent first. */
export function actionsOf(e: TripEvent, i: TicketItem): NextAction[] {
  if (i.state === "cancelled") return [];
  const base = { ticketId: i.id, eventId: e.eventId, orderId: i.orderId };
  const out: NextAction[] = [];
  if (i.state === "suspended" || i.overdue) out.push({ ...base, kind: "pay_overdue", deadline: i.overdue?.graceUntil ?? null });
  if (i.named && i.needsName && !i.nameLocked) out.push({ ...base, kind: "name", deadline: i.personaliseBy ?? null });
  if (i.pickDays && i.pickDays.length > 0) out.push({ ...base, kind: "pick_day", deadline: e.startsAt });
  if (i.checkinBy && !i.checkedInAt) out.push({ ...base, kind: "check_in", deadline: i.checkinBy });
  return out.sort((a, b) => {
    if (a.kind === "pay_overdue" || b.kind === "pay_overdue") return PRIORITY[a.kind] - PRIORITY[b.kind];
    const da = a.deadline ? Date.parse(a.deadline) : Infinity;
    const db = b.deadline ? Date.parse(b.deadline) : Infinity;
    return da - db || PRIORITY[a.kind] - PRIORITY[b.kind];
  });
}

/**
 * An item's one next action: pay an overdue instalment first, then the earliest deadline among
 * naming, picking the day and checking in; on the event day with nothing else left, show the code.
 */
export function nextAction(e: TripEvent, i: TicketItem, now: number): NextAction | null {
  const a = actionsOf(e, i);
  if (a.length > 0) return a[0];
  if (i.state === "valid" && isEventDay(e, now)) return { ticketId: i.id, eventId: e.eventId, orderId: i.orderId, kind: "show_code", deadline: null };
  return null;
}

/** The single most urgent action across the trip (the wallet's header). */
export function headerAction(trip: Trip, now: number): NextAction | null {
  const all: NextAction[] = [];
  for (const e of trip.events) for (const i of liveItems(e)) {
    const a = nextAction(e, i, now);
    if (a) all.push(a);
  }
  all.sort((a, b) => {
    if (a.kind === "pay_overdue" || b.kind === "pay_overdue") return PRIORITY[a.kind] - PRIORITY[b.kind];
    const da = a.deadline ? Date.parse(a.deadline) : Infinity;
    const db = b.deadline ? Date.parse(b.deadline) : Infinity;
    return da - db || PRIORITY[a.kind] - PRIORITY[b.kind];
  });
  return all[0] ?? null;
}
