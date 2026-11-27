import { DEMO_DOMAIN, expect, expectNoHorizontalScroll, setLocale, test } from "./fixtures";

test.describe("sign-in", () => {
  test("anonymous Studio visit redirects to sign-in and returns to Studio afterwards", async ({ page }) => {
    await setLocale(page, "en");
    await page.goto("/studio");
    await expect(page).toHaveURL(/\/sign-in\?next=%2Fstudio/);
    await page.getByTestId("demo-organiser").click();
    await expect(page).toHaveURL(/\/studio$/);
    await expect(page.getByTestId("studio-user")).toContainText("Demo organiser");
  });

  test("demo organiser opens Studio as the shared demo organiser", async ({ page }) => {
    await setLocale(page, "en");
    await page.goto("/sign-in");
    await page.getByTestId("demo-organiser").click();
    await expect(page).toHaveURL(/\/studio$/);
    await expect(page.getByTestId("studio-user")).toContainText("Demo organiser");
  });

  test("two browsers choosing demo fan are two different users", async ({ browser }) => {
    const subs: string[] = [];
    for (let i = 0; i < 2; i++) {
      const ctx = await browser.newContext();
      const page = await ctx.newPage();
      await page.goto("/sign-in");
      await page.getByTestId("demo-fan").click();
      await expect(page).toHaveURL(/\/status$/);
      const res = await page.request.get("/api/core-token");
      expect(res.status()).toBe(200);
      const { token } = await res.json();
      subs.push(JSON.parse(Buffer.from(token.split(".")[1], "base64url").toString()).sub);
      await ctx.close();
    }
    expect(subs[0]).not.toBe(subs[1]);
  });

  test("demo mailbox shows the magic link for a demo address, and it signs in once", async ({ page }) => {
    await setLocale(page, "en");
    const email = `mailbox-${Date.now()}@${DEMO_DOMAIN}`;
    await page.goto("/sign-in?next=%2Fstudio");
    await page.getByLabel("Email address").fill(email);
    await page.getByRole("button", { name: "Send sign-in link" }).click();
    const mailbox = page.getByTestId("demo-mailbox");
    await expect(mailbox).toBeVisible();
    const href = await mailbox.getByRole("link", { name: "Open sign-in link" }).first().getAttribute("href");
    expect(href).toBeTruthy();
    await page.goto(href!);
    await expect(page).toHaveURL(/\/studio$/);
    await expect(page.getByTestId("studio-user")).toContainText(email.split("@")[0].slice(0, 5));

    // The same link a second time: no session, offer a new link.
    await page.context().clearCookies();
    await setLocale(page, "en");
    await page.goto(href!);
    await expect(page.getByTestId("link-error")).toBeVisible();
  });

  for (const [locale, title] of [
    ["ro", "Autentifică-te în EPS"],
    ["en", "Sign in to EPS"],
  ] as const) {
    test(`sign-in page in ${locale}: text, no horizontal scroll`, async ({ page }) => {
      await setLocale(page, locale);
      await page.goto("/sign-in");
      await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
      await expectNoHorizontalScroll(page);
    });
  }
});
