// Waiting-room spec (step 5, E2-E4): a fan who arrives 20 minutes before opening is in the
// pre-queue with a countdown to the draw; a reload keeps the place. On an open sale the checkout
// sends the fan to the waiting room, a paused sale says the place is safe, and once the organiser
// resumes the fan is let in with a pass and the checkout holds the tickets.
import { desktopOnly, expect, expectNoHorizontalScroll, test } from "./fixtures";
import { addSale, expectNoConsoleErrors, fan, fillSale, pickAndContinue, publishedEvent } from "./onsale-helpers";

test("Join early: the pre-queue counts down to the draw and a reload keeps the place (EN desktop, RO phone)", async ({ page, browser }, info) => {
  test.skip(desktopOnly(info.project.name), "creates data");
  test.setTimeout(300_000);
  const { brandSlug, eventUrl } = await publishedEvent(page);
  const sale = await addSale(page, eventUrl, { opensIn: 20, prequeueMinutes: 30 });

  for (const [phone, locale] of [[false, "en"], [true, "ro"]] as const) {
    const f = await fan(browser, { phone, locale });
    await f.goto(`/s/${brandSlug}/${locale}/queue?sale=${sale}`);
    const room = f.getByTestId("waiting-room");
    await expect(room).toHaveAttribute("data-state", "prequeue", { timeout: 20_000 });
    await expect(f.getByTestId("queue-countdown")).toBeVisible();
    await expect(f.getByTestId("queue-prequeue")).toContainText(locale === "en" ? "arriving earlier gives no advantage" : "venitul mai devreme nu aduce niciun avantaj");
    const place = await f.getByTestId("queue-place").getAttribute("data-place");
    expect(place).toMatch(/^[0-9A-F]{6}$/);
    // Scenario "Reload keeps place": same place after a reload and in a second tab.
    await f.reload();
    await expect(room).toHaveAttribute("data-state", "prequeue", { timeout: 20_000 });
    await expect(f.getByTestId("queue-place")).toHaveAttribute("data-place", place!);
    const tab = await f.context().newPage();
    await tab.goto(`/s/${brandSlug}/${locale}/queue?sale=${sale}`);
    await expect(tab.getByTestId("queue-place")).toHaveAttribute("data-place", place!, { timeout: 20_000 });
    await expectNoHorizontalScroll(f);
    expectNoConsoleErrors(f);
    await f.context().close();
  }
  // Studio counts the two places in the pre-queue.
  await page.goto(`${eventUrl}/sales`);
  await expect(page.getByTestId("sale-queue-prequeue")).toHaveText("2");
});

test("Checkout sends the fan to the waiting room; paused, then admitted with a pass (scenario Pause)", async ({ page, browser }, info) => {
  test.skip(desktopOnly(info.project.name), "creates data");
  test.setTimeout(300_000);
  const { brandSlug, eventSlug, eventUrl } = await publishedEvent(page);
  const sale = await addSale(page, eventUrl, { opensIn: -1, paused: true, botDefence: true });

  const f = await fan(browser, { locale: "en" });
  await pickAndContinue(f, brandSlug, eventSlug, 2);
  await expect(f).toHaveURL(new RegExp(`/queue\\?sale=${sale}&next=`), { timeout: 20_000 });
  await expect(f.getByTestId("waiting-room")).toHaveAttribute("data-state", "waiting", { timeout: 20_000 });
  await expect(f.getByTestId("queue-paused")).toHaveText("Paused for a moment, your place is safe");
  await expect(f.getByTestId("queue-ahead")).toHaveAttribute("data-ahead", "0");

  // The organiser resumes admissions: within seconds the fan is let in and the checkout holds.
  const form = page.locator(`[data-sale-id="${sale}"]`).getByTestId("sale-form");
  await fillSale(form, { paused: false });
  await form.getByTestId("sale-save").click();
  await expect(page.getByTestId("sale-saved")).toHaveText("Saved as version 2.");
  await expect(f).toHaveURL(/\/checkout\?order=[0-9a-f-]{36}/, { timeout: 40_000 });
  await expect(f.getByTestId("order-state")).toHaveAttribute("data-state", "held");
  await expectNoHorizontalScroll(f);
  expectNoConsoleErrors(f);

  // Studio shows one fan admitted and shopping.
  await page.goto(`${eventUrl}/sales`);
  await expect(page.getByTestId("sale-queue-admitted")).toHaveText("1");
  await expect(page.getByTestId("sale-queue-active")).toHaveText("1");
});
