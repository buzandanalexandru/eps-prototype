// Storefront builder spec: first visit, change template, brand kit (logo, contrast), sections
// (reorder, hide, keyboard), the locked checkout.
import { desktopOnly, expect, expectErrorResponse, expectNoHorizontalScroll, onboard, setLocale, signInDemoOrganiser, signInFreshUser, test } from "./fixtures";
import { colourPng, frame, frameTypes, openBuilder, saved, sectionTypes, showPane } from "./builder-helpers";

test.describe("builder", () => {
  test("first visit creates a draft from the onboarding template; change template keeps the brand's tokens", async ({ page }, info) => {
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
    await saved(page);
    // Change template to resort, with confirmation.
    await page.getByTestId("template-choice").selectOption("resort");
    await page.getByTestId("change-template").click();
    await page.getByTestId("confirm-template").click();
    await saved(page);
    await page.getByTestId("tab-pages").click();
    await page.getByTestId("section-row").first().getByTestId("section-select").click();
    await expect(page.getByTestId("field-image").locator("select")).toHaveValue("sample:resort-pool");
    await page.getByTestId("tab-brand").click();
    await expect(page.getByTestId("radius")).toHaveValue("none");
    await expect(page.getByTestId("colour-light-primary")).toHaveValue("#0b5394");
    await expect(frame(page).getByTestId("storefront")).toHaveAttribute("style", /--eps-l-primary:\s*#0b5394/);
    await expectNoHorizontalScroll(page);
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
  });

  test("logo upload shows in the preview header and proposes a palette; an SVG is refused; failing contrast is fixed by the suggestion", async ({ page }, info) => {
    test.skip(desktopOnly(info.project.name), "creates data");
    await setLocale(page, "en");
    await signInFreshUser(page);
    await onboard(page);
    await openBuilder(page);
    await page.getByTestId("tab-brand").click();
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

  test("the checkout tab is a locked preview with the two named slots", async ({ page }) => {
    await setLocale(page, "en");
    await signInDemoOrganiser(page);
    await openBuilder(page);
    await showPane(page, "structure");
    await page.getByTestId("tab-checkout").click();
    const preview = page.getByTestId("checkout-preview");
    await expect(preview).toBeVisible();
    await expect(page.getByTestId("checkout-locked-note")).toBeVisible();
    await expect(preview.getByTestId("slot-afterContact")).toContainText("checkout.slots.afterContact");
    await expect(preview.getByTestId("slot-consents")).toContainText("checkout.slots.consents");
    await expect(preview.locator("input, textarea, select")).toHaveCount(0);
    await expectNoHorizontalScroll(page);
  });
});
