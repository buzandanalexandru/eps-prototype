// Step 7 helpers: buy on the sample shops with the simulated provider, then find the tickets in the
// trip wallet and show a live code.
import { expect, type Page } from "@playwright/test";
import { expectOrderState, giveDetails, holdGalaxia, pressPay } from "./checkout-helpers";

export const SUNWAVE = (locale: "en" | "ro" = "en") => `/s/sunwave-festival/${locale}/e/sunwave-beach-day-2027`;

/** Buys `qty` Galaxia General Access passes by card (approved at the provider); returns the order id. */
export async function buyGalaxia(page: Page, opts: { qty?: number; locale?: "en" | "ro"; email?: string } = {}) {
  const id = await holdGalaxia(page, { qty: opts.qty, locale: opts.locale });
  await giveDetails(page, opts.email);
  await pressPay(page, "hosted_card");
  await page.getByTestId("psp-approve").click();
  await expectOrderState(page, /^(paid|issued)$/);
  return id;
}

/** Buys one Sunwave day ticket (another sample organiser). */
export async function buySunwave(page: Page) {
  await page.goto(SUNWAVE());
  await expect(page.getByTestId("shop-tickets")).toHaveAttribute("data-availability", "ok");
  const card = page.locator("[data-testid=ticket-card]").filter({ has: page.getByRole("heading", { name: "Day ticket", exact: true }) });
  await card.getByTestId("plus").click();
  await page.getByTestId("continue").click();
  await expect(page).toHaveURL(/\/checkout\?order=[0-9a-f-]{36}/);
  await giveDetails(page);
  await pressPay(page, "hosted_card");
  await page.getByTestId("psp-approve").click();
  await expectOrderState(page, /^(paid|issued)$/);
}

/** Opens the wallet and waits until it lists `n` items; returns their ticket ids. */
export async function walletTickets(page: Page, n = 1): Promise<string[]> {
  await page.goto("/wallet");
  await expect(page.getByTestId("trip-item")).toHaveCount(n, { timeout: 30_000 });
  return page.getByTestId("trip-item").evaluateAll((els) => els.map((e) => e.getAttribute("data-ticket-id")!));
}

/** On a live ticket that needs a name: names it and waits for the code. */
export async function nameTicket(page: Page, name = "Ana Pop") {
  await page.getByTestId("name-input").fill(name);
  await page.getByTestId("name-save").click();
  await expect(page.getByTestId("ticket-holder")).toContainText(name);
  await expect(page.getByTestId("live-code")).toBeVisible({ timeout: 15_000 });
}

/** The code on screen now and its window. */
export async function currentCode(page: Page): Promise<{ code: string; window: number }> {
  const el = page.getByTestId("live-code");
  return { code: (await el.getAttribute("data-code"))!, window: Number(await el.getAttribute("data-window")) };
}
