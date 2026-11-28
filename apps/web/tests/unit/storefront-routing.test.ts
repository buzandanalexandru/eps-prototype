// Storefront spec: shop addresses, languages and brand hostnames (design D3.5).
import { describe, expect, it } from "vitest";
import { NextRequest } from "next/server";
import { proxy } from "@/proxy";
import { hostRewrite, keyFromParam, pickShopLocale, shopBase, shopLocaleOfPath } from "@/lib/storefront/routing";
import { loadPublishedShop } from "@/lib/storefront/published";
import { publishDraft } from "@/lib/storefront/domain";
import { noSalesBeforeFirstPublish, snapshotHook } from "@/lib/snapshot/publish";
import { asOwner, describeDb, tx } from "./db";
import { shopOrganiser } from "./storefront-fixtures";

function call(host: string, path: string, headers: Record<string, string> = {}) {
  return proxy(new NextRequest(`http://${host}${path}`, { headers: { host, ...headers } }));
}

describe("shop routing", () => {
  it("a language the shop does not sell redirects to one it does: cookie, then browser, then the default", () => {
    expect(pickShopLocale({ cookie: "en", acceptLanguage: "en-GB,en;q=0.9", enabled: ["ro"], fallback: "ro" })).toBe("ro");
    expect(pickShopLocale({ cookie: undefined, acceptLanguage: "ro-RO,ro;q=0.9,en;q=0.5", enabled: ["ro", "en"], fallback: "en" })).toBe("ro");
    expect(pickShopLocale({ cookie: "en", acceptLanguage: "ro", enabled: ["ro", "en"], fallback: "ro" })).toBe("en");
    expect(pickShopLocale({ cookie: undefined, acceptLanguage: "de-DE,fr;q=0.8", enabled: ["ro", "en"], fallback: "en" })).toBe("en");
    expect(pickShopLocale({ cookie: undefined, acceptLanguage: "en;q=0, ro;q=0.1", enabled: ["en", "ro"], fallback: "en" })).toBe("ro");
  });

  it("a brand hostname serves the shop at its root, with links on the hostname", () => {
    const res = call("tickets.sunwave.test", "/en");
    expect(res.headers.get("x-middleware-rewrite")).toBe("http://tickets.sunwave.test/s/~tickets.sunwave.test/en");
    expect(res.headers.get("content-security-policy")).toContain("frame-ancestors 'none'");
    expect(keyFromParam("~tickets.sunwave.test")).toEqual({ host: "tickets.sunwave.test" });
    expect(shopBase({ host: "tickets.sunwave.test" }, "en")).toBe("/en");
    expect(shopBase({ slug: "sunwave-festival" }, "en")).toBe("/s/sunwave-festival/en");
    expect(shopLocaleOfPath("/s/~tickets.sunwave.test/en/e/x")).toBe("en");
    // Assets, the app's files, its APIs and the platform's legal pages are served as they are.
    for (const p of ["/assets/abc", "/samples/logo-sunwave.svg", "/_next/static/x.js", "/api/public/events/x/availability", "/fees", "/terms"]) expect(hostRewrite(p, "h.test"), p).toBeNull();
    // Studio is not served on a brand host: it is just another (unknown) shop page there.
    expect(hostRewrite("/studio", "h.test")).toBe("/s/~h.test/studio");
  });

  it("platform hosts route by path, and a client cannot claim a brand host", () => {
    const res = call("localhost:3000", "/s/~evil.test/en", { "x-eps-brand-host": "evil.test" });
    expect(res.headers.get("x-middleware-rewrite")).toBeNull();
    // The forwarded request headers never carry the client's value.
    expect(res.headers.get("x-middleware-request-x-eps-brand-host")).toBeNull();
    expect(res.headers.get("x-middleware-request-x-eps-shop-locale")).toBe("en");
  });
});

describeDb("shop lookup by hostname", () => {
  it("a configured hostname gives the same shop as the slug; an unknown host gives nothing", async () => {
    const o = await shopOrganiser();
    await tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: 1, userId: null }, [snapshotHook({ usage: noSalesBeforeFirstPublish, userId: null })]));
    const host = `shop-${o.organiserId.slice(0, 8)}.example.test`;
    await asOwner((c) => c.query("insert into control.brand_hostnames (hostname, organiser_id, brand_id) values ($1, $2, $3)", [host, o.organiserId, o.brandId]));
    const slug = (await tx(o.scope, (c) => c.query("select slug from control.brands where id = $1", [o.brandId]))).rows[0].slug;
    const byHost = await tx({}, (c) => loadPublishedShop(c, { host: host.toUpperCase() }));
    const bySlug = await tx({}, (c) => loadPublishedShop(c, { slug }));
    expect(byHost).not.toBeNull();
    expect(byHost).toEqual(bySlug);
    expect(byHost!.hostnames).toEqual([host]);
    expect(await tx({}, (c) => loadPublishedShop(c, { host: "unknown.example.test" }))).toBeNull();
  });

  it("a brand with only a draft and a brand that does not exist both give nothing", async () => {
    const o = await shopOrganiser();
    const slug = (await tx(o.scope, (c) => c.query("select slug from control.brands where id = $1", [o.brandId]))).rows[0].slug;
    expect(await tx({}, (c) => loadPublishedShop(c, { slug }))).toBeNull();
    expect(await tx({}, (c) => loadPublishedShop(c, { slug: "no-such-brand-anywhere" }))).toBeNull();
    expect(await tx({}, (c) => loadPublishedShop(c, { slug: "../../etc" }))).toBeNull();
  });
});
