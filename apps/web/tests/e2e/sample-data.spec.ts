// Sample-data spec: the seeded NIBIRU organiser as the demo organiser sees it, labelled, apart from
// Sunwave; plus the phone and desktop layout of every Studio screen in both languages.
import { expect, expectNoHorizontalScroll, setLocale, signInDemoOrganiser, test } from "./fixtures";

test.describe("sample data", () => {
  test("the demo organiser owns NIBIRU with both brands under pass on; nothing of Sunwave is visible", async ({ page }) => {
    await setLocale(page, "en");
    await signInDemoOrganiser(page);
    await page.goto("/studio");
    await expect(page.getByTestId("active-organiser")).toHaveText("NIBIRU");
    await expect(page.getByTestId("sample-badge")).toHaveText("Sample data");
    await expect(page.getByTestId("overview-brands")).toContainText("NIBIRU Festivals");
    await expect(page.getByTestId("overview-brands")).toContainText("NIBIRU Resort");
    await expect(page.getByTestId("overview-brands")).toContainText("/s/nibiru-resort");
    await expect(page.getByTestId("overview-events")).toContainText("Galaxia 2027");
    await expect(page.locator("body")).not.toContainText("Sunwave");
    await page.goto("/studio/fees");
    await expect(page.getByTestId("current-policy")).toContainText("Pass on");
    await page.goto("/studio/team");
    await expect(page.getByTestId("member")).toHaveCount(1);
    await expect(page.getByTestId("member-role")).toContainText("Owner");
    await page.goto("/studio/events");
    await expect(page.getByTestId("event-list")).not.toContainText("Sunwave");
  });

  test("Galaxia's GA pass shows 552.48 all-in, the Arena pool and a ready event", async ({ page }) => {
    await setLocale(page, "en");
    await signInDemoOrganiser(page);
    await page.goto("/studio/events");
    await page.getByRole("link", { name: "Galaxia 2027" }).click();
    await expect(page.getByTestId("readiness")).toHaveAttribute("data-ready", "true");
    const eventUrl = new URL(page.url()).pathname;
    await page.goto(`${eventUrl}/products`);
    const ga = page.getByTestId("product").filter({ has: page.getByRole("link", { name: "General Access", exact: true }) });
    await expect(ga.getByTestId("fan-pays")).toHaveText("Fan pays 552.48 RON");
    // Instalments: 10 RON now and up to 7 monthly payments, capped per buyer.
    await ga.getByRole("link", { name: "General Access", exact: true }).click();
    await expect(page.getByTestId("deposit")).toHaveText("10.00 RON now");
    await expect(page.getByTestId("breakpoints")).toBeVisible();
    await page.goto(`${eventUrl}/pools`);
    const arena = page.getByTestId("pool").filter({ has: page.getByTestId("pool-name").getByText("Arena", { exact: true }) });
    await expect(arena).toContainText("General Access, General Access Plus");
    await expect(arena.getByTestId("pool-capacity")).toContainText("40,000");
  });

  for (const locale of ["ro", "en"] as const) {
    test(`every Studio screen in ${locale}: labelled, no horizontal scroll`, async ({ page }) => {
      test.setTimeout(120_000);
      await setLocale(page, locale);
      await signInDemoOrganiser(page);
      await expect(page.getByTestId("sample-badge")).toHaveText(locale === "ro" ? "Date de exemplu" : "Sample data");
      await page.goto("/studio/events");
      const events = await page.getByTestId("event-list").locator("a").evaluateAll((as) => as.map((a) => a.getAttribute("href")!));
      const galaxia = events[0];
      await page.goto(`${galaxia}/products`);
      const product = await page.getByTestId("product-list").locator("a").first().getAttribute("href");
      const brand = await (async () => {
        await page.goto("/studio/brands");
        return page.getByTestId("brand-list").locator("a").first().getAttribute("href");
      })();
      for (const path of ["/studio", "/studio/brands", brand!, "/studio/events", "/studio/venues", "/studio/team", "/studio/fees", galaxia, `${galaxia}/products`, `${galaxia}/pools`, product!, "/studio/events/new", "/fees"]) {
        const res = await page.goto(path);
        expect(res?.status(), path).toBe(200);
        await expect(page.getByRole("heading", { level: 1 }).first()).toBeVisible();
        if (path.startsWith("/studio")) await expect(page.getByTestId("sample-badge")).toBeVisible();
        await expectNoHorizontalScroll(page);
      }
    });
  }
});
