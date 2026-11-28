import { expect, type Page } from "@playwright/test";

/** In a fresh organiser's Studio (English UI): a venue, an event with 2 days, a pool and a 539.00 RON pass. */
export async function createSellableEvent(page: Page, slug: string) {
  await page.goto("/studio/venues");
  const v = page.getByTestId("new-venue-form");
  await v.getByLabel("Name", { exact: true }).fill("Shop Test Arena");
  await v.getByRole("button", { name: "Create venue" }).click();
  await expect(page.getByTestId("venue-list")).toContainText("Shop Test Arena");
  await page.goto("/studio/events/new");
  const ev = page.getByTestId("new-event-form");
  await ev.getByLabel("Venue").selectOption({ label: "Shop Test Arena" });
  await ev.getByLabel("Name (Romanian)").fill("Festivalul de test");
  await ev.getByLabel("Name (English)").fill("Test Festival");
  await ev.getByLabel("Description (Romanian)").fill("Două nopți la mare.");
  await ev.getByLabel("Description (English)").fill("Two nights by the sea.");
  await ev.getByLabel("Event address").fill(slug);
  await ev.getByLabel("Starts").fill("2027-07-15T17:00");
  await ev.getByLabel("Ends").fill("2027-07-17T03:00");
  await ev.getByLabel("Opens").fill("17:00");
  await ev.getByLabel("Closes").fill("03:00");
  await ev.getByRole("button", { name: "Create event" }).click();
  await expect(page).toHaveURL(/\/studio\/events\/[0-9a-f-]{36}$/);
  const eventUrl = new URL(page.url()).pathname;
  await page.goto(`${eventUrl}/pools`);
  const pool = page.getByTestId("new-pool-form");
  await pool.getByLabel("Name", { exact: true }).fill("Arena");
  await pool.getByLabel("Capacity").fill("40000");
  await pool.getByRole("button", { name: "Create pool" }).click();
  await expect(page.getByTestId("pool-name")).toHaveText("Arena");
  await page.goto(`${eventUrl}/products`);
  await page.getByLabel("Product type").selectOption("pass");
  await page.getByRole("button", { name: "Add product" }).click();
  const f = page.getByTestId("product-form");
  await f.getByLabel("Name (Romanian)").fill("Acces general");
  await f.getByLabel("Name (English)").fill("General Access");
  await f.getByLabel("Zone").fill("Arena");
  await f.getByLabel(/^Price/).fill("539");
  await f.getByLabel("Capacity pool").selectOption({ label: "Arena" });
  await f.getByRole("button", { name: "Create product" }).click();
  await expect(page).toHaveURL(/\/products\/[0-9a-f-]{36}$/);
  const productUrl = new URL(page.url()).pathname;
  await page.goto(eventUrl);
  await expect(page.getByTestId("readiness")).toHaveAttribute("data-ready", "true");
  return { eventUrl, productUrl };
}
