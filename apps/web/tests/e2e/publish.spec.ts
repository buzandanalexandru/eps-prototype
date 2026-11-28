// Sale-snapshots and storefront specs: publishing makes the shop live with the event's snapshot;
// Studio shows the live version and unpublished catalogue changes; draft edits change nothing in
// the shop until the next publish, which the next request shows.
import { desktopOnly, expect, expectNoHorizontalScroll, onboard, setLocale, signInFreshUser, test, uniqueSlug } from "./fixtures";
import { openBuilder, saved } from "./builder-helpers";
import { createSellableEvent } from "./shop-helpers";

test("publish, edit the draft and the catalogue, publish again: the shop follows publishes only", async ({ page, browser }, info) => {
  test.skip(desktopOnly(info.project.name), "creates data");
  test.setTimeout(240_000);
  await setLocale(page, "en");
  await signInFreshUser(page);
  const { brandSlug } = await onboard(page, { brand: "Publish Test" });
  const eventSlug = uniqueSlug("fest");
  const { eventUrl, productUrl } = await createSellableEvent(page, eventSlug);
  await page.goto(eventUrl);
  await expect(page.getByTestId("publication-state")).toContainText("Not on sale yet");

  await openBuilder(page);
  await page.getByTestId("tab-publish").click();
  await page.getByTestId("publish").click();
  await expect(page.getByTestId("publish-message")).toHaveText("Version 1 is live.");

  const fan = await (await browser.newContext()).newPage();
  await setLocale(fan, "en");
  await fan.goto(`/s/${brandSlug}/en/e/${eventSlug}`);
  await expect(fan.getByTestId("ticket-price").first()).toHaveText("RON 552.48");
  await expect(fan.getByTestId("from-price")).toHaveText("from RON 552.48");
  await expect(fan.locator('meta[name="eps-storefront-version"]')).toHaveAttribute("content", "1");
  await fan.goto(`/s/${brandSlug}/en`);
  const heroV1 = await fan.locator("[data-eps-type=hero] h1").first().textContent();
  expect(heroV1).toBeTruthy();

  // Studio: version 1 live, nothing unpublished.
  await page.goto(eventUrl);
  await expect(page.getByTestId("live-snapshot")).toContainText("On sale: version 1");
  await expect(page.getByTestId("unpublished-changes")).toHaveCount(0);

  // A catalogue edit: Studio says so, the shop keeps the published price.
  await page.goto(productUrl);
  const ladder = page.getByTestId("ladder-form");
  await ladder.getByTestId("ladder-step").nth(0).getByLabel(/^Price/).fill("599");
  await ladder.getByRole("button", { name: "Save ladder" }).click();
  await expect(page.getByTestId("price-in-force")).toHaveText("RON 599.00");
  await page.goto(eventUrl);
  await expect(page.getByTestId("unpublished-changes")).toHaveText("Changes not published");

  // A draft edit of the hero title, saved but not published.
  await openBuilder(page);
  await page.getByTestId("preview-locale").selectOption("en");
  await page.locator("[data-testid=section-row][data-section-type=hero]").first().getByTestId("section-select").click();
  await page.getByTestId("field-heading").locator("input[data-locale=en]").fill("Brand new title");
  await page.getByTestId("field-heading").locator("input[data-locale=ro]").fill("Titlu nou");
  await saved(page);
  await fan.goto(`/s/${brandSlug}/en`);
  await expect(fan.locator("[data-eps-type=hero] h1").first()).toHaveText(heroV1!);
  await fan.goto(`/s/${brandSlug}/en/e/${eventSlug}`);
  await expect(fan.getByTestId("ticket-price").first()).toHaveText("RON 552.48");

  // Publish: the next request shows the new title and the new price, without a deploy.
  await page.getByTestId("tab-publish").click();
  await page.getByTestId("publish").click();
  await expect(page.getByTestId("publish-message")).toHaveText("Version 2 is live.");
  await fan.goto(`/s/${brandSlug}/en`);
  await expect(fan.locator("[data-eps-type=hero] h1").first()).toHaveText("Brand new title");
  await fan.goto(`/s/${brandSlug}/en/e/${eventSlug}`);
  await expect(fan.getByTestId("ticket-price").first()).toHaveText("RON 613.98");
  await expect(fan.locator('meta[name="eps-storefront-version"]')).toHaveAttribute("content", "2");
  await expectNoHorizontalScroll(fan);
  await page.goto(eventUrl);
  await expect(page.getByTestId("live-snapshot")).toContainText("On sale: version 2");
  await expect(page.getByTestId("unpublished-changes")).toHaveCount(0);
  await fan.context().close();
});
