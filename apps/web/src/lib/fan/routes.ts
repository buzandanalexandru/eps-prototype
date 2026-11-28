// The fan's ticket calls (step 7) and the door's calls, forwarded from our own origin to the core
// (the pages' CSP allows no other origin). Only the listed operations pass; nothing else of the
// core is reachable this way.
const UUID = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";
const LINK = "[A-Za-z0-9_.-]{20,400}";

export type Route = { method: string; path: RegExp; token: "account" | "none" };

/** /api/fan/<path> → the core's /v1/<path>. */
export const FAN_ROUTES: Route[] = [
  { method: "GET", path: /^ticket-keys$/, token: "none" },
  { method: "GET", path: /^me\/trip$/, token: "account" },
  { method: "GET", path: new RegExp(`^me/tickets/${UUID}$`), token: "account" },
  { method: "GET", path: new RegExp(`^me/tickets/${UUID}/codes$`), token: "account" },
  { method: "PUT", path: new RegExp(`^me/tickets/${UUID}/name$`), token: "account" },
  { method: "POST", path: new RegExp(`^me/tickets/${UUID}/(check-in|pick-day)$`), token: "account" },
  { method: "GET", path: new RegExp(`^ticket-links/${LINK}$`), token: "none" },
  { method: "GET", path: new RegExp(`^ticket-links/${LINK}/tickets/${UUID}/codes$`), token: "none" },
  { method: "PUT", path: new RegExp(`^ticket-links/${LINK}/tickets/${UUID}/name$`), token: "none" },
];

/** /api/door/<path> → the core's /v1/organiser/<path>, with the organiser's token. */
export const DOOR_ROUTES: Route[] = [
  { method: "POST", path: new RegExp(`^events/${UUID}/door/devices$`), token: "account" },
  { method: "GET", path: new RegExp(`^events/${UUID}/door-sync$`), token: "account" },
  { method: "POST", path: new RegExp(`^events/${UUID}/door/scans$`), token: "account" },
];

export function matchRoute(routes: Route[], method: string, path: string): Route | null {
  return routes.find((r) => r.method === method && r.path.test(path)) ?? null;
}

/** The query parameters the door sync reads (everything else is dropped). */
export function doorQuery(search: URLSearchParams): string {
  const out = new URLSearchParams();
  const since = search.get("since");
  if (since && /^\d{1,15}$/.test(since)) out.set("since", since);
  if (search.get("manifest") === "true") out.set("manifest", "true");
  const after = search.get("after");
  if (after && new RegExp(`^${UUID}$`).test(after)) out.set("after", after);
  const device = search.get("device");
  if (device && new RegExp(`^${UUID}$`).test(device)) out.set("device", device);
  const s = out.toString();
  return s ? `?${s}` : "";
}
