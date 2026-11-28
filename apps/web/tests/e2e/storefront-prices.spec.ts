// Storefront spec "All-in prices everywhere" and the picker's running total and hand-off.
import { expect, expectNoHorizontalScroll, test } from "./fixtures";

const GALAXIA = "/s/nibiru/en/e/galaxia-2027";

test("GA shows RON 552.48 everywhere, with the fee it includes; Romanian reads 552,48 RON", async ({ page }) => {
  await page.goto(GALAXIA);
  const ga = page.getByTestId("ticket-card").filter({ hasText: "General Access" }).first();
  await expect(ga.getByTestId("ticket-price")).toHaveText("RON 552.48");
  await expect(ga.getByTestId("fee-line")).toHaveText("includes platform fee of RON 13.48");
  await expect(page.getByTestId("from-price")).toHaveText("from RON 552.48");
  await expect(ga.getByTestId("ladder-sales")).toHaveText("Price may rise as tickets sell");
  await expect(ga.getByTestId("instalments")).toContainText("RON 10.00 now + ");
  await page.goto("/s/nibiru/ro/e/galaxia-2027");
  await expect(page.getByTestId("from-price")).toHaveText("de la 552,48 RON");
  await expect(page.getByTestId("fee-line").first()).toHaveText("include taxa platformei de 13,48 RON");
});

test("2 GA and 1 camping: the running all-in total, and Continue hands the selection to checkout", async ({ page }) => {
  await page.goto(GALAXIA);
  await expect(page.getByTestId("shop-tickets")).toHaveAttribute("data-availability", "ok");
  const ga = page.locator("[data-testid=ticket-card]").filter({ has: page.getByRole("heading", { name: "General Access", exact: true }) });
  const camping = page.locator("[data-testid=ticket-card]").filter({ hasText: "Galaxia camping" });
  await camping.getByTestId("plus").click();
  await expect(page.getByTestId("needs-ticket")).toHaveText("Galaxia camping needs a Galaxia 2027 ticket");
  await expect(page.getByTestId("continue")).toBeDisabled();
  await ga.getByTestId("plus").click();
  await ga.getByTestId("plus").click();
  await expect(ga.getByTestId("qty")).toHaveText("2");
  await expect(page.getByTestId("picker-total")).toHaveText("RON 1,308.94");
  await expect(page.getByTestId("needs-ticket")).toHaveCount(0);
  await expectNoHorizontalScroll(page);
  await page.getByTestId("continue").click();
  await expect(page).toHaveURL(/\/s\/nibiru\/en\/checkout\?event=[0-9a-f-]{36}&v=\d+&items=/);
  const items = new URL(page.url()).searchParams.get("items")!;
  expect(items.split(",").map((i) => i.split(":")[1]).sort()).toEqual(["1", "2"]);
  await expect(page.getByTestId("checkout-line")).toHaveCount(2);
  await expect(page.getByTestId("checkout-total")).toHaveText("RON 1,308.94");
  await expect(page.locator('meta[name="robots"]')).toHaveAttribute("content", /noindex/);
});
