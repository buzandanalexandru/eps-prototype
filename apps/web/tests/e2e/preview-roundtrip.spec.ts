import { expect, expectNoHorizontalScroll, setLocale, test } from "./fixtures";

// The step-0 round trip: browser → web (Vercel) → Neon, and → Go core (DOKS) with a core token.
// EXPECT_WEB_COMMIT / EXPECT_CORE_COMMIT / CORE_URL are set by the preview workflow.

test("signed-in status page shows web and core commits, schema versions and the whoami echo", async ({ page }) => {
  await setLocale(page, "en");
  await page.goto("/sign-in?next=%2Fstatus");
  await page.getByTestId("demo-organiser").click();
  await expect(page).toHaveURL(/\/status$/);
  await expect(page.getByTestId("db-state")).toHaveText("Ready");
  await expect(page.getByTestId("schema-control")).toHaveText(/^\d+$/);
  await expect(page.getByTestId("schema-sales")).toHaveText(/^\d+$/);
  if (process.env.EXPECT_WEB_COMMIT) await expect(page.getByTestId("web-commit")).toHaveText(process.env.EXPECT_WEB_COMMIT);
  await expect(page.getByTestId("core-commit")).toBeVisible();
  if (process.env.EXPECT_CORE_COMMIT)
    await expect(page.getByTestId("core-commit")).toHaveText(process.env.EXPECT_CORE_COMMIT);
  await expect(page.getByTestId("whoami-kind")).toHaveText(/^(organiser|fan)$/);
  await expect(page.getByTestId("whoami-sub")).not.toBeEmpty();
  await expectNoHorizontalScroll(page);
});

test("the core answers /healthz (over HTTPS on a deployment)", async ({ request }) => {
  const core = process.env.CORE_URL || "http://localhost:8080";
  const res = await request.get(`${core}/healthz`);
  expect(res.status()).toBe(200);
  expect(await res.json()).toEqual({ status: "ok" });
  if (process.env.CORE_URL) expect(core.startsWith("https://")).toBe(true);
  const who = await request.get(`${core}/v1/whoami`);
  expect(who.status()).toBe(401);
  expect(who.headers()["content-type"]).toContain("application/problem+json");
});
