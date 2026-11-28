// Fan account spec: a claimed guest order in the account and the wallet, paying the next
// instalment, and deleting one's data.
import { expect, expectNoHorizontalScroll, expectNotFoundResponse, signInFreshUser, signInWithMailbox, DEMO_DOMAIN, test } from "./fixtures";
import { expectOrderState, giveDetails, holdGalaxia } from "./checkout-helpers";
import { buyGalaxia, walletTickets } from "./ticket-helpers";

test("Claimed guest order: it appears in the account and the trip wallet", async ({ page }) => {
  test.setTimeout(90_000);
  const email = `claim-${Date.now()}-${Math.random().toString(36).slice(2, 6)}@${DEMO_DOMAIN}`;
  await buyGalaxia(page, { email }); // as a guest
  await expect(page.getByTestId("order-state")).toHaveAttribute("data-state", "issued", { timeout: 30_000 });
  const link = new URL(await page.getByTestId("order-link").inputValue());
  await signInWithMailbox(page, email, link.pathname + link.search);
  await page.getByTestId("add-to-account").click();
  await expect(page.getByTestId("claimed")).toBeVisible();
  await page.goto("/account");
  await expect(page.getByTestId("account-order")).toHaveCount(1);
  await expect(page.getByTestId("account-order").first()).toHaveAttribute("data-state", "issued");
  // The claim moves the tickets to the account (a reissue by the worker) within seconds.
  await expect(async () => {
    await page.reload();
    await expect(page.getByTestId("account-ticket")).toHaveCount(1, { timeout: 1000 });
  }).toPass({ timeout: 30_000 });
  await walletTickets(page, 1);
  await expectNoHorizontalScroll(page);
});

test("Pay the next instalment: the payment shows as paid and the count goes up", async ({ page }) => {
  test.setTimeout(120_000);
  await signInFreshUser(page);
  await holdGalaxia(page);
  await giveDetails(page);
  await page.getByTestId("plan-instalments").check();
  await page.getByTestId("method-hosted_card").locator("input").check();
  await page.getByTestId("pay").click();
  await page.getByTestId("psp-approve").click();
  await expectOrderState(page, /^(paid|issued)$/);
  await page.goto("/account");
  const plan = page.getByTestId("plan");
  await expect(plan).toHaveAttribute("data-paid", "0");
  await plan.getByTestId("plan-pay-now").click();
  await expect(async () => {
    await page.reload();
    await expect(page.getByTestId("plan")).toHaveAttribute("data-paid", "1", { timeout: 2000 });
  }).toPass({ timeout: 45_000 });
  await expect(page.getByTestId("plan").getByTestId("plan-part").first()).toHaveAttribute("data-state", "paid");
});

test("Delete my data: the account and its personal data go, the tickets stop working", async ({ page }) => {
  await signInFreshUser(page);
  await buyGalaxia(page);
  const [id] = await walletTickets(page);
  await page.goto("/account");
  await page.getByTestId("delete-confirm").check();
  await page.getByTestId("delete-submit").click();
  await expect(page).toHaveURL(/\/\?deleted=1$/);
  // Signed out for good: the wallet asks to sign in, and a new account does not see the ticket.
  await page.goto("/wallet");
  await expect(page).toHaveURL(/\/sign-in\?next=%2Fwallet/);
  await signInFreshUser(page);
  expectNotFoundResponse(page);
  const res = await page.goto(`/wallet/tickets/${id}`);
  expect([404]).toContain(res?.status());
});
