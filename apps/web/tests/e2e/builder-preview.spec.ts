// Storefront builder spec "Live preview on desktop and phone".
import { desktopOnly, expect, expectNoHorizontalScroll, onboard, setLocale, signInDemoOrganiser, signInFreshUser, test } from "./fixtures";
import { frame, openBuilder, showPane } from "./builder-helpers";

test.describe("builder preview", () => {
  test("a new hero title shows in the preview within a second, before autosave completes", async ({ page }, info) => {
    test.skip(desktopOnly(info.project.name), "creates data");
    await setLocale(page, "en");
    await signInFreshUser(page);
    await onboard(page);
    await openBuilder(page);
    await page.getByTestId("preview-locale").selectOption("en");
    await page.locator("[data-testid=section-row][data-section-type=hero]").getByTestId("section-select").click();
    const input = page.getByTestId("field-heading").locator("input[data-locale=en]");
    await input.fill("Summer nights 2027");
    await expect(frame(page).locator("[data-eps-type=hero] h1")).toHaveText("Summer nights 2027", { timeout: 1000 });
    await expect(page.getByTestId("save-state")).toHaveText("All changes saved", { timeout: 10_000 });
  });

  test("phone width: the frame is 390 px wide, has no horizontal scroll and the ticket grid stacks", async ({ page }) => {
    await setLocale(page, "en");
    await signInDemoOrganiser(page);
    await openBuilder(page);
    await page.getByTestId("device-phone").click();
    await expect(page.getByTestId("preview-frame")).toHaveAttribute("data-device", "phone");
    const f = frame(page);
    await expect.poll(() => f.locator("html").evaluate((el) => el.clientWidth)).toBe(390);
    expect(await f.locator("html").evaluate((el) => el.scrollWidth - el.clientWidth)).toBeLessThanOrEqual(0);
    const cards = f.getByTestId("ticket-card");
    await expect(cards.first()).toBeVisible();
    const xs = await cards.evaluateAll((els) => els.slice(0, 3).map((e) => Math.round(e.getBoundingClientRect().left)));
    expect(new Set(xs).size).toBe(1);
    await expect(f.getByTestId("ticket-price").first()).toContainText("RON");
    await expectNoHorizontalScroll(page);
  });

  test("clicking a section in the preview opens its settings; the language switch shows Romanian with diacritics", async ({ page }) => {
    await setLocale(page, "en");
    await signInDemoOrganiser(page);
    await openBuilder(page);
    await page.getByTestId("preview-locale").selectOption("ro");
    await expect(frame(page).getByTestId("buy-disabled").first()).toHaveText("Bilete disponibile după publicare");
    await expect(frame(page).getByTestId("ticket-price").first()).toContainText(/\d,\d{2} RON/);
    await frame(page).locator("[data-eps-type=faq] h2").click();
    await showPane(page, "settings");
    await expect(page.getByTestId("section-form")).toHaveAttribute("data-section-type", "faq");
    await expectNoHorizontalScroll(page);
  });
});
