// Organisers spec "The platform fee is published": public, no sign-in, RO and EN.
import { expect, expectNoHorizontalScroll, setLocale, test } from "./fixtures";

for (const [locale, title, pass, fan] of [
  ["en", "Platform fees", "Pass on", "552.48 RON"],
  ["ro", "Comisioanele platformei", "Transferat", "552,48 RON"],
] as const) {
  test(`fees page in ${locale}: 2.5% and no fixed part, three policies and a worked example`, async ({ page }) => {
    await setLocale(page, locale);
    const res = await page.goto("/fees");
    expect(res?.status()).toBe(200);
    await expect(page.getByRole("heading", { level: 1 })).toHaveText(title);
    await expect(page.getByTestId("fee-percent")).toHaveText(locale === "ro" ? "2,5%" : "2.5%");
    await expect(page.getByTestId("fee-fixed")).toHaveText(locale === "ro" ? "Fără sumă fixă pe bilet." : "No fixed part per ticket.");
    for (const p of ["absorb", "pass_on", "split"]) await expect(page.getByTestId(`policy-${p}`)).toBeVisible();
    const row = page.getByTestId("example-pass_on");
    await expect(row).toContainText(pass);
    await expect(row.getByTestId("fan-pays")).toHaveText(fan);
    await expectNoHorizontalScroll(page);
  });
}
