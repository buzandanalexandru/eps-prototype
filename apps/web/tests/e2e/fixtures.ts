import { test as base, expect, type Page } from "@playwright/test";

/** Fails the test on any console error or uncaught page error. */
export const test = base.extend<{ page: Page }>({
  page: async ({ page }, provide) => {
    const errors: string[] = [];
    page.on("console", (m) => {
      if (m.type() === "error") errors.push(m.text());
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

export const DEMO_DOMAIN = process.env.EPS_DEMO_EMAIL_DOMAIN || "demo.eps-prototype.dev";
