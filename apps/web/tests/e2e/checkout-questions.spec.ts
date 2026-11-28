// Checkout spec "Checkout questions and consents" (scenario "Required consent missing"): an
// organiser adds a required consent in Studio; after the publish the checkout asks it and payment
// does not start until it is ticked; the answer is kept with the order.
import { desktopOnly, expect, onboard, setLocale, signInFreshUser, test, uniqueSlug } from "./fixtures";
import { openBuilder } from "./builder-helpers";
import { createSellableEvent } from "./shop-helpers";
import { expectOrderState, giveDetails } from "./checkout-helpers";

test("a required consent blocks payment until it is ticked", async ({ page, browser }, info) => {
  test.skip(desktopOnly(info.project.name), "creates data");
  test.setTimeout(240_000);
  await setLocale(page, "en");
  await signInFreshUser(page);
  const { brandSlug, brandUrl } = await onboard(page, { brand: "Questions Test" });
  const eventSlug = uniqueSlug("q");
  await createSellableEvent(page, eventSlug);
  await page.goto(`${brandUrl}/questions`);
  const f = page.getByTestId("question-form");
  await f.getByRole("combobox").selectOption("consent");
  await f.locator("input[name=label_ro]").fill("Accept regulamentul festivalului");
  await f.locator("input[name=label_en]").fill("I accept the festival rules");
  await f.locator("input[name=required]").check();
  await f.getByRole("button").click();
  await expect(page.getByTestId("question-added")).toBeVisible();
  await expect(page.getByTestId("question-row")).toHaveCount(1);
  await openBuilder(page);
  await page.getByTestId("tab-publish").click();
  await page.getByTestId("publish").click();
  await expect(page.getByTestId("publish-message")).toHaveText("Version 1 is live.");

  const fan = await (await browser.newContext()).newPage();
  await fan.goto(`/s/${brandSlug}/en/e/${eventSlug}`);
  await expect(fan.getByTestId("shop-tickets")).toHaveAttribute("data-availability", "ok");
  await fan.getByTestId("ticket-card").first().getByTestId("plus").click();
  await fan.getByTestId("continue").click();
  await expect(fan.getByTestId("order-state")).toHaveAttribute("data-state", "held");
  await expect(fan.getByTestId("consents")).toContainText("I accept the festival rules (required)");
  await giveDetails(fan);
  await fan.getByTestId("method-hosted_card").locator("input").check();
  await fan.getByTestId("pay").click();
  await expect(fan.getByTestId("consent-needed")).toHaveText("Tick this to continue");
  await expect(fan.getByTestId("psp-page")).toHaveCount(0);
  await expectOrderState(fan, "held");
  await fan.getByTestId("consents").locator("input[type=checkbox]").check();
  await fan.getByTestId("pay").click();
  await expect(fan.getByTestId("psp-page")).toBeVisible();
  await fan.context().close();
});
