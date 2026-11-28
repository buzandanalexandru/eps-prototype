// Storefront spec: the published NIBIRU sample shop (read only, so it runs on desktop and phone).
import { desktopOnly, expect, expectErrorResponse, expectNoHorizontalScroll, expectNotFoundResponse, onboard, setLocale, signInFreshUser, test } from "./fixtures";

const GALAXIA = "/s/nibiru/en/e/galaxia-2027";

test("a visitor whose browser prefers Romanian lands on the Romanian shop, labelled sample data and not indexed", async ({ browser }) => {
  const ctx = await browser.newContext({ locale: "ro-RO" });
  const page = await ctx.newPage();
  const res = await page.goto("/s/nibiru");
  expect(res?.status()).toBe(200);
  await expect(page).toHaveURL(/\/s\/nibiru\/ro$/);
  await expect(page.locator("html")).toHaveAttribute("lang", "ro");
  await expect(page.getByTestId("shop-sample-banner")).toHaveText("Date de exemplu");
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute("content", /noindex/);
  await expect(page.getByText("preț final, taxe incluse").first()).toBeVisible();
  await expect(page.getByTestId("ticket-price").first()).toHaveText("552,48 RON");
  expect(res?.headers()["content-security-policy"]).toContain("frame-ancestors 'none'");
  expect(res?.headers()["x-frame-options"]).toBe("DENY");
  // Every image and request stays on our own origin.
  for (const src of await page.locator("img").evaluateAll((els) => els.map((e) => (e as HTMLImageElement).src))) expect(new URL(src).origin).toBe(new URL(page.url()).origin);
  await page.getByTestId("shop-lang-en").click();
  await expect(page).toHaveURL(/\/s\/nibiru\/en$/);
  await expect(page.getByTestId("shop-sample-banner")).toHaveText("Sample data");
  await ctx.close();
});

test("an unknown brand and a brand with only a draft give the same 404", async ({ page }, info) => {
  test.skip(desktopOnly(info.project.name), "creates data");
  await setLocale(page, "en");
  await signInFreshUser(page);
  const { brandSlug } = await onboard(page);
  expectNotFoundResponse(page);
  const draft = await page.goto(`/s/${brandSlug}/en`);
  const draftText = await page.locator("body").innerText();
  const unknown = await page.goto("/s/no-such-brand-xyz/en");
  const unknownText = await page.locator("body").innerText();
  expect(draft?.status()).toBe(404);
  expect(unknown?.status()).toBe(404);
  expect(draftText).toBe(unknownText);
  expect((await page.goto(`/s/${brandSlug}`))?.status()).toBe(404);
});

test("the event page shows dates, venue, line-up and tickets from the live snapshot in the brand's look", async ({ page }) => {
  await page.goto(GALAXIA);
  await expect(page.getByTestId("event-summary").locator("h1")).toHaveText("Galaxia 2027");
  await expect(page.getByTestId("event-dates")).toHaveText("15 July 2027 – 18 July 2027");
  await expect(page.getByTestId("event-venue")).toHaveText("NIBIRU Arena");
  await expect(page.locator("[data-eps-type=lineup]")).toContainText("Nova Drift");
  await expect(page.getByTestId("ticket-card")).toHaveCount(6);
  await expect(page.getByTestId("shop-tickets")).toHaveAttribute("data-availability", "ok");
  const primary = await page.getByTestId("storefront").evaluate((el) => getComputedStyle(el).getPropertyValue("--eps-l-primary").trim());
  expect(primary).toMatch(/^#[0-9a-f]{6}$/i);
  await expectNoHorizontalScroll(page);
});

test("a product not yet on sale says when, and cannot be added", async ({ page }) => {
  await page.route("**/api/public/events/*/availability", async (route) => {
    const res = await route.fetch();
    const body = await res.json();
    body.products[0] = { ...body.products[0], status: "not_yet", opensAt: "2027-02-01T08:00:00Z" };
    await route.fulfill({ response: res, json: body });
  });
  await page.goto(GALAXIA);
  const ga = page.getByTestId("ticket-card").first();
  await expect(ga.getByTestId("ticket-status")).toHaveText("On sale from 1 Feb 2027");
  await expect(ga.getByTestId("plus")).toBeDisabled();
  await expect(page.getByTestId("ticket-card").nth(1).getByTestId("plus")).toBeEnabled();
});

test("when the core cannot be reached the page keeps its prices and pauses buying, without console errors", async ({ page }) => {
  expectErrorResponse(page, 503);
  await page.route("**/api/public/events/*/availability", (route) => route.fulfill({ status: 503, contentType: "application/problem+json", body: '{"code":"unavailable"}' }));
  await page.goto(GALAXIA);
  await expect(page.getByTestId("availability-failed")).toHaveText("Tickets are temporarily unavailable, try again shortly");
  await expect(page.getByTestId("ticket-price").first()).toHaveText("RON 552.48");
  await expect(page.getByTestId("plus").first()).toBeDisabled();
  await expect(page.getByTestId("continue")).toBeDisabled();
});

test("on a 390 px phone in English the ticket grid stacks without horizontal scroll", async ({ browser }) => {
  const ctx = await browser.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
  const page = await ctx.newPage();
  await setLocale(page, "en");
  await page.goto(GALAXIA);
  const xs = await page.getByTestId("ticket-card").evaluateAll((els) => els.map((e) => Math.round(e.getBoundingClientRect().left)));
  expect(new Set(xs).size).toBe(1);
  await expect(page.getByTestId("ticket-price").first()).toHaveText("RON 552.48");
  await expectNoHorizontalScroll(page);
  await ctx.close();
});
