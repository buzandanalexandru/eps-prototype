import { expect, expectNoHorizontalScroll, test } from "./fixtures";

// Production smoke test after a merge to main.
test("home, sign-in and status render", async ({ page }) => {
  for (const path of ["/", "/sign-in", "/status"]) {
    const res = await page.goto(path);
    expect(res?.status(), path).toBeLessThan(400);
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    await expectNoHorizontalScroll(page);
  }
  if (process.env.EXPECT_WEB_COMMIT) await expect(page.getByTestId("web-commit")).toHaveText(process.env.EXPECT_WEB_COMMIT);
  await expect(page.getByTestId("core-commit")).toBeVisible();
  if (process.env.EXPECT_CORE_COMMIT)
    await expect(page.getByTestId("core-commit")).toHaveText(process.env.EXPECT_CORE_COMMIT);
});
