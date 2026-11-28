// The door page's service worker (step 7, design D7): it keeps the door page and its scripts so a
// prepared phone reloads and scans with no network. API calls always go to the network (scans are
// stored in IndexedDB by the page and uploaded when online). Scope: /door/.
const CACHE = "eps-door-v1";

self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (e) => e.waitUntil(self.clients.claim()));

// The page sends the address of itself and of the scripts and styles it loaded.
self.addEventListener("message", (e) => {
  const d = e.data || {};
  if (d.type !== "cache" || !Array.isArray(d.urls)) return;
  e.waitUntil(
    caches.open(CACHE).then((c) =>
      Promise.all(
        d.urls
          .filter((u) => typeof u === "string" && new URL(u, self.location.origin).origin === self.location.origin)
          .map((u) =>
            fetch(u, { credentials: "same-origin" })
              .then((r) => (r.ok ? c.put(new URL(u, self.location.origin).pathname + (new URL(u, self.location.origin).search || ""), r) : null))
              .catch(() => null),
          ),
      ),
    ),
  );
});

self.addEventListener("fetch", (e) => {
  const req = e.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin || url.pathname.startsWith("/api/")) return;
  if (req.mode === "navigate" && url.pathname.startsWith("/door/")) {
    // The page: from the network when online (and kept), else the kept copy.
    e.respondWith(
      fetch(req)
        .then((r) => {
          if (r.ok) {
            const copy = r.clone();
            caches.open(CACHE).then((c) => c.put(url.pathname, copy));
          }
          return r;
        })
        .catch(() => caches.match(url.pathname).then((hit) => hit || Response.error())),
    );
    return;
  }
  if (url.pathname.startsWith("/_next/static/") || url.pathname.startsWith("/samples/") || url.pathname === "/favicon.ico") {
    e.respondWith(
      caches.match(url.pathname).then(
        (hit) =>
          hit ||
          fetch(req).then((r) => {
            if (r.ok) {
              const copy = r.clone();
              caches.open(CACHE).then((c) => c.put(url.pathname, copy));
            }
            return r;
          }),
      ),
    );
  }
});
