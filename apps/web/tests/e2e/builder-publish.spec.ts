// Storefront builder spec: first publish and the address state, rollback, private preview links.
import { desktopOnly, expect, expectNotFoundResponse, onboard, setLocale, signInDemoOrganiser, signInFreshUser, test } from "./fixtures";
import { frame, openBuilder, saved } from "./builder-helpers";

test.describe("builder publish", () => {
  test("first publish: version 1 is live with time and author, the address is live; rollback publishes a new version", async ({ page }, info) => {
    test.skip(desktopOnly(info.project.name), "creates data");
    await setLocale(page, "en");
    await signInFreshUser(page);
    await onboard(page);
    await openBuilder(page);
    await expect(page.getByTestId("address-state")).toHaveText("Not published yet");
    await expect(page.getByTestId("brand-address")).toHaveText(/^\/s\/brand-/);
    await page.getByTestId("tab-publish").click();
    await expect(page.getByTestId("no-problems")).toBeVisible();
    await page.getByTestId("publish").click();
    await expect(page.getByTestId("publish-message")).toHaveText("Version 1 is live.");
    await expect(page.getByTestId("address-state")).toHaveText("Published · version 1");
    await expect(page.getByTestId("address-note")).toContainText("step 3");
    await expect(page.getByTestId("version")).toHaveCount(1);
    await expect(page.getByTestId("version").first()).toContainText(/v1.*\d{4}/);
    // Version 2 with a new title, then restore version 1 as version 3.
    await page.getByTestId("tab-pages").click();
    await page.getByTestId("preview-locale").selectOption("en");
    await page.locator("[data-testid=section-row][data-section-type=hero]").getByTestId("section-select").click();
    await page.getByTestId("field-heading").locator("input[data-locale=en]").fill("Version two");
    await page.getByTestId("field-heading").locator("input[data-locale=ro]").fill("Versiunea doi");
    await saved(page);
    await page.getByTestId("tab-publish").click();
    await page.getByTestId("publish").click();
    await expect(page.getByTestId("address-state")).toHaveText("Published · version 2");
    await page.locator("[data-testid=version][data-number='1']").getByTestId("restore").click();
    await expect(page.getByRole("alertdialog")).toContainText("unpublished edits");
    await expect(page.getByRole("alertdialog")).toContainText("prices always come from the current catalogue");
    await page.getByTestId("confirm-restore").click();
    await expect(page.getByTestId("address-state")).toHaveText("Published · version 3");
    await expect(page.locator("[data-testid=version][data-number='3']")).toContainText("restored from v1");
    await expect(page.getByTestId("version")).toHaveCount(3);
    await expect(frame(page).locator("[data-eps-type=hero] h1")).not.toHaveText("Version two");
    await page.reload();
    await expect(page.getByTestId("address-state")).toHaveText("Published · version 3");
  });

  test("a preview link shows the draft to a signed-out visitor with the banner and no buying; revoked it is not found", async ({ page, browser }, info) => {
    test.skip(desktopOnly(info.project.name), "creates a link");
    await setLocale(page, "en");
    await signInDemoOrganiser(page);
    await openBuilder(page);
    await page.getByTestId("tab-publish").click();
    await page.getByTestId("create-link").click();
    const url = await page.getByTestId("fresh-link-url").textContent();
    expect(url).toMatch(/\/p\/[A-Za-z0-9_-]{22}$/);
    const path = new URL(url!).pathname;
    const visitor = await (await browser.newContext()).newPage();
    await setLocale(visitor, "en");
    const res = await visitor.goto(path);
    expect(res?.status()).toBe(200);
    expect(res?.headers()["cache-control"]).toMatch(/no-store/);
    expect(res?.headers()["x-robots-tag"]).toMatch(/noindex/);
    await expect(visitor.getByTestId("preview-banner")).toContainText("Preview, not published");
    await expect(visitor.locator('meta[name="robots"]')).toHaveAttribute("content", /noindex/);
    await expect(visitor.getByTestId("buy-disabled").first()).toHaveText("Tickets available after publishing");
    await expect(visitor.getByTestId("buy-disabled").first()).toBeDisabled();
    await expect(visitor.getByTestId("shop-sample-label")).toBeVisible();
    await expect(visitor.getByTestId("shop-privacy")).toContainText("no outside services");
    // The map, when shown, is an image from our own origin (never a map service).
    for (const src of await visitor.locator("img").evaluateAll((els) => els.map((e) => (e as HTMLImageElement).src))) expect(new URL(src).origin).toBe(new URL(visitor.url()).origin);
    expect(res?.headers()["x-frame-options"]).toBe("DENY");
    // Revoke: the link is gone.
    await page.getByTestId("link-row").first().getByTestId("revoke-link").click();
    await expect(page.getByTestId("link-row").first()).toContainText("revoked");
    expectNotFoundResponse(visitor);
    const gone = await visitor.goto(path);
    expect(gone?.status()).toBe(404);
    const guess = await visitor.goto("/p/AAAAAAAAAAAAAAAAAAAAAA");
    expect(guess?.status()).toBe(404);
    await visitor.context().close();
  });
});
