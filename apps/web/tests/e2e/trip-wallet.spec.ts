// Trip wallet spec: two organisers in their own brands, the empty state, a ticket id the fan does
// not hold, items issuing without a reload, and the Romanian wallet.
import { expect, expectNoHorizontalScroll, expectNotFoundResponse, setLocale, signInFreshUser, test } from "./fixtures";
import { buyGalaxia, buySunwave, walletTickets } from "./ticket-helpers";

test("Nothing held yet: an empty state, no error", async ({ page }) => {
  await signInFreshUser(page);
  await page.goto("/wallet");
  await expect(page.getByTestId("trip-empty")).toContainText("No tickets yet");
  await expectNoHorizontalScroll(page);
});

test("Two organisers: two event groups in their own brand colours, by date; issuing shown honestly", async ({ page }) => {
  test.setTimeout(90_000);
  await signInFreshUser(page);
  await buySunwave(page); // 21 August 2027
  await buyGalaxia(page); // 15 July 2027
  await page.goto("/wallet");
  // Items still issuing are shown as such, then turn into tickets without a reload.
  await expect(page.getByTestId("trip-item")).toHaveCount(2, { timeout: 30_000 });
  await expect(page.getByTestId("issuing")).toHaveCount(0);
  const groups = page.getByTestId("trip-event");
  await expect(groups).toHaveCount(2);
  await expect(groups.nth(0)).toContainText("Galaxia 2027");
  await expect(groups.nth(1)).toContainText("Sunwave Beach Day 2027");
  const colours = await page.getByTestId("trip-event-header").evaluateAll((els) => els.map((e) => getComputedStyle(e).backgroundColor));
  expect(colours[0]).not.toBe(colours[1]);
  await expect(groups.nth(0)).toHaveAttribute("data-brand", "nibiru");
  await expect(groups.nth(1)).toHaveAttribute("data-brand", "sunwave-festival");
  // The header names the most urgent action: Galaxia's pass needs a name.
  await expect(page.getByTestId("trip-header-action")).toHaveAttribute("data-kind", "name");
  await expectNoHorizontalScroll(page);
});

test("Another fan's ticket id in the URL: not found", async ({ page, browser }) => {
  const ctx = await browser.newContext();
  const other = await ctx.newPage();
  await signInFreshUser(other);
  await buyGalaxia(other);
  const [id] = await walletTickets(other);
  await ctx.close();
  await signInFreshUser(page);
  expectNotFoundResponse(page);
  const res = await page.goto(`/wallet/tickets/${id}`);
  expect(res?.status()).toBe(404);
  await expect(page.getByTestId("live-ticket")).toHaveCount(0);
});

test("Romanian wallet: labels, actions and deadlines in Romanian with diacritics", async ({ page }) => {
  await setLocale(page, "ro");
  await signInFreshUser(page);
  await buyGalaxia(page, { locale: "ro" });
  await walletTickets(page);
  await expect(page.getByRole("heading", { name: "Biletele mele" })).toBeVisible();
  await expect(page.getByTestId("trip-item-action")).toContainText("Completează numele până pe");
  await expect(page.getByTestId("trip-item-action")).toContainText("iulie 2027");
  await expect(page.getByTestId("trip-item-state")).toHaveText("Fără nume");
  await expectNoHorizontalScroll(page);
});
