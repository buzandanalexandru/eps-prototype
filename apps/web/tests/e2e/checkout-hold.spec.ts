// Checkout spec "Visible countdown and automatic release" (scenario "Countdown follows the server")
// and the hold hand-off from the shop: the timer is computed from the core's deadline and clock,
// so a device clock that is minutes off still shows the true time left.
import { expect, expectNoHorizontalScroll, setLocale, test } from "./fixtures";
import { GALAXIA, holdGalaxia } from "./checkout-helpers";

test("the hold timer follows the server's deadline, not the device clock or page load", async ({ page, browser }) => {
  const id = await holdGalaxia(page);
  const left = Number(await page.getByTestId("hold-timer").getAttribute("data-seconds"));
  expect(left).toBeGreaterThan(7 * 60);
  expect(left).toBeLessThanOrEqual(8 * 60);
  const resume = await page.getByTestId("resume-link").inputValue();
  // The same order opened on a device whose clock runs 2 minutes ahead.
  const ctx = await browser.newContext();
  const other = await ctx.newPage();
  await other.clock.install({ time: new Date(Date.now() + 2 * 60_000) });
  await other.goto(resume);
  await expect(other.getByTestId("order")).toHaveAttribute("data-order-id", id);
  const there = Number(await other.getByTestId("hold-timer").getAttribute("data-seconds"));
  expect(Math.abs(there - left)).toBeLessThan(20);
  await ctx.close();
});

test("Romanian checkout: the hold, lines and total in Romanian; reload holds once", async ({ page }) => {
  await setLocale(page, "ro");
  await holdGalaxia(page, { locale: "ro", qty: 2 });
  await expect(page.getByRole("heading", { level: 1 })).toHaveText("Finalizează comanda");
  await expect(page.getByTestId("checkout-total")).toHaveText("1.104,96 RON");
  await expect(page.getByTestId("hold-timer")).toContainText("Biletele sunt rezervate încă");
  await expectNoHorizontalScroll(page);
  const url = page.url();
  await page.reload();
  expect(page.url()).toBe(url);
  await expect(page.getByTestId("order-state")).toHaveText("held");
});

test("a second hand-off for the same sale returns the order already held", async ({ page }) => {
  const first = await holdGalaxia(page);
  const second = await holdGalaxia(page, { qty: 2 });
  expect(second).toBe(first);
  await page.goto(GALAXIA());
});
