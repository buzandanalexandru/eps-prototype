// Step 6, E8: the live dashboard. Spec onsale-dashboard: "Before any traffic", "Connection drops"
// and "Turn the dial". The organiser's set-up runs on desktop; the dashboard is checked on desktop
// and phone, in English and Romanian, with no console errors and no horizontal scroll.
import { devices, type Page } from "@playwright/test";
import { desktopOnly, expect, expectErrorResponse, expectNoHorizontalScroll, setLocale, test } from "./fixtures";
import { addSale, publishedEvent } from "./onsale-helpers";

async function frameTs(page: Page) {
  return Number(await page.getByTestId("live-tiles").getAttribute("data-frame-ts"));
}

test("before any traffic every tile shows its value and the frames keep coming (EN desktop, RO phone)", async ({ page, browser }, info) => {
  test.skip(desktopOnly(info.project.name), "the organiser's set-up runs once; the phone view is checked below");
  test.setTimeout(120_000);
  const ev = await publishedEvent(page);
  const saleId = await addSale(page, ev.eventUrl, { opensIn: 60 });
  expectErrorResponse(page, 401); // a fresh environment's first key reaches the gateway within ~20 s
  await page.goto(`/studio/sales/${saleId}/live`);
  await expect(page.getByTestId("live-ok")).toBeVisible({ timeout: 60_000 });
  for (const id of ["queue", "prequeue", "admitted", "shopping", "failovers", "bots"]) await expect(page.getByTestId(`tile-${id}`)).toHaveText("0");
  await expect(page.getByTestId("tile-sold")).toHaveText(/^0 \/ \d/);
  await expect(page.getByTestId("provider-a")).toHaveAttribute("data-state", "closed");
  await expect(page.getByTestId("provider-b")).toContainText("Taking payments");
  await expect(page.getByTestId("live-stock").getByTestId("stock-line").first()).toBeVisible();
  const t1 = await frameTs(page);
  await expect.poll(() => frameTs(page), { timeout: 10_000 }).toBeGreaterThan(t1);
  await expectNoHorizontalScroll(page);
  // The same organiser on a phone, in Romanian.
  const phone = await browser.newContext({ ...devices["Pixel 7"], storageState: await page.context().storageState() });
  const p = await phone.newPage();
  const errors: string[] = [];
  p.on("console", (m) => m.type() === "error" && errors.push(m.text()));
  p.on("pageerror", (e) => errors.push(e.message));
  await setLocale(p, "ro");
  await p.goto(`/studio/sales/${saleId}/live`);
  await expect(p.getByTestId("live-ok")).toBeVisible({ timeout: 20_000 });
  await expect(p.getByRole("heading", { level: 2 })).toContainText("Vânzare live");
  await expect(p.getByTestId("provider-a")).toContainText("Primește plăți");
  await expectNoHorizontalScroll(p);
  expect(errors, "console errors on the phone").toEqual([]);
  await phone.close();
});

test("the dial moves the sale's rate and the frames show it", async ({ page }, info) => {
  test.skip(desktopOnly(info.project.name), "organiser set-up on desktop");
  test.setTimeout(120_000);
  const ev = await publishedEvent(page);
  const saleId = await addSale(page, ev.eventUrl, { opensIn: 60, rate: 2000 });
  expectErrorResponse(page, 401); // a fresh environment's first key reaches the gateway within ~20 s
  await page.goto(`/studio/sales/${saleId}/live`);
  await expect(page.getByTestId("live-ok")).toBeVisible({ timeout: 60_000 });
  await expect(page.getByTestId("live-rate-setting")).toHaveText("2000");
  await page.getByTestId("live-rate").fill("4000");
  await page.getByTestId("live-rate-apply").click();
  await expect(page.getByTestId("live-notice")).toHaveText("Saved.");
  await expect(page.getByTestId("live-rate-setting")).toHaveText("4000", { timeout: 10_000 });
  await page.getByTestId("live-pause").click();
  await expect(page.getByTestId("live-pause")).toHaveText("Resume admissions", { timeout: 10_000 });
});

test("a dropped connection shows stale, reconnects and resumes with the latest numbers", async ({ page }, info) => {
  test.skip(desktopOnly(info.project.name), "organiser set-up on desktop");
  test.setTimeout(120_000);
  const ev = await publishedEvent(page);
  const saleId = await addSale(page, ev.eventUrl, { opensIn: 60 });
  expectErrorResponse(page, 401); // a fresh environment's first key reaches the gateway within ~20 s
  await page.goto(`/studio/sales/${saleId}/live`);
  await expect(page.getByTestId("live-ok")).toBeVisible({ timeout: 60_000 });
  const frame = await page.getByTestId("live-tiles").getAttribute("data-frame-ts");
  // From now on the stream ends at once and reconnects are refused for 10 seconds.
  let blocked = true;
  await page.route("**/v1/live/**", async (route) => {
    if (!blocked) return route.continue();
    const origin = new URL(page.url()).origin;
    expectErrorResponse(page, 503);
    await route.fulfill({ status: 503, headers: { "access-control-allow-origin": origin, "content-type": "application/problem+json" }, body: '{"code":"EPS-UNAVAILABLE"}' });
  });
  expect(frame).not.toBeNull();
  // The open stream drops (new connections then hit the route) for 10 seconds.
  await page.evaluate(() => window.dispatchEvent(new Event("eps-live-drop")));
  await expect(page.getByTestId("live-stale")).toBeVisible({ timeout: 10_000 });
  await page.waitForTimeout(5_000);
  const before = await frameTs(page);
  blocked = false;
  await expect(page.getByTestId("live-ok")).toBeVisible({ timeout: 60_000 });
  await expect.poll(() => frameTs(page), { timeout: 15_000 }).toBeGreaterThan(before + 9_000);
});
