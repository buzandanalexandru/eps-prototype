// Storefront builder spec: first visit, change template (tokens, fonts and logo kept), brand kit
// (logo, contrast), sections (reorder, hide, keyboard), two tabs editing one draft.
import { desktopOnly, expect, expectErrorResponse, expectNoHorizontalScroll, onboard, setLocale, signInFreshUser, test } from "./fixtures";
import { colourPng, frame, frameTypes, openBuilder, saved, sectionTypes } from "./builder-helpers";

test.describe("builder", () => {
  test("first visit creates a draft from the onboarding template; change template keeps the brand's tokens; logo upload and contrast fix", async ({ page }, info) => {
    test.skip(desktopOnly(info.project.name), "creates data");
    await setLocale(page, "en");
    await signInFreshUser(page);
    await onboard(page, { template: "festival" });
    await openBuilder(page);
    expect(await sectionTypes(page)).toEqual(["hero", "text", "gallery", "faq", "footer"]);
    await expect(page.getByTestId("address-state")).toHaveText("Not published yet");
    await expect(frame(page).locator("[data-eps-type=hero] h1")).toBeVisible();
    // Brand kit: square corners and a new primary colour.
    await page.getByTestId("tab-brand").click();
    await page.getByTestId("radius").selectOption("none");
    await page.getByTestId("colour-light-primary").fill("#0b5394");
    await page.getByTestId("fontHeading").selectOption("lora");
    await page.getByTestId("logo").locator("select").selectOption("sample:logo-sunwave");
    await saved(page);
    // Change template to resort, with confirmation.
    const revisionBefore = await page.getByTestId("builder").getAttribute("data-revision");
    await page.getByTestId("template-choice").selectOption("resort");
    await page.getByTestId("change-template").click();
    await page.getByTestId("confirm-template").click();
    await expect(page.getByTestId("confirm-template")).toHaveCount(0);
    await expect(page.getByTestId("builder")).not.toHaveAttribute("data-revision", revisionBefore!);
    await saved(page);
    await page.getByTestId("tab-pages").click();
    await page.getByTestId("section-row").first().getByTestId("section-select").click();
    await expect(page.getByTestId("field-image").locator("select")).toHaveValue("sample:resort-pool");
    await page.getByTestId("tab-brand").click();
    await expect(page.getByTestId("radius")).toHaveValue("none");
    await expect(page.getByTestId("colour-light-primary")).toHaveValue("#0b5394");
    await expect(page.getByTestId("fontHeading")).toHaveValue("lora");
    await expect(page.getByTestId("logo").locator("select")).toHaveValue("sample:logo-sunwave");
    await expect(frame(page).getByTestId("shop-logo")).toHaveAttribute("src", "/samples/logo-sunwave.svg");
    await expect(frame(page).getByTestId("storefront")).toHaveAttribute("style", /--eps-l-primary:\s*#0b5394/);
    await expectNoHorizontalScroll(page);
    // Logo upload, palette proposal, refused SVG and the contrast suggestion (one sign-in: the
    // demo sign-in is rate limited per address on previews).
    expectErrorResponse(page, 422); // the refused SVG upload
    await page.getByTestId("logo-upload").setInputFiles({ name: "logo.svg", mimeType: "image/svg+xml", buffer: Buffer.from('<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>') });
    await expect(page.getByTestId("upload-error")).toContainText("SVG");
    await page.getByTestId("logo-upload").setInputFiles({ name: "logo.png", mimeType: "image/png", buffer: colourPng([20, 120, 220]) });
    await expect(frame(page).getByTestId("shop-logo")).toBeVisible();
    await expect(page.getByTestId("palette-proposal")).toBeVisible();
    await page.getByTestId("accept-proposal").click();
    await expect(page.getByTestId("contrast-pair").and(page.locator("[data-ok=false]"))).toHaveCount(0);
    // White text on light yellow buttons fails; the suggestion fixes it.
    await page.getByTestId("mode").selectOption("light");
    await page.getByTestId("colour-light-onPrimary").fill("#ffffff");
    await page.getByTestId("colour-light-primary").fill("#fde047");
    const failing = page.locator("[data-testid=contrast-pair][data-ok=false]");
    await expect(failing.first()).toBeVisible();
    while ((await failing.count()) > 0) await failing.first().getByTestId("accept-suggestion").click();
    await page.getByTestId("tab-publish").click();
    await expect(page.getByTestId("no-problems")).toBeVisible();
  });

  test("reorder by dragging and with the keyboard, hide a section; the footer stays last", async ({ page }, info) => {
    test.skip(desktopOnly(info.project.name), "creates data");
    await setLocale(page, "en");
    await signInFreshUser(page);
    await onboard(page);
    await openBuilder(page);
    // Keyboard: move the FAQ up twice with its "up" control (focus and Enter).
    const faq = page.locator("[data-testid=section-row][data-section-type=faq]");
    await faq.getByTestId("move-up").focus();
    await page.keyboard.press("Enter");
    await faq.getByTestId("move-up").focus();
    await page.keyboard.press("Enter");
    expect(await sectionTypes(page)).toEqual(["hero", "faq", "text", "gallery", "footer"]);
    // Drag: the gallery onto the hero's place.
    await page.locator("[data-testid=section-row][data-section-type=gallery]").dragTo(page.locator("[data-testid=section-row][data-section-type=hero]"));
    expect(await sectionTypes(page)).toEqual(["gallery", "hero", "faq", "text", "footer"]);
    // Hide the gallery: it stays in the draft and leaves the preview.
    await page.locator("[data-testid=section-row][data-section-type=gallery]").getByTestId("toggle-hidden").click();
    await expect(page.locator("[data-testid=section-row][data-section-type=gallery]")).toHaveAttribute("data-hidden", "true");
    await expect.poll(() => frameTypes(page)).toEqual(["hero", "faq", "text", "footer"]);
    await expect(page.locator("[data-testid=section-row][data-section-type=footer]").getByTestId("remove-section")).toHaveCount(0);
    await saved(page);
    await page.reload();
    expect(await sectionTypes(page)).toEqual(["gallery", "hero", "faq", "text", "footer"]);
    await expect(page.locator("[data-testid=section-row][data-section-type=gallery]")).toHaveAttribute("data-hidden", "true");
    // Two tabs: the other tab saves first; this tab's next save is refused and offers to reload.
    await openBuilder(page);
    const other = await page.context().newPage();
    await openBuilder(other);
    await other.locator("[data-testid=section-row][data-section-type=text]").getByTestId("toggle-hidden").click();
    await saved(other);
    expectErrorResponse(page, 409);
    await page.locator("[data-testid=section-row][data-section-type=faq]").getByTestId("toggle-hidden").click();
    await expect(page.getByTestId("conflict")).toBeVisible({ timeout: 10_000 });
    await expect(page.getByTestId("save-state")).toHaveText("Not saved: changed elsewhere");
    await page.getByTestId("conflict-reload").click();
    await expect(page.getByTestId("conflict")).toHaveCount(0);
    await expect(page.locator("[data-testid=section-row][data-section-type=text]")).toHaveAttribute("data-hidden", "true");
    await expect(page.locator("[data-testid=section-row][data-section-type=faq]")).toHaveAttribute("data-hidden", "false");
    await other.close();
  });
});
