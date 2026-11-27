import { test as base, expect, type Page } from "@playwright/test";

const expected404 = new WeakSet<Page>();
/** For a test that opens a page expected to answer 404: the browser logs the status as an error. */
export function expectNotFoundResponse(page: Page) {
  expected404.add(page);
}

/** Fails the test on any console error or uncaught page error. */
export const test = base.extend<{ page: Page }>({
  page: async ({ page }, provide) => {
    const errors: string[] = [];
    page.on("console", (m) => {
      if (m.type() !== "error") return;
      if (expected404.has(page) && /status of 404/.test(m.text())) return;
      errors.push(m.text());
    });
    page.on("pageerror", (e) => errors.push(e.message));
    await provide(page);
    expect(errors, "console errors").toEqual([]);
  },
});

export { expect };

export async function expectNoHorizontalScroll(page: Page) {
  const overflow = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
  expect(overflow, "horizontal scroll").toBeLessThanOrEqual(0);
}

export async function setLocale(page: Page, locale: "ro" | "en") {
  await page.context().addCookies([
    { name: "NEXT_LOCALE", value: locale, url: process.env.PLAYWRIGHT_BASE_URL || "http://localhost:3000" },
  ]);
}

export const DEMO_DOMAIN = process.env.EPS_DEMO_EMAIL_DOMAIN || "demo.eps.invalid";

const rnd = () => Math.random().toString(36).slice(2, 8);
export function uniqueSlug(prefix: string) {
  return `${prefix}-${Date.now().toString(36)}-${rnd()}`.slice(0, 40);
}

/** Signs in as a fresh demo fan (a new user with no organisation). */
export async function signInFreshUser(page: Page) {
  await page.goto("/sign-in");
  await page.getByTestId("demo-fan").click();
  await expect(page).toHaveURL(/\/status$/);
}

export async function signInDemoOrganiser(page: Page) {
  await page.goto("/sign-in");
  await page.getByTestId("demo-organiser").click();
  await expect(page).toHaveURL(/\/studio/);
}

/** Signs in through the demo mailbox with a given demo-domain address (same user every time). */
export async function signInWithMailbox(page: Page, email: string, next = "/studio") {
  await page.goto(`/sign-in?next=${encodeURIComponent(next)}`);
  await page.locator('input[type="email"]').fill(email);
  await page.locator('form button[type="submit"]').first().click();
  const link = page.getByTestId("demo-mailbox").locator("a").first();
  await expect(link).toBeVisible();
  await page.goto((await link.getAttribute("href"))!);
}

/** Runs onboarding for the signed-in user (English UI); returns the slugs and brand page URL. */
export async function onboard(page: Page, opts: { org?: string; brand?: string; template?: string } = {}) {
  const orgSlug = uniqueSlug("org");
  const brandSlug = uniqueSlug("brand");
  await page.goto("/studio");
  await expect(page).toHaveURL(/\/studio\/onboarding$/);
  await page.getByLabel("Organisation name").fill(opts.org ?? "E2E Events");
  await page.getByLabel("Organisation address").fill(orgSlug);
  await page.getByRole("button", { name: "Continue" }).click();
  await expect(page.getByTestId("onboarding-step")).toHaveText("Step 2 of 2");
  await page.getByLabel("Brand name").fill(opts.brand ?? "E2E Festival");
  await page.getByLabel("Shop address").fill(brandSlug);
  await page.locator(`input[name=template][value=${opts.template ?? "festival"}]`).check();
  await page.getByRole("button", { name: "Create brand and open Studio" }).click();
  await expect(page).toHaveURL(/\/studio\/brands\/[0-9a-f-]{36}\?welcome=1$/);
  return { orgSlug, brandSlug, brandUrl: new URL(page.url()).pathname };
}

export const desktopOnly = (projectName: string) => projectName !== "desktop";
