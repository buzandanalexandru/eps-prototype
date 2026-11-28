// Tickets spec: issuing shown honestly, the rotating signed code, the in-memory buffer running out
// offline, the signed guest link and the wallet pass preview.
import { expect, expectNoHorizontalScroll, expectOfflineErrors, setLocale, signInFreshUser, test } from "./fixtures";
import { buyGalaxia, currentCode, nameTicket, walletTickets } from "./ticket-helpers";

test("Status page shows issuing honestly, then opens the tickets", async ({ page }) => {
  await signInFreshUser(page);
  await buyGalaxia(page);
  const state = await page.getByTestId("order-state").getAttribute("data-state");
  if (state === "paid") await expect(page.getByTestId("issuing-tickets")).toContainText("Issuing your tickets");
  // Never "tickets ready" before they are issued; once issued the page links to them.
  await expect(page.getByTestId("order-state")).toHaveAttribute("data-state", "issued", { timeout: 30_000 });
  await page.getByTestId("open-tickets").click();
  await expect(page).toHaveURL(/\/wallet$/);
  await expect(page.getByTestId("trip-item")).toHaveCount(1);
});

test("Code changes each window: three codes in 45 seconds with a countdown", async ({ page }, info) => {
  test.skip(info.project.name === "phone", "one project is enough for a 45-second wait");
  test.setTimeout(120_000);
  await signInFreshUser(page);
  await buyGalaxia(page);
  const [id] = await walletTickets(page);
  await page.goto(`/wallet/tickets/${id}`);
  // Galaxia passes are named: no code until the ticket has a name.
  await expect(page.getByTestId("live-code")).toHaveCount(0);
  await nameTicket(page);
  await expect(page.getByTestId("countdown")).toContainText(/New code in \d+ s/);
  const seen = new Map<number, string>();
  const start = Date.now();
  while (Date.now() - start < 46_000) {
    const c = await currentCode(page);
    seen.set(c.window, c.code);
    await page.waitForTimeout(1000);
  }
  expect(seen.size).toBeGreaterThanOrEqual(3);
  expect(new Set(seen.values()).size).toBe(seen.size);
  await expectNoHorizontalScroll(page);
});

test("Offline after the buffer runs out: reconnect, no QR", async ({ page, context }) => {
  test.setTimeout(90_000);
  await signInFreshUser(page);
  await buyGalaxia(page);
  const [id] = await walletTickets(page);
  await page.clock.install({ time: new Date() });
  await page.goto(`/wallet/tickets/${id}`);
  await nameTicket(page, "Ion Pop");
  expectOfflineErrors(page);
  await context.setOffline(true);
  // Past the 10-minute buffer: the page never shows a stale code.
  await page.clock.fastForward("11:00");
  await expect(page.getByTestId("reconnect")).toContainText("Reconnect to show your code");
  await expect(page.getByTestId("live-code")).toHaveCount(0);
  await context.setOffline(false);
});

test("Guest opens the signed link: live codes without an account, an offer to claim, printable without a code", async ({ page }) => {
  await buyGalaxia(page);
  await expect(page.getByTestId("order-state")).toHaveAttribute("data-state", "issued", { timeout: 30_000 });
  await page.getByTestId("open-tickets").click();
  await expect(page).toHaveURL(/\/t\/[A-Za-z0-9_.-]+$/);
  await expect(page.getByTestId("live-ticket")).toHaveCount(1);
  await nameTicket(page, "Maria Ionescu");
  await expect(page.getByTestId("claim-offer")).toBeVisible();
  await page.emulateMedia({ media: "print" });
  await expect(page.getByTestId("live-code")).toBeHidden(); // a printout carries no code…
  await expect(page.getByText("printed tickets carry no entry code")).toBeVisible(); // …and says where to find it
  await page.emulateMedia({ media: "screen" });
  // A changed link shows nothing of the order.
  const url = new URL(page.url());
  await page.goto(url.pathname.slice(0, -3) + "AAA");
  await expect(page.getByTestId("link-invalid")).toContainText("Link not valid");
  await expect(page.getByTestId("live-ticket")).toHaveCount(0);
});

test("Wallet pass preview in English and Romanian", async ({ page }) => {
  await signInFreshUser(page);
  await buyGalaxia(page);
  const [id] = await walletTickets(page);
  await page.goto(`/wallet/tickets/${id}`);
  await nameTicket(page, "Ana Pop");
  await page.getByTestId("add-google-wallet").click();
  await expect(page.getByTestId("pass-preview")).toContainText("Galaxia 2027");
  await expect(page.getByTestId("pass-preview")).toContainText("Ana Pop");
  await expect(page.getByTestId("pass-note")).toContainText("real pass needs the organiser's Apple and Google signing accounts");
  await page.getByTestId("pass-close").click();
  await setLocale(page, "ro");
  await page.reload();
  await page.getByTestId("add-apple-wallet").click();
  await expect(page.getByTestId("pass-note")).toContainText("conturile de semnare Apple și Google");
  await expectNoHorizontalScroll(page);
});
