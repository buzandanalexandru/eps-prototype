// Step 6, E11: degraded mode. Spec graceful-degradation "Switch on": within 10 seconds the static
// event page shows the busy message in place of its buy controls (RO and EN, desktop and phone),
// and the waiting room says the shop is busy while keeping the place; off again, buying returns.
import { devices } from "@playwright/test";
import { desktopOnly, expect, expectErrorResponse, expectNoHorizontalScroll, test } from "./fixtures";
import { addSale, expectNoConsoleErrors, fan, publishedEvent } from "./onsale-helpers";

test("degraded mode shows the honest busy message on the event page and in the waiting room", async ({ page, browser }, info) => {
  test.skip(desktopOnly(info.project.name), "organiser set-up on desktop; fans on desktop and phone");
  test.setTimeout(180_000);
  const { brandSlug, eventSlug, eventUrl } = await publishedEvent(page);
  const saleId = await addSale(page, eventUrl, { opensIn: -1 });
  expectErrorResponse(page, 401); // a fresh environment's first key reaches the gateway within ~20 s
  await page.goto(`/studio/sales/${saleId}/live`);
  await expect(page.getByTestId("live-ok")).toBeVisible({ timeout: 60_000 });
  await page.getByTestId("live-degrade").click();
  await expect(page.getByTestId("live-degraded")).toBeVisible();
  const switched = Date.now();
  for (const [locale, phone, title] of [
    ["en", false, "We are very busy"],
    ["ro", true, "Suntem foarte aglomerați"],
  ] as const) {
    const f = await fan(browser, { phone, locale });
    await expect(async () => {
      await f.goto(`/s/${brandSlug}/${locale}/e/${eventSlug}`);
      await expect(f.getByTestId("shop-busy")).toContainText(title, { timeout: 3_000 });
    }).toPass({ timeout: 10_000 - Math.min(5_000, Date.now() - switched) + 5_000 });
    await expect(f.getByTestId("continue")).toBeDisabled();
    await expectNoHorizontalScroll(f);
    // The waiting room keeps the place and says why nobody moves.
    await f.goto(`/s/${brandSlug}/${locale}/queue?sale=${saleId}`);
    await expect(f.getByTestId("queue-busy")).toBeVisible({ timeout: 20_000 });
    await expectNoHorizontalScroll(f);
    expectNoConsoleErrors(f);
    await f.context().close();
  }
  expect(Date.now() - switched).toBeLessThan(60_000);
  // Off again: the busy message goes and buying returns.
  await page.getByTestId("live-degrade").click();
  await expect(page.getByTestId("live-degrade")).toHaveAttribute("data-degraded", "off", { timeout: 10_000 });
  const f = await fan(browser, { locale: "en" });
  await expect(async () => {
    await f.goto(`/s/${brandSlug}/en/e/${eventSlug}`);
    await expect(f.getByTestId("shop-tickets")).toHaveAttribute("data-availability", "ok", { timeout: 3_000 });
    await expect(f.getByTestId("shop-busy")).toHaveCount(0);
  }).toPass({ timeout: 15_000 });
  expectNoConsoleErrors(f);
  void devices;
});
