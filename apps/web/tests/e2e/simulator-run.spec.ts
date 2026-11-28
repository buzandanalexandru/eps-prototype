// Step 6, E7 + E8 on the deployed system: a small rehearsal run with provider A down in its middle.
// Specs onsale-simulator "Failover in a real run" and "Summary after the run", onsale-dashboard
// "During a run": the dashboard's numbers change, a failover appears, and the run ends with a
// summary; and "Start the default run" with Stop. Small on purpose (a shared preview cluster). The
// two tests run one after the other: an environment runs one rehearsal at a time.
import { desktopOnly, expect, test } from "./fixtures";

test.describe.configure({ mode: "serial" });
import { openRehearsal, saleToRehearse, startRun } from "./simulator-helpers";

test("a small run with an A outage: live numbers, a failover, then the summary", async ({ page, browser }, info) => {
  test.skip(desktopOnly(info.project.name), "one run at a time per environment: desktop only");
  test.setTimeout(12 * 60_000);
  const { saleId } = await saleToRehearse(page);
  await openRehearsal(page, saleId);
  await page.getByTestId("knob-fans").fill("150");
  await page.getByTestId("knob-bot-share").fill("0.1");
  await page.getByTestId("knob-opens-in").fill("30");
  await page.getByTestId("knob-duration").fill("90");
  await page.getByTestId("knob-poll").fill("5");
  await page.getByTestId("knob-abandon").fill("0");
  await page.getByTestId("knob-decline").fill("0");
  await page.getByTestId("knob-provider-a").selectOption("outage");
  await page.getByTestId("knob-at").fill("0");
  await page.getByTestId("knob-for").fill("45");
  await startRun(page);
  await page.reload(); // the run is on the fresh rehearsal
  const rehearsalId = (await page.getByTestId("rehearsal").getAttribute("data-rehearsal-id"))!;
  // The dashboard of the rehearsal, beside the panel.
  const dash = await page.context().newPage();
  await dash.goto(`/studio/sales/${rehearsalId}/live`);
  await expect(dash.getByTestId("live-rehearsal")).toBeVisible();
  await expect(dash.getByTestId("live-ok")).toBeVisible({ timeout: 60_000 });
  // Fans arrive in the pre-queue, then are let in and buy.
  await expect.poll(async () => Number((await dash.getByTestId("tile-prequeue").textContent())?.replace(/\D/g, "") || 0), { timeout: 60_000 }).toBeGreaterThan(0);
  await expect.poll(async () => Number((await dash.getByTestId("tile-sold").textContent())?.split("/")[0].replace(/\D/g, "") || 0), { timeout: 180_000 }).toBeGreaterThan(0);
  // A is down from opening for 45 s: payments fail over to B and the dashboard shows it.
  await expect.poll(async () => Number((await dash.getByTestId("tile-failovers").textContent())?.replace(/\D/g, "") || 0), { timeout: 120_000 }).toBeGreaterThan(0);
  await expect(dash.getByTestId("spark-sold-marker").first()).toBeAttached();
  await expect(dash.getByTestId("live-truth")).toBeVisible();
  // The run ends with its summary.
  await expect(page.getByTestId("simulator-run")).toHaveAttribute("data-state", /done|stopped/, { timeout: 6 * 60_000 });
  await expect(page.getByTestId("simulator-summary")).toBeVisible();
  await expect(page.getByTestId("summary-sFansSent")).toHaveText(/^1(35|36)$/);
  expect(Number((await page.getByTestId("summary-sTickets").textContent())?.replace(/\D/g, ""))).toBeGreaterThan(0);
  expect(Number((await page.getByTestId("summary-sFailovers").textContent())?.replace(/\D/g, ""))).toBeGreaterThan(0);
  await expect(page.getByTestId("simulator-load-run")).toContainText("requests a second");
  await dash.close();
  void browser;
});

test("Start with the defaults runs 1,000 fans with 10% bots, and Stop ends it", async ({ page }, info) => {
  test.skip(desktopOnly(info.project.name), "one run at a time per environment: desktop only");
  test.setTimeout(8 * 60_000);
  const { saleId } = await saleToRehearse(page);
  await openRehearsal(page, saleId);
  await startRun(page);
  await expect(page.getByTestId("simulator-state")).toBeVisible();
  await expect(page.getByTestId("simulator-run")).toContainText(/1,000/);
  await expect(page.getByTestId("simulator-stop")).toBeVisible();
  await page.getByTestId("simulator-stop").click();
  await expect(page.getByTestId("simulator-run")).toHaveAttribute("data-state", "stopped", { timeout: 30_000 });
  await expect(page.getByTestId("simulator-start")).toBeEnabled();
});
