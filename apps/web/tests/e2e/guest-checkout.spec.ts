// Checkout spec "Guest checkout": a guest buys with an email and the order link opens the status
// page without signing in; the claim into an account is covered in claim_test.go.
import { expect, test } from "./fixtures";
import { expectOrderState, giveDetails, holdGalaxia, pressPay } from "./checkout-helpers";

test("a guest buys and the order link opens the order on another device", async ({ page, browser }) => {
  const id = await holdGalaxia(page);
  await giveDetails(page, "guest@example.test");
  await pressPay(page, "hosted_card");
  await page.getByTestId("psp-approve").click();
  await expectOrderState(page, /^(paid|issued)$/);
  const link = await page.getByTestId("order-link").inputValue();
  await expect(page.getByTestId("add-to-account")).toBeVisible();
  await expect(page.getByTestId("save-this-link")).toHaveText("Save this link: it is the only way to reopen the order without an account.");
  const ctx = await browser.newContext();
  const other = await ctx.newPage();
  await other.goto(link);
  await expect(other.getByTestId("order")).toHaveAttribute("data-order-id", id);
  await expectOrderState(other, /^(paid|issued)$/);
  await ctx.close();
});
