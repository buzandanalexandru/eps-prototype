// The shop's availability route: our own origin in front of the core's public answer.
import { afterEach, describe, expect, it, vi } from "vitest";
import { GET } from "@/app/api/public/events/[eventId]/availability/route";

const EVENT = "3dc5d5b1-2453-5e15-886e-b4b6eedcc859";
const call = (q = "") => GET(new Request(`http://shop.test/api/public/events/${EVENT}/availability${q}`), { params: Promise.resolve({ eventId: EVENT }) });

describe("availability route", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("refuses a query string, so nobody can vary past the CDN's copy", async () => {
    const res = await call("?x=1");
    expect(res.status).toBe(400);
    expect(res.headers.get("cache-control")).toBe("no-store");
  });

  it("caches good answers briefly with stale fallbacks, failures 5 s, and never a 404 publicly", async () => {
    const body = { eventId: EVENT, version: 1, generatedAt: "2026-12-05T10:00:00Z", products: [], instances: [{ instanceId: `${EVENT}@2027-07-15`, status: "sold_out", remaining: 0 }] };
    vi.stubGlobal("fetch", async () => Response.json(body));
    const ok = await call();
    expect(ok.headers.get("cache-control")).toBe("public, max-age=2, s-maxage=2, stale-while-revalidate=10, stale-if-error=60");
    expect((await ok.json()).instances).toEqual(body.instances);
    vi.stubGlobal("fetch", async () => new Response("{}", { status: 404 }));
    expect((await call()).headers.get("cache-control")).toBe("no-store");
    vi.stubGlobal("fetch", async () => {
      throw new Error("down");
    });
    const down = await call();
    expect(down.status).toBe(503);
    expect(down.headers.get("cache-control")).toBe("public, max-age=5, s-maxage=5");
    // An answer for another event, or of another shape, is not passed on.
    vi.stubGlobal("fetch", async () => Response.json({ ...body, eventId: "0e000000-0000-4000-8000-000000000001" }));
    expect((await call()).status).toBe(503);
  });
});
