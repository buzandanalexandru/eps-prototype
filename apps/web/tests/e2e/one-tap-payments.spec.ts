// Payments spec "Simulated Apple Pay and Google Pay" and "Saved cards kept by the provider": a
// one-tap wallet sheet at the provider (no card page), and a provider-saved card that pays the next
// order in one tap.
import { expect, signInFreshUser, test } from "./fixtures";
import { expectOrderState, giveDetails, holdGalaxia, pressPay } from "./checkout-helpers";

test("Apple Pay (simulated): a wallet sheet, no card page, the order paid with apple_pay", async ({ page }) => {
  await holdGalaxia(page);
  await giveDetails(page);
  await expect(page.getByTestId("method-apple_pay")).toContainText("Apple Pay (simulated)");
  await pressPay(page, "apple_pay");
  await expect(page.getByTestId("psp-page")).toHaveAttribute("data-method", "apple_pay");
  await expect(page.getByTestId("psp-sheet")).toBeVisible();
  await expect(page.getByTestId("psp-approve")).toHaveCount(0);
  await page.getByTestId("psp-wallet-pay").click();
  await expectOrderState(page, /^(paid|issued)$/);
});

test("Google Pay (simulated) completes the same way", async ({ page }) => {
  await holdGalaxia(page);
  await giveDetails(page);
  await pressPay(page, "google_pay");
  await expect(page.getByTestId("psp-page")).toHaveAttribute("data-method", "google_pay");
  await page.getByTestId("psp-wallet-pay").click();
  await expectOrderState(page, /^(paid|issued)$/);
});

test("a card saved with the provider pays the next order in one tap", async ({ page }) => {
  test.setTimeout(120_000);
  await signInFreshUser(page);
  await holdGalaxia(page);
  await giveDetails(page);
  await page.getByTestId("method-hosted_card").locator("input").check();
  await page.getByTestId("save-card").check();
  await page.getByTestId("pay").click();
  await page.getByTestId("psp-approve").click();
  await expectOrderState(page, /^(paid|issued)$/);
  // The next order: the saved card is offered and pays without any provider page.
  await holdGalaxia(page);
  const saved = page.getByTestId("method-saved_card");
  await expect(saved).toContainText("Test Visa •••• 4242 (saved)");
  await giveDetails(page);
  await saved.locator("input").check();
  await page.getByTestId("pay").click();
  await expectOrderState(page, /^(paid|issued)$/);
  expect(new URL(page.url()).pathname).toBe("/s/nibiru/en/checkout");
});
