// Step 6: the live dashboard's stream parsing, the Studio policy that lets only the live page reach
// the gateway, and the shop's reading of degraded mode.
import { NextRequest } from "next/server";
import { describe, expect, it } from "vitest";
import { parseEvents } from "@/lib/studio/sse";
import { studioConnectOrigins } from "@/lib/security-headers";
import { readAnswer } from "@/lib/storefront/availability-answer";
import { proxy } from "@/proxy";

const SALE = "0f000000-0000-4000-8000-000000000001";

describe("server-sent events", () => {
  it("splits blocks, skips comments and keeps a partial block for later", () => {
    const { events, rest } = parseEvents("retry: 2000\n\n: heartbeat\n\nevent: history\ndata: [1]\n\nevent: frame\nid: 5\ndata: {\"ts\":5}\n\nevent: fra");
    expect(events).toEqual([
      { event: "history", data: "[1]" },
      { event: "frame", data: '{"ts":5}' },
    ]);
    expect(rest).toBe("event: fra");
  });
});

describe("the live page may connect to the gateway, and only it", () => {
  it("allows the gateway origin on /studio/sales/<id>/live only", () => {
    expect(studioConnectOrigins(`/studio/sales/${SALE}/live`, "https://gw.prod.203.0.113.1.sslip.io")).toEqual(["https://gw.prod.203.0.113.1.sslip.io"]);
    expect(studioConnectOrigins(`/studio/sales/${SALE}/simulator`, "https://gw.example")).toEqual([]);
    expect(studioConnectOrigins("/studio/builder", "https://gw.example")).toEqual([]);
    expect(studioConnectOrigins(`/studio/sales/${SALE}/live`, "javascript:alert(1)")).toEqual([]);
    expect(studioConnectOrigins(`/studio/sales/${SALE}/live`, null)).toEqual([]);
  });
  it("puts it in the page's connect-src", () => {
    const res = proxy(new NextRequest(`http://localhost:3000/studio/sales/${SALE}/live`, { headers: { cookie: "better-auth.session_token=x", host: "localhost:3000" } }));
    const csp = res.headers.get("content-security-policy") ?? "";
    expect(csp).toMatch(/connect-src 'self' http:\/\/localhost:8081/);
    const other = proxy(new NextRequest("http://localhost:3000/studio/events", { headers: { cookie: "better-auth.session_token=x", host: "localhost:3000" } }));
    expect(other.headers.get("content-security-policy") ?? "").not.toMatch(/localhost:8081/);
  });
});

describe("degraded mode in the shop", () => {
  it("reads degraded from the availability answer", () => {
    const body = { version: 3, products: [{ productId: "p", status: "on_sale", allInMinor: 100, priceStepIndex: 0 }], instances: [], degraded: true };
    const load = readAnswer(body, 3);
    expect(load.state === "ok" && load.data.degraded).toBe(true);
    const normal = readAnswer({ ...body, degraded: undefined }, 3);
    expect(normal.state === "ok" && normal.data.degraded).toBeFalsy();
  });
});
