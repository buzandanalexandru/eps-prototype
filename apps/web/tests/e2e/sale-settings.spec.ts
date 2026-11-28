// Sale-settings spec (step 5, E1): an organiser adds a sale moment to a published event, saves its
// settings as a new version, changes the live dial during the sale, and reads the history.
import { desktopOnly, expect, expectNoHorizontalScroll, setLocale, test } from "./fixtures";
import { addSale, fillSale, publishedEvent } from "./onsale-helpers";

test("Save settings, change the rate live, read the history", async ({ page }, info) => {
  test.skip(desktopOnly(info.project.name), "creates data");
  test.setTimeout(300_000);
  const { eventUrl } = await publishedEvent(page);
  // Scenario "Save settings": opening in 2 hours, pre-queue 30 minutes before, 2,000 a minute,
  // 4 per order and 6 per person.
  const id = await addSale(page, eventUrl, { opensIn: 120, prequeueMinutes: 30, rate: 2000, capPerOrder: 4, capPerPerson: 6 });
  const sale = page.locator(`[data-sale-id="${id}"]`);
  await expect(sale.getByTestId("sale-version")).toContainText("Version 1");
  await expect(sale.getByTestId("sale-version")).toContainText("Before the waiting room");
  await expect(sale.getByTestId("sale-rate")).toHaveValue("2000");
  await expect(sale.getByTestId("sale-cap-order")).toHaveValue("4");
  await expect(sale.getByTestId("sale-cap-person")).toHaveValue("6");
  await expect(sale.getByTestId("sale-prequeue")).toHaveValue("30");
  await expect(sale.getByTestId("sale-link")).toHaveAttribute("href", new RegExp(`/queue\\?sale=${id}$`));

  // A value out of range is refused with the allowed range (scenario "Value out of range").
  await sale.getByTestId("sale-hold-minutes").fill("30");
  await sale.getByTestId("sale-save").click();
  await expect(page.getByTestId("sale-error")).toContainText("holdMinutes must be between 5 and 15");
  await expect(page.locator(`[data-sale-id="${id}"]`)).toHaveAttribute("data-version", "1");

  // The live dial: three changes are three versions.
  for (const [i, rate] of [500, 900, 1200].entries()) {
    const form = page.locator(`[data-sale-id="${id}"]`).getByTestId("sale-form");
    await fillSale(form, { rate });
    await form.getByTestId("sale-save").click();
    await expect(page.getByTestId("sale-saved")).toHaveText(`Saved as version ${i + 2}.`);
  }
  const now = page.locator(`[data-sale-id="${id}"]`);
  await expect(now.getByTestId("sale-rate")).toHaveValue("1200");
  await now.getByText("History").click();
  await expect(now.getByTestId("sale-history-row")).toHaveCount(4);
  await expect(now.getByTestId("sale-history-row").first()).toContainText("1200");
  await expectNoHorizontalScroll(page);

  // The same screen in Romanian at phone width.
  await setLocale(page, "ro");
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${eventUrl}/sales`);
  await expect(page.getByRole("heading", { name: "Momente de vânzare" })).toBeVisible();
  await expect(page.getByTestId("sale-version").first()).toContainText("Versiunea 4");
  await expectNoHorizontalScroll(page);
});
