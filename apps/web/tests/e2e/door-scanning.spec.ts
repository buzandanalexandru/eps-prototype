// Door scanning spec: box office prepares a phone, reloads it with no network, admits a live code
// in airplane mode, refuses the same ticket again, and refuses a screenshot of a code taken earlier
// (with the reason in Romanian).
import type { Browser, Page } from "@playwright/test";
import { expect, expectNoHorizontalScroll, expectOfflineErrors, setLocale, signInDemoOrganiser, signInFreshUser, test } from "./fixtures";
import { buyGalaxia, currentCode, nameTicket, walletTickets } from "./ticket-helpers";

/** A fan in their own browser with a named Galaxia pass open on its live code. */
async function fanWithLiveCode(browser: Browser, name: string) {
  const ctx = await browser.newContext();
  const fan = await ctx.newPage();
  await signInFreshUser(fan);
  await buyGalaxia(fan);
  const [id] = await walletTickets(fan);
  const eventId = (await fan.getByTestId("trip-event").first().getAttribute("data-event-id"))!;
  await fan.goto(`/wallet/tickets/${id}`);
  await nameTicket(fan, name);
  return { ctx, fan, eventId };
}

async function prepareDoor(page: Page, eventId: string, gate: string) {
  await signInDemoOrganiser(page);
  await page.goto(`/door/${eventId}`);
  await page.getByTestId("door-gate").fill(gate);
  await page.getByTestId("door-prepare").click();
  await expect(page.getByTestId("door-ready")).toBeVisible({ timeout: 30_000 });
  // The service worker holds the page before the device goes offline.
  await expect
    .poll(async () => page.evaluate(async () => Boolean((await navigator.serviceWorker.getRegistration("/door/"))?.active && (await caches.match(location.pathname)))), { timeout: 20_000 })
    .toBe(true);
}

async function scan(page: Page, code: string) {
  await page.getByTestId("door-code-input").fill(code);
  await page.getByTestId("door-check").click();
  return page.getByTestId("door-result");
}

test("Box office prepares a device, reloads it offline and admits a live code once", async ({ page, browser, context }) => {
  test.setTimeout(150_000);
  const { ctx, fan, eventId } = await fanWithLiveCode(browser, "Ana Pop");
  await prepareDoor(page, eventId, "Gate B");
  await expectNoHorizontalScroll(page);
  // Airplane mode: the page opens from the device's cache and still verifies.
  expectOfflineErrors(page);
  await context.setOffline(true);
  await page.reload();
  await expect(page.getByTestId("door-ready")).toBeVisible();
  await expect(page.getByTestId("door-sync-age")).toContainText("Offline");
  const { code } = await currentCode(fan);
  const r = await scan(page, code);
  await expect(r).toHaveAttribute("data-ok", "true");
  await expect(r).toContainText("Valid");
  await expect(r).toContainText("Ana Pop");
  await expect(r).toBeHidden({ timeout: 5000 }); // back to scanning within two seconds
  // The same ticket again (its next code): refused with the time and gate of the first entry.
  await page.waitForTimeout(3000);
  const again = await scan(page, (await currentCode(fan)).code);
  await expect(again).toHaveAttribute("data-reason", "already");
  await expect(again).toContainText(/Already scanned at \d{2}:\d{2} at Gate B/);
  await expect(page.getByTestId("door-pending")).toContainText("1 scans waiting to upload");
  // Online again: the scan is uploaded.
  await context.setOffline(false);
  await page.getByTestId("door-sync").click();
  await expect(page.getByTestId("door-pending")).toHaveCount(0, { timeout: 20_000 });
  await ctx.close();
});

test("Rejected scan in Romanian: a screenshot of a code from earlier", async ({ page, browser }, info) => {
  test.skip(info.project.name === "desktop", "the door is a phone page; one project is enough for the wait");
  test.setTimeout(150_000);
  const { ctx, fan, eventId } = await fanWithLiveCode(browser, "Ion Pop");
  const shot = await currentCode(fan); // the "screenshot"
  await setLocale(page, "ro");
  await prepareDoor(page, eventId, "Poarta A");
  // Two windows later the screenshot's code is expired.
  await expect.poll(async () => (await currentCode(fan)).window, { timeout: 60_000, intervals: [1000] }).toBeGreaterThanOrEqual(shot.window + 3);
  const r = await scan(page, shot.code);
  await expect(r).toHaveAttribute("data-ok", "false");
  await expect(r).toHaveAttribute("data-reason", "expired");
  await expect(r).toContainText("Cod expirat: pare o captură de ecran sau un cod vechi");
  await expect(r).toBeHidden({ timeout: 5000 });
  // The live code on the holder's phone is admitted.
  await page.waitForTimeout(500);
  const live = await scan(page, (await currentCode(fan)).code);
  await expect(live).toHaveAttribute("data-ok", "true");
  await expect(live).toContainText("Valid");
  await expectNoHorizontalScroll(page);
  await ctx.close();
});
