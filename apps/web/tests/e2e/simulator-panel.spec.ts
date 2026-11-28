// Step 6, E7: the simulator panel. Spec onsale-simulator: "Arithmetic labelled" (the run itself:
// simulator-run.spec.ts); desktop and phone, English and Romanian, no console errors, no horizontal scroll.
import { devices } from "@playwright/test";
import { desktopOnly, expect, expectNoHorizontalScroll, setLocale, test } from "./fixtures";
import { openRehearsal, saleToRehearse } from "./simulator-helpers";

test("the panel says the fans are simulated, shows measured k6 numbers and the 100k figure as arithmetic (EN desktop, RO phone)", async ({ page, browser }, info) => {
  test.skip(desktopOnly(info.project.name), "the organiser's set-up runs once; the phone view is checked below");
  test.setTimeout(120_000);
  const { saleId } = await saleToRehearse(page);
  await page.goto(`/studio/sales/${saleId}/simulator`);
  await expect(page.getByTestId("simulator-simulated-note")).toContainText("Simulated fans, real traffic");
  await expect(page.getByTestId("rehearsal-none")).toBeVisible();
  await expect(page.getByTestId("simulator-load-measured")).toContainText("1,000 joins and 2,000 status polls a second");
  await expect(page.getByTestId("simulator-load-math")).toContainText("100,000 fans");
  await expect(page.getByText("Arithmetic, not a test")).toBeVisible();
  await expect(page.getByTestId("simulator-load-cap")).toContainText("simulated fans at a time");
  const rehearsalId = await openRehearsal(page, saleId);
  expect(rehearsalId).toMatch(/^[0-9a-f-]{36}$/);
  await expect(page.getByTestId("rehearsal-name")).toHaveText("Rehearsal: General sale");
  await expect(page.getByTestId("knob-fans")).toHaveValue("1000");
  await expect(page.getByTestId("knob-bot-share")).toHaveValue("0.1");
  await expect(page.getByTestId("simulator-run")).toHaveAttribute("data-state", "none");
  await expectNoHorizontalScroll(page);
  // Its waiting room labels the rehearsal.
  await page.goto((await page.getByTestId("rehearsal-room").getAttribute("href"))!);
  await expect(page.getByTestId("queue-rehearsal")).toContainText("Rehearsal: simulated fans");
  await expectNoHorizontalScroll(page);
  // Romanian, with diacritics, on a phone.
  const phone = await browser.newContext({ ...devices["Pixel 7"], storageState: await page.context().storageState() });
  const p = await phone.newPage();
  const errors: string[] = [];
  p.on("console", (m) => m.type() === "error" && errors.push(m.text()));
  p.on("pageerror", (e) => errors.push(e.message));
  await setLocale(p, "ro");
  await p.goto(`/studio/sales/${saleId}/simulator`);
  await expect(p.getByRole("heading", { level: 2 })).toContainText("Repetiția vânzării");
  await expect(p.getByTestId("simulator-load-math")).toContainText("Niciuna dintre presupuneri nu a fost măsurată aici");
  await expect(p.getByTestId("knob-fans")).toBeVisible();
  await expectNoHorizontalScroll(p);
  expect(errors, "console errors on the phone").toEqual([]);
  await phone.close();
});
