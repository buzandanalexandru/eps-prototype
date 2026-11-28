// Checkout spec "Instalment checkout" (scenario "Deposit pays the order"): the plan and its full
// schedule before payment, the deposit charged now with a provider-saved card, the order paid and
// the plan active with its parts scheduled; "simulate next due date" charges the next part.
import { expect, expectNoHorizontalScroll, test } from "./fixtures";
import { expectOrderState, giveDetails, holdGalaxia } from "./checkout-helpers";

test("pay a deposit now: the order is paid, the plan active, the parts scheduled on the saved card", async ({ page }) => {
  test.setTimeout(120_000);
  await holdGalaxia(page);
  await giveDetails(page);
  await page.getByTestId("plan-instalments").check();
  const schedule = page.getByTestId("schedule");
  await expect(schedule.getByTestId("schedule-deposit")).toContainText("RON 10.00");
  expect(await schedule.getByTestId("schedule-part").count()).toBeGreaterThan(0);
  await expect(schedule.getByTestId("schedule-total")).toHaveText(await page.getByTestId("checkout-total").textContent() ?? "");
  await expect(page.getByTestId("method-apple_pay")).toHaveCount(0);
  await expect(page.getByTestId("pay")).toHaveText("Pay RON 10.00");
  await expectNoHorizontalScroll(page);
  await page.getByTestId("method-hosted_card").locator("input").check();
  await page.getByTestId("pay").click();
  await page.getByTestId("psp-approve").click();
  await expectOrderState(page, /^(paid|issued)$/);
  const plan = page.getByTestId("plan");
  await expect(plan).toHaveAttribute("data-plan-state", "active");
  await expect(plan.getByTestId("plan-card")).toContainText("Test Visa •••• 4242");
  const parts = plan.getByTestId("plan-part");
  await expect(parts.first()).toHaveAttribute("data-part-state", "scheduled");
  // Demo: move the plan's clock; the worker charges the part with the saved card.
  await page.getByTestId("simulate-next").click();
  await expect(parts.first()).toHaveAttribute("data-part-state", "paid", { timeout: 30_000 });
});
