// Checkout helpers (step 4): hold tickets on the NIBIRU sample shop, give contact details and pay on
// the simulated provider's page. Every test uses its own browser context, so its own guest.
import { expect, type Page } from "@playwright/test";

export const GALAXIA = (locale: "en" | "ro" = "en") => `/s/nibiru/${locale}/e/galaxia-2027`;

/** Picks `qty` General Access tickets on Galaxia and continues to checkout; returns the order id. */
export async function holdGalaxia(page: Page, opts: { locale?: "en" | "ro"; qty?: number } = {}) {
  const locale = opts.locale ?? "en";
  await page.goto(GALAXIA(locale));
  await expect(page.getByTestId("shop-tickets")).toHaveAttribute("data-availability", "ok");
  const ga = page.locator("[data-testid=ticket-card]").filter({ has: page.getByRole("heading", { name: "General Access", exact: true }) });
  for (let i = 0; i < (opts.qty ?? 1); i++) await ga.getByTestId("plus").click();
  await page.getByTestId("continue").click();
  await expect(page).toHaveURL(/\/checkout\?order=[0-9a-f-]{36}/);
  await expect(page.getByTestId("order-state")).toHaveText("held");
  return new URL(page.url()).searchParams.get("order")!;
}

export async function giveDetails(page: Page, email = `fan-${Date.now()}@example.test`) {
  await page.getByTestId("contact-name").fill("Ana Pop");
  await page.getByTestId("contact-email").fill(email);
}

/** Chooses a method and presses Pay; for provider methods, waits for the provider's page. */
export async function pressPay(page: Page, method: "hosted_card" | "apple_pay" | "google_pay" | "wallet" | "saved_card") {
  await page.getByTestId(`method-${method}`).first().locator("input").check();
  await page.getByTestId("pay").click();
  if (method === "hosted_card" || method === "apple_pay" || method === "google_pay") {
    await expect(page.getByTestId("psp-page")).toBeVisible();
  }
}

/** Back on the shop after the provider: waits for the order's state. */
export async function expectOrderState(page: Page, state: string | RegExp) {
  await expect(page.getByTestId("order-state")).toHaveText(state, { timeout: 30_000 });
}
