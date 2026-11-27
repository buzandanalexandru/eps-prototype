// Organisers spec "Self-serve organiser sign-up".
import { DEMO_DOMAIN, desktopOnly, expect, expectNoHorizontalScroll, onboard, setLocale, signInFreshUser, signInWithMailbox, test, uniqueSlug } from "./fixtures";

test.describe("organiser onboarding", () => {
  test("complete onboarding: organisation, brand with the festival template, owner, Studio on the brand", async ({ page }, info) => {
    test.skip(desktopOnly(info.project.name), "creates data; the phone layout is checked in sample-data.spec.ts");
    await setLocale(page, "en");
    await signInFreshUser(page);
    const { brandSlug } = await onboard(page, { org: "Sunwave Events", brand: "Sunwave Festival", template: "festival" });
    await expect(page.getByTestId("brand-title")).toHaveText("Sunwave Festival");
    await expect(page.getByTestId("brand-address")).toHaveText(`/s/${brandSlug}`);
    await expect(page.getByTestId("brand-template")).toHaveText("Festival");
    await expect(page.getByTestId("active-organiser")).toHaveText("Sunwave Events");
    await page.goto("/studio/team");
    await expect(page.getByTestId("member-role")).toContainText("Owner");
    await expectNoHorizontalScroll(page);
  });

  test("interrupted onboarding resumes at the brand step instead of creating a second organisation", async ({ page, browser }, info) => {
    test.skip(desktopOnly(info.project.name), "creates data");
    const email = `resume-${Date.now()}@${DEMO_DOMAIN}`;
    await setLocale(page, "en");
    await signInWithMailbox(page, email);
    await expect(page).toHaveURL(/\/studio\/onboarding$/);
    await page.getByLabel("Organisation name").fill("Resume Events");
    await page.getByLabel("Organisation address").fill(uniqueSlug("resume"));
    await page.getByRole("button", { name: "Continue" }).click();
    await expect(page.getByTestId("onboarding-step")).toHaveText("Step 2 of 2");

    // "Closes the browser": a new browser context, signed in again as the same address.
    const ctx = await browser.newContext();
    const again = await ctx.newPage();
    await setLocale(again, "en");
    await signInWithMailbox(again, email);
    await expect(again).toHaveURL(/\/studio\/onboarding$/);
    await expect(again.getByTestId("onboarding-step")).toHaveText("Step 2 of 2");
    await expect(again.getByRole("heading", { level: 1 })).toContainText("Resume Events");
    await ctx.close();
  });

  test("slug rules are explained in Romanian with diacritics", async ({ page }, info) => {
    test.skip(desktopOnly(info.project.name), "creates a user");
    await setLocale(page, "ro");
    await signInFreshUser(page);
    await page.goto("/studio/onboarding");
    await expect(page.getByRole("heading", { level: 1 })).toHaveText("Creează-ți organizația");
    await page.getByLabel("Numele organizației").fill("Test");
    await page.getByLabel("Adresa organizației").fill("studio");
    await page.getByRole("button", { name: "Continuă" }).click();
    await expect(page.getByTestId("error-slug")).toHaveText("Adresa este rezervată de platformă.");
    await page.getByLabel("Adresa organizației").fill("Brașov");
    await page.getByRole("button", { name: "Continuă" }).click();
    await expect(page.getByTestId("error-slug")).toContainText("fără spații sau diacritice");
    await expectNoHorizontalScroll(page);
  });
});
