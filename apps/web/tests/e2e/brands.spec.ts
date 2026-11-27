// Organisers spec "Several brands per organiser".
import { desktopOnly, expectNotFoundResponse, expect, expectNoHorizontalScroll, onboard, setLocale, signInDemoOrganiser, signInFreshUser, test, uniqueSlug } from "./fixtures";

test.describe("brands", () => {
  test("a second brand: both listed, each with its own /s/ address; settings save", async ({ page }, info) => {
    test.skip(desktopOnly(info.project.name), "creates data");
    await setLocale(page, "en");
    await signInFreshUser(page);
    const { brandSlug } = await onboard(page, { org: "NIBIRU-like Org", brand: "Festivals Brand" });
    await page.goto("/studio/brands");
    const form = page.getByTestId("new-brand-form");
    const second = uniqueSlug("resort");
    await form.getByLabel("Brand name").fill("Resort Brand");
    await form.getByLabel("Shop address").fill(second);
    await form.locator("input[name=template][value=resort]").check();
    await form.getByRole("button", { name: "Create brand" }).click();
    await expect(page.getByTestId("brand-title")).toHaveText("Resort Brand");
    await page.goto("/studio/brands");
    await expect(page.getByTestId("brand-address")).toHaveText([`/s/${brandSlug}`, `/s/${second}`]);
    // A slug already used (by any organiser) is refused without naming the holder.
    await form.getByLabel("Brand name").fill("Copy");
    await form.getByLabel("Shop address").fill(brandSlug);
    await form.getByRole("button", { name: "Create brand" }).click();
    await expect(form.getByTestId("error-slug")).toHaveText("This address is taken.");
    // Edit settings: languages and currency.
    await page.getByRole("link", { name: "Resort Brand" }).click();
    const settings = page.getByTestId("brand-settings");
    await settings.getByLabel("Currency").selectOption("EUR");
    await settings.getByRole("button", { name: "Save" }).click();
    await expect(settings.getByTestId("saved")).toBeVisible();
    await page.reload();
    await expect(page.getByTestId("brand-settings").getByLabel("Currency")).toHaveValue("EUR");
    await expectNoHorizontalScroll(page);
  });

  test("a member of organiser A opening organiser B's brand settings sees not found", async ({ page, browser }, info) => {
    test.skip(desktopOnly(info.project.name), "creates data");
    // B: the NIBIRU sample organiser's brand (read its URL as the demo organiser).
    const ctx = await browser.newContext();
    const demo = await ctx.newPage();
    await setLocale(demo, "en");
    await signInDemoOrganiser(demo);
    await demo.goto("/studio/brands");
    const href = await demo.getByTestId("brand-list").locator("a").first().getAttribute("href");
    await ctx.close();
    expect(href).toMatch(/^\/studio\/brands\/[0-9a-f-]{36}$/);

    await setLocale(page, "en");
    await signInFreshUser(page);
    await onboard(page);
    expectNotFoundResponse(page);
    const res = await page.goto(href!);
    expect(res?.status()).toBe(404);
    await expect(page.getByTestId("not-found")).toBeVisible();
    await expect(page.getByText("NIBIRU")).toHaveCount(0);
  });
});
