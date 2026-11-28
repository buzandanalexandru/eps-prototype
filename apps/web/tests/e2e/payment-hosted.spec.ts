// Payments spec "No card data in EPS" (scenario "Hosted test outcomes"): card payment happens on
// the simulated provider's page, which offers test outcomes and no card number field.
import { expect, expectNoHorizontalScroll, test } from "./fixtures";
import { expectOrderState, giveDetails, holdGalaxia, pressPay } from "./checkout-helpers";

test("card on the provider's page: test outcomes, no card field, back to a paid order", async ({ page }) => {
  await holdGalaxia(page);
  await giveDetails(page);
  await pressPay(page, "hosted_card");
  const psp = page.getByTestId("psp-page");
  for (const id of ["psp-approve", "psp-decline-funds", "psp-decline-3ds", "psp-cancel"]) await expect(page.getByTestId(id)).toBeVisible();
  await expect(psp.locator("input")).toHaveCount(0);
  await expect(page.getByTestId("psp-amount")).toHaveText("RON 552.48");
  await expectNoHorizontalScroll(page);
  await page.getByTestId("psp-approve").click();
  await expect(page).toHaveURL(/\/s\/nibiru\/en\/checkout\?order=/);
  await expectOrderState(page, /^(paid|issued)$/);
  await expect(page.getByTestId("order-paid")).toBeVisible();
  await expect(page.getByTestId("checkout-total")).toHaveText("RON 552.48");
});
