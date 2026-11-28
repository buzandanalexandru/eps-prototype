// Storefront spec "The same shop on a configured hostname", end to end through the proxy: the
// browser resolves the seeded sample hostname (shop.sunwave.test) and an unknown one to the local
// server. Local runs only (a deployment cannot be reached under those names).
import { expect, expectNoHorizontalScroll, expectNotFoundResponse, test } from "./fixtures";

const PORT = new URL(process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000").port || "3000";
test.use({ launchOptions: { args: ["--host-resolver-rules=MAP shop.sunwave.test 127.0.0.1, MAP unknown-brand.test 127.0.0.1"] } });

test.describe("brand hostname", () => {
  test.skip(!!process.env.PLAYWRIGHT_BASE_URL, "local only: needs the test hostnames to resolve to this server");

  test("the shop answers at the root of its hostname and its links stay there", async ({ page }) => {
    const res = await page.goto(`http://shop.sunwave.test:${PORT}/en`);
    expect(res?.status()).toBe(200);
    await expect(page.getByTestId("storefront")).toContainText("Sunwave");
    await expect(page.getByTestId("shop-sample-banner")).toHaveText("Sample data");
    const hrefs = await page.locator("a[href^='/']").evaluateAll((els) => els.map((e) => e.getAttribute("href")!));
    expect(hrefs.length).toBeGreaterThan(0);
    for (const h of hrefs.filter((x) => !["/fees", "/terms"].includes(x))) expect(h, h).not.toMatch(/^\/s\//);
    await expect(page.getByTestId("shop-lang-ro")).toHaveAttribute("href", "/ro");
    // An event page on the host, with live availability through the host's own /api.
    const event = await page.locator("nav a[href^='/en/e/']").first().getAttribute("href");
    await page.goto(`http://shop.sunwave.test:${PORT}${event}`);
    await expect(page.getByTestId("shop-tickets")).toHaveAttribute("data-availability", "ok");
    await expect(page.getByTestId("ticket-price").first()).toContainText("EUR");
    await expectNoHorizontalScroll(page);
    // The root redirects to a language.
    await page.goto(`http://shop.sunwave.test:${PORT}/`);
    await expect(page).toHaveURL(new RegExp(`shop\\.sunwave\\.test:${PORT}/(ro|en)$`));
  });

  test("an unknown hostname gets the platform 404 and no brand content", async ({ page }) => {
    expectNotFoundResponse(page);
    const res = await page.goto(`http://unknown-brand.test:${PORT}/en`);
    expect(res?.status()).toBe(404);
    await expect(page.getByTestId("storefront")).toHaveCount(0);
  });
});
