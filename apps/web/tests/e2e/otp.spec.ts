// OTP spec (step 5, E12): with OTP on and a threshold of 4, an ordinary fan buying 2 goes straight
// to payment; a fan buying 6 must enter the code shown in the simulated SMS panel first.
import { desktopOnly, expect, expectNoHorizontalScroll, test } from "./fixtures";
import { expectOrderState, giveDetails, pressPay } from "./checkout-helpers";
import { addSale, expectNoConsoleErrors, fan, pickAndContinue, publishedEvent } from "./onsale-helpers";

test("Ordinary fan skips OTP; a bulk order enters the simulated code, then pays (EN desktop, RO phone)", async ({ page, browser }, info) => {
  test.skip(desktopOnly(info.project.name), "creates data");
  test.setTimeout(300_000);
  const { brandSlug, eventSlug, eventUrl } = await publishedEvent(page);
  await addSale(page, eventUrl, { opensIn: -1, queueOn: false, otpOn: true, otpThreshold: 4, capPerOrder: 8, capPerPerson: 10 });

  // Scenario "Ordinary fan skips OTP".
  const a = await fan(browser, { locale: "en" });
  await pickAndContinue(a, brandSlug, eventSlug, 2);
  await expect(a).toHaveURL(/\/checkout\?order=[0-9a-f-]{36}/, { timeout: 30_000 });
  await expect(a.getByTestId("order-state")).toHaveAttribute("data-state", "held");
  await expect(a.getByTestId("otp")).toHaveCount(0);
  await expect(a.getByTestId("pay")).toBeEnabled();
  expectNoConsoleErrors(a);

  // Scenario "Bulk order asked" (Romanian, phone).
  const b = await fan(browser, { phone: true, locale: "ro" });
  await pickAndContinue(b, brandSlug, eventSlug, 6, "ro");
  await expect(b).toHaveURL(/\/checkout\?order=[0-9a-f-]{36}/, { timeout: 30_000 });
  await expect(b.getByTestId("otp")).toBeVisible();
  await expect(b.getByTestId("otp")).toContainText("Confirmă că ești tu");
  await expect(b.getByTestId("pay")).toBeDisabled();
  await b.getByTestId("otp-send").click();
  const sms = b.getByTestId("otp-sms-message");
  await expect(sms).toContainText("Codul tău EPS este");
  const code = /(\d{6})/.exec((await sms.textContent()) ?? "")![1];
  await b.getByTestId("otp-code").fill(code === "000000" ? "111111" : "000000");
  await b.getByTestId("otp-verify").click();
  await expect(b.getByTestId("otp-error")).toContainText("Codul nu este corect.");
  await b.getByTestId("otp-code").fill(code);
  await b.getByTestId("otp-verify").click();
  await expect(b.getByTestId("otp")).toHaveCount(0);
  await expect(b.getByTestId("pay")).toBeEnabled();
  await expectNoHorizontalScroll(b);
  await giveDetails(b);
  await pressPay(b, "hosted_card");
  await b.getByTestId("psp-approve").click();
  await expectOrderState(b, /paid|issued/);
  expectNoConsoleErrors(b);
});
