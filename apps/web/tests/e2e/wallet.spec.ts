// Fan-wallet spec: "Top-up confirmed", "One-tap during a drop" and "Label shown" (RO and EN).
import { expect, expectNoHorizontalScroll, setLocale, signInFreshUser, test } from "./fixtures";
import { expectOrderState, giveDetails, holdGalaxia } from "./checkout-helpers";

test("top up 600 RON, pay a 552.48 RON order from the wallet in one step", async ({ page }) => {
  test.setTimeout(120_000);
  await signInFreshUser(page);
  await page.goto("/s/nibiru/en/wallet");
  await expect(page.getByTestId("wallet-notice")).toHaveText("Simulated balance, no real money");
  await expect(page.getByTestId("wallet-balance")).toHaveText("RON 0.00");
  await page.getByTestId("topup-amount").fill("600");
  await page.getByTestId("topup-submit").click();
  await expect(page.getByTestId("psp-page")).toBeVisible();
  await page.getByTestId("psp-approve").click();
  await expect(page.getByTestId("topup-status")).toHaveAttribute("data-state", "succeeded", { timeout: 30_000 });
  await expect(page.getByTestId("wallet-balance")).toHaveText("RON 600.00");
  await expect(page.getByTestId("wallet-move").first()).toHaveAttribute("data-kind", "topup");
  await expectNoHorizontalScroll(page);
  const code = await page.getByTestId("wallet-code").textContent();
  expect(code).toMatch(/^[A-Z0-9]{4}-[A-Z0-9]{4}$/);
  await page.getByTestId("pay-code-show").click();
  await expect(page.getByTestId("pay-code")).toHaveText(/^\d{6}$/);

  await holdGalaxia(page);
  await giveDetails(page);
  await expect(page.getByTestId("method-wallet")).toContainText("EPS Wallet (RON 600.00)");
  await page.getByTestId("method-wallet").locator("input").check();
  await page.getByTestId("pay").click();
  await expectOrderState(page, /^(paid|issued)$/);
  expect(new URL(page.url()).pathname).toBe("/s/nibiru/en/checkout");
  await page.goto("/s/nibiru/en/wallet");
  await expect(page.getByTestId("wallet-balance")).toHaveText("RON 47.52");
});

test("the simulated-money notice in Romanian, and a sign-in prompt for guests", async ({ page }) => {
  await setLocale(page, "ro");
  await page.goto("/s/nibiru/ro/wallet");
  await expect(page.getByTestId("wallet-notice")).toHaveText("Sold simulat, fără bani reali");
  await expect(page.getByTestId("wallet-signin")).toContainText("Autentifică-te pentru a folosi portofelul");
  await expectNoHorizontalScroll(page);
});
