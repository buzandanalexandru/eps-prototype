// The shop's own-origin route to the core (step 4): only the listed checkout, order, payment and
// wallet paths are forwarded, with only the headers and query parameters the core reads.
import { describe, expect, it } from "vitest";
import { allowedShopRoute, forwardHeaders, forwardQuery } from "@/lib/shop/proxy";

const ID = "0d000000-0000-4000-8000-0000000000b1";

describe("shop proxy", () => {
  it("forwards only the shop's own operations", () => {
    expect(allowedShopRoute("POST", "orders")).toBe(true);
    expect(allowedShopRoute("GET", `orders/${ID}`)).toBe(true);
    expect(allowedShopRoute("POST", `orders/${ID}/payments`)).toBe(true);
    expect(allowedShopRoute("POST", `wallets/${ID}/topups`)).toBe(true);
    expect(allowedShopRoute("DELETE", `orders/${ID}`)).toBe(false);
    expect(allowedShopRoute("GET", "../organiser/payments/providers")).toBe(false);
    expect(allowedShopRoute("PUT", "organiser/payments/providers/a/knobs")).toBe(false);
    expect(allowedShopRoute("GET", `orders/${ID}/../../x`)).toBe(false);
  });

  it("keeps the idempotency key and order link, drops everything else", () => {
    const h = forwardHeaders(new Headers({ "Idempotency-Key": "abcdefgh-1234", "X-Order-Token": "t.s", Cookie: "secret", "Content-Type": "application/json", Authorization: "Bearer evil" }), "tok");
    expect(h.get("authorization")).toBe("Bearer tok");
    expect(h.get("idempotency-key")).toBe("abcdefgh-1234");
    expect(h.get("x-order-token")).toBe("t.s");
    expect(h.get("cookie")).toBeNull();
    expect(forwardHeaders(new Headers({ "Idempotency-Key": "bad key!" }), "t").get("idempotency-key")).toBeNull();
    expect(forwardQuery(new URLSearchParams(`organiserId=${ID}&currency=RON&other=1`))).toBe(`?organiserId=${ID}&currency=RON`);
    expect(forwardQuery(new URLSearchParams("organiserId=a%26b"))).toBe("");
  });
});
