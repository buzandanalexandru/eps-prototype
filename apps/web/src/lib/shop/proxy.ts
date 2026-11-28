// The shop's checkout, order, payment and wallet calls (step 4) go from the browser to our own
// origin (the shop's fixed CSP allows no other) and from here to the Go core with a fresh core
// token: the visitor's account, or their guest identity (a signed cookie, created on first use).
// Only the listed paths are forwarded; nothing else of the core is reachable this way.

const UUID = "[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}";

const ROUTES: { method: string; path: RegExp }[] = [
  { method: "POST", path: /^orders$/ },
  { method: "GET", path: /^orders$/ },
  { method: "GET", path: new RegExp(`^orders/${UUID}$`) },
  { method: "PUT", path: new RegExp(`^orders/${UUID}/details$`) },
  { method: "POST", path: new RegExp(`^orders/${UUID}/(payments|cancel|instalments/simulate-next|instalments/pay-overdue|otp|otp/verify)$`) },
  { method: "POST", path: /^claims$/ },
  { method: "GET", path: /^saved-cards$/ },
  { method: "DELETE", path: new RegExp(`^saved-cards/${UUID}$`) },
  { method: "GET", path: /^wallets$/ },
  { method: "GET", path: new RegExp(`^wallets/${UUID}$`) },
  { method: "POST", path: new RegExp(`^wallets/${UUID}/topups$`) },
  { method: "POST", path: new RegExp(`^wallets/${UUID}/pay-code$`) },
  { method: "GET", path: new RegExp(`^wallets/${UUID}/topups/${UUID}$`) },
];

// The waiting room (step 5) is the gateway's, not the core's.
const QUEUE_ROUTES: { method: string; path: RegExp }[] = [
  { method: "POST", path: new RegExp(`^queue/${UUID}/join$`) },
  { method: "GET", path: new RegExp(`^queue/${UUID}/status$`) },
];

/** Whether a method and path (after /api/shop/) may be forwarded. */
export function allowedShopRoute(method: string, path: string): boolean {
  return ROUTES.some((r) => r.method === method && r.path.test(path)) || isQueueRoute(method, path);
}

/** Whether a path is a waiting-room operation (forwarded to the gateway's /v1/queue/...). */
export function isQueueRoute(method: string, path: string): boolean {
  return QUEUE_ROUTES.some((r) => r.method === method && r.path.test(path));
}

/** The query parameters the core reads (everything else is dropped). */
export function forwardQuery(search: URLSearchParams): string {
  const out = new URLSearchParams();
  for (const k of ["organiserId", "currency"]) {
    const v = search.get(k);
    if (v && /^[A-Za-z0-9-]{1,64}$/.test(v)) out.set(k, v);
  }
  const s = out.toString();
  return s ? `?${s}` : "";
}

/** Request headers passed on to the core. */
export function forwardHeaders(from: Headers, token: string): Headers {
  const h = new Headers({ Authorization: `Bearer ${token}`, Accept: "application/json" });
  const key = from.get("idempotency-key");
  if (key && /^[A-Za-z0-9_-]{8,100}$/.test(key)) h.set("Idempotency-Key", key);
  const link = from.get("x-order-token");
  if (link && link.length <= 400) h.set("X-Order-Token", link);
  if (from.get("content-type")?.startsWith("application/json")) h.set("Content-Type", "application/json");
  // Step 5: the waiting room's pass and queue token, the session secret they are bound to, and the
  // browser's user agent (a bot-defence signal).
  const pass = from.get("x-eps-pass");
  if (pass && pass.length <= 2048 && /^v1\.[A-Za-z0-9_.-]+$/.test(pass)) h.set("X-EPS-Pass", pass);
  const qt = from.get("x-eps-queue-token");
  if (qt && qt.length <= 2048 && /^v1\.[A-Za-z0-9_.-]+$/.test(qt)) h.set("X-EPS-Queue-Token", qt);
  const session = from.get("x-eps-session");
  if (session && /^[A-Za-z0-9_-]{16,200}$/.test(session)) h.set("X-EPS-Session", session);
  const ua = from.get("user-agent");
  if (ua) h.set("X-EPS-Client-UA", ua.slice(0, 400));
  return h;
}
