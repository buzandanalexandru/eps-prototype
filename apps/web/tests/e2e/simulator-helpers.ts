// Step 6 helpers: a fresh organiser's published event with a sale moment, its rehearsal, and runs
// started from the simulator panel (one run at a time per environment: Start waits while busy).
import { expect, type Page } from "@playwright/test";
import { addSale, publishedEvent } from "./onsale-helpers";

/** A fresh organiser, a published event and a sale moment opening in a day; returns its ids. */
export async function saleToRehearse(page: Page) {
  const ev = await publishedEvent(page);
  const saleId = await addSale(page, ev.eventUrl, { opensIn: 24 * 60, rate: 60000 });
  return { ...ev, saleId };
}

/** Opens the simulator panel and creates the rehearsal when there is none. */
export async function openRehearsal(page: Page, saleId: string) {
  await page.goto(`/studio/sales/${saleId}/simulator`);
  if (await page.getByTestId("rehearsal-create").isVisible()) await page.getByTestId("rehearsal-create").click();
  await expect(page.getByTestId("rehearsal")).toBeVisible();
  return (await page.getByTestId("rehearsal").getAttribute("data-rehearsal-id"))!;
}

/** Presses Start, waiting while another rehearsal runs in this environment (up to 5 minutes). */
export async function startRun(page: Page) {
  const deadline = Date.now() + 5 * 60_000;
  for (;;) {
    await page.getByTestId("simulator-start").click();
    const run = page.getByTestId("simulator-run");
    const err = page.getByTestId("simulator-start-error");
    await expect(run.and(page.locator("[data-run-id]:not([data-run-id=''])")).or(err)).toBeVisible({ timeout: 20_000 });
    if (!(await err.isVisible())) break;
    const text = (await err.textContent()) ?? "";
    if (!/Another rehearsal|O altă repetiție/.test(text) || Date.now() > deadline) throw new Error(`start refused: ${text}`);
    await page.waitForTimeout(10_000);
  }
  await expect(page.getByTestId("simulator-run")).toHaveAttribute("data-state", /pending|starting|running|finishing|done/);
}
