import { expect, expectNoHorizontalScroll, test } from "./fixtures";

// Production smoke test after a merge to main.
test("home, sign-in and status render", async ({ page }) => {
  for (const path of ["/", "/sign-in", "/status"]) {
    const res = await page.goto(path);
    expect(res?.status(), path).toBeLessThan(400);
    const h1 = page.getByRole("heading", { level: 1 });
    const shown = await h1.waitFor({ state: "visible", timeout: 5000 }).then(
      () => true,
      () => false,
    );
    if (!shown) {
      // Name the page that came back (a wrong base URL or a Vercel interstitial) in the failure.
      const text = (
        (await page
          .locator("body")
          .innerText()
          .catch(() => "")) || ""
      )
        .replace(/\s+/g, " ")
        .slice(0, 300);
      const u = new URL(page.url()); // no query string or fragment in the log
      throw new Error(
        `${u.origin}${u.pathname}: no visible h1; title "${await page.title()}"; text "${text}"`,
      );
    }
    await expectNoHorizontalScroll(page);
  }
  if (process.env.EXPECT_WEB_COMMIT)
    await expect(page.getByTestId("web-commit")).toHaveText(
      process.env.EXPECT_WEB_COMMIT,
    );
  await expect(page.getByTestId("core-commit")).toBeVisible();
  if (process.env.EXPECT_CORE_COMMIT)
    await expect(page.getByTestId("core-commit")).toHaveText(
      process.env.EXPECT_CORE_COMMIT,
    );
});

// The production domain serves the same commit (a missing or rolled-back alias fails the deploy).
test("production domain serves this commit", async ({ page }, info) => {
  const domain = process.env.PRODUCTION_DOMAIN_URL;
  const sha = process.env.EXPECT_WEB_COMMIT;
  test.skip(
    !domain || !sha || info.project.name !== "desktop",
    "only after a production deploy, once",
  );
  test.setTimeout(240_000);
  await expect(async () => {
    await page.goto(new URL("/status", domain).toString());
    await expect(page.getByTestId("web-commit")).toHaveText(sha!, {
      timeout: 5000,
    });
  }).toPass({ timeout: 180_000, intervals: [10_000] });
});
