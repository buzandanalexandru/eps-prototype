// Payments spec "Automatic failover" (scenario "A fails, B pays") end to end: a fresh organiser
// switches on provider A's outage in Studio; a fan pays with a card and the payment completes on
// provider B without an error; the status page tells the truth. Knobs and breakers are per
// organiser (each organiser is its own merchant at the simulated providers), so this outage
// touches no other test.
import { desktopOnly, expect, onboard, setLocale, signInFreshUser, test, uniqueSlug } from "./fixtures";
import { openBuilder } from "./builder-helpers";
import { createSellableEvent } from "./shop-helpers";
import { expectOrderState, giveDetails } from "./checkout-helpers";

test("provider A down: the fan pays on B and the order is paid", async ({ page, browser }, info) => {
  test.skip(desktopOnly(info.project.name), "creates data");
  test.setTimeout(240_000);
  await setLocale(page, "en");
  await signInFreshUser(page);
  const { brandSlug } = await onboard(page, { brand: "Failover Test" });
  const eventSlug = uniqueSlug("fail");
  await createSellableEvent(page, eventSlug);
  await openBuilder(page);
  await page.getByTestId("tab-publish").click();
  await page.getByTestId("publish").click();
  await expect(page.getByTestId("publish-message")).toHaveText("Version 1 is live.");

  // Studio: provider A's outage on (503 at once).
  await page.goto("/studio/payments");
  const a = page.getByTestId("knobs-a");
  await a.getByTestId("knob-outage").check();
  await a.getByRole("combobox").selectOption("503");
  await a.getByTestId("knobs-save").click();
  await expect(page.getByTestId("knobs-saved")).toHaveText("Provider A saved.");
  await expect(page.getByTestId("knob-outage").first()).toBeChecked();

  // A fan buys.
  const fan = await (await browser.newContext()).newPage();
  await fan.goto(`/s/${brandSlug}/en/e/${eventSlug}`);
  await expect(fan.getByTestId("shop-tickets")).toHaveAttribute("data-availability", "ok");
  await fan.getByTestId("ticket-card").first().getByTestId("plus").click();
  await fan.getByTestId("continue").click();
  await expect(fan.getByTestId("order-state")).toHaveText("held");
  await giveDetails(fan);
  await fan.getByTestId("method-hosted_card").locator("input").check();
  await fan.getByTestId("pay").click();
  await expect(fan.getByTestId("psp-page")).toBeVisible();
  expect(new URL(fan.url()).pathname).toMatch(/^\/b\/pay\//);
  await expect(fan.getByRole("heading", { level: 1 })).toHaveText("Simulated payment provider B");
  await fan.getByTestId("psp-approve").click();
  await expectOrderState(fan, /^(paid|issued)$/);
  await expect(fan.getByTestId("checkout-error")).toHaveCount(0);
  await expect(fan.getByTestId("order-paid")).toBeVisible();

  // Back to normal.
  await page.goto("/studio/payments");
  await page.getByTestId("knobs-a").getByTestId("knob-outage").uncheck();
  await page.getByTestId("knobs-a").getByTestId("knobs-save").click();
  await expect(page.getByTestId("knobs-saved")).toBeVisible();
  await fan.context().close();
});
