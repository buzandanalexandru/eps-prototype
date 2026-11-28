// Checkout spec "Resume payment link" (scenario "Resume on another device"); the expired and
// tampered links are covered in services/internal/checkout/resume_test.go, with the page's answer
// for a changed link checked here.
import { devices } from "@playwright/test";
import { expect, expectNoHorizontalScroll, test } from "./fixtures";
import { expectOrderState, giveDetails, holdGalaxia, pressPay } from "./checkout-helpers";

test("the resume link opens the same order, timer and total on a phone, and pays there", async ({ page, browser }) => {
  const id = await holdGalaxia(page, { qty: 2 });
  const total = await page.getByTestId("checkout-total").textContent();
  const link = await page.getByTestId("resume-link").inputValue();
  const phone = await browser.newContext({ ...devices["Pixel 7"] });
  const p = await phone.newPage();
  await p.goto(link);
  await expect(p.getByTestId("order")).toHaveAttribute("data-order-id", id);
  await expect(p.getByTestId("checkout-total")).toHaveText(total!);
  await expect(p.getByTestId("hold-timer")).toBeVisible();
  await expect(p.getByTestId("pay-form")).toBeVisible();
  await expectNoHorizontalScroll(p);
  await giveDetails(p);
  await pressPay(p, "hosted_card");
  await p.getByTestId("psp-approve").click();
  await expectOrderState(p, /^(paid|issued)$/);
  await phone.close();
  // The first device sees the truth too.
  await page.reload();
  await expectOrderState(page, /^(paid|issued)$/);
});

test("a changed link shows nothing of the order (Order not found)", async ({ page }) => {
  const id = await holdGalaxia(page);
  const ctx = await page.context().browser()!.newContext();
  const other = await ctx.newPage();
  await other.goto(`/s/nibiru/en/checkout?order=${id}&t=forged.token`);
  await expect(other.getByTestId("order-not-found")).toHaveText("Order not found");
  await expect(other.getByTestId("pay-form")).toHaveCount(0);
  await ctx.close();
});
