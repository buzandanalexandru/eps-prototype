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
  // Fans are ready before the switch, so the 10 seconds measure the switch, not their set-up
  // (review spec L5: asserted against switched + 10 s each way).
  const fans = [
    { locale: "en", title: "We are very busy", f: await fan(browser, { phone: false, locale: "en" }) },
    { locale: "ro", title: "Suntem foarte aglomerați", f: await fan(browser, { phone: true, locale: "ro" }) },
  ] as const;
  await page.getByTestId("live-degrade").click();
  await expect(page.getByTestId("live-degraded")).toBeVisible();
  const switched = Date.now();
  await Promise.all(
    fans.map(({ locale, title, f }) =>
      expect(async () => {
        await f.goto(`/s/${brandSlug}/${locale}/e/${eventSlug}`);
        await expect(f.getByTestId("shop-busy")).toContainText(title, { timeout: 1_000 });
      }).toPass({ timeout: Math.max(1_000, switched + 10_000 - Date.now()) }),
    ),
  );
  expect(Date.now(), "the busy message within 10 s of the switch").toBeLessThanOrEqual(switched + 10_000);
  for (const { locale, f } of fans) {
    await expect(f.getByTestId("continue")).toBeDisabled();
    await expectNoHorizontalScroll(f);
    // The waiting room keeps the place and says why nobody moves.
    await f.goto(`/s/${brandSlug}/${locale}/queue?sale=${saleId}`);
    await expect(f.getByTestId("queue-busy")).toBeVisible({ timeout: 20_000 });
    await expectNoHorizontalScroll(f);
    expectNoConsoleErrors(f);
  }
  await fans[1].f.context().close();
  // Off again: within 10 s the busy message goes and buying returns.
  const f = fans[0].f;
  await page.getByTestId("live-degrade").click();
  await expect(page.getByTestId("live-degrade")).toHaveAttribute("data-degraded", "off", { timeout: 5_000 });
  const off = Date.now();
  await expect(async () => {
    await f.goto(`/s/${brandSlug}/en/e/${eventSlug}`);
    await expect(f.getByTestId("shop-tickets")).toHaveAttribute("data-availability", "ok", { timeout: 1_000 });
    await expect(f.getByTestId("shop-busy")).toHaveCount(0);
  }).toPass({ timeout: Math.max(1_000, off + 10_000 - Date.now()) });
  expect(Date.now(), "buying back within 10 s of the switch").toBeLessThanOrEqual(off + 10_000);
  expectNoConsoleErrors(f);
  void devices;
});
