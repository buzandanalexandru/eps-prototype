// Checkout spec "Honest order status page": waiting for the bank, a declined card with its code
// and the hold still running, and an unknown order.
import { expect, expectNoHorizontalScroll, test } from "./fixtures";
import { expectOrderState, giveDetails, holdGalaxia, pressPay } from "./checkout-helpers";

test("back from the provider before it confirmed: waiting, not success", async ({ page }) => {
  const id = await holdGalaxia(page);
  await giveDetails(page);
  await pressPay(page, "hosted_card");
  // The fan comes back to the order without finishing at the provider.
  await page.goBack();
  await expect(page.getByTestId("order")).toHaveAttribute("data-order-id", id);
  await expectOrderState(page, "paying");
  await expect(page.getByTestId("waiting-provider")).toContainText("Waiting for confirmation from the payment provider");
  await expect(page.getByTestId("hold-timer")).toBeVisible();
  await expect(page.getByTestId("order-paid")).toHaveCount(0);
});

test("a declined card: plain words, EPS-PAY-DECLINED, the hold and a retry", async ({ page }) => {
  await holdGalaxia(page);
  await giveDetails(page);
  await pressPay(page, "hosted_card");
  await page.getByTestId("psp-decline-funds").click();
  await expectOrderState(page, "payment_failed");
  await expect(page.getByTestId("payment-error")).toContainText("The payment was declined by the bank.");
  await expect(page.getByTestId("payment-error").getByTestId("error-code")).toHaveText("EPS-PAY-DECLINED");
  await expect(page.getByTestId("hold-timer")).toBeVisible();
  await expect(page.getByTestId("pay")).toHaveText("Try again");
  await expectNoHorizontalScroll(page);
  // Trying again works.
  await pressPay(page, "hosted_card");
  await page.getByTestId("psp-approve").click();
  await expectOrderState(page, /^(paid|issued)$/);
});

test("an unknown order or a missing link: Order not found, never a success screen", async ({ page }) => {
  await page.goto("/s/nibiru/en/checkout?order=00000000-0000-4000-8000-000000000000");
  await expect(page.getByTestId("order-not-found")).toHaveText("Order not found");
  await expect(page.getByTestId("order-paid")).toHaveCount(0);
  // Someone else's order, opened without its link.
  const ctx = await page.context().browser()!.newContext();
  const other = await ctx.newPage();
  const id = await holdGalaxia(other);
  await ctx.close();
  await page.goto(`/s/nibiru/en/checkout?order=${id}`);
  await expect(page.getByTestId("order-not-found")).toBeVisible();
});
