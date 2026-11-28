// Catalogue spec: each product type can be created in Studio; the price ladder and instalment
// panels; pools; the readiness checklist.
import { desktopOnly, expect, expectNoHorizontalScroll, onboard, setLocale, signInFreshUser, test } from "./fixtures";

test("build a small festival: venue, event, pool, every product type, ladder, instalments, ready", async ({ page }, info) => {
  test.skip(desktopOnly(info.project.name), "creates data; the phone layout is checked in sample-data.spec.ts");
  test.setTimeout(180_000);
  await setLocale(page, "en");
  await signInFreshUser(page);
  await onboard(page, { org: "Catalogue Org" });

  await page.goto("/studio/venues");
  const v = page.getByTestId("new-venue-form");
  await v.getByLabel("Name", { exact: true }).fill("Seaside Arena");
  await v.getByLabel("Stages").fill("Main, Beach");
  await v.getByRole("button", { name: "Create venue" }).click();
  await expect(page.getByTestId("venue-list")).toContainText("Seaside Arena");

  await page.goto("/studio/events/new");
  const ev = page.getByTestId("new-event-form");
  await ev.getByLabel("Venue").selectOption({ label: "Seaside Arena" });
  await ev.getByLabel("Name (Romanian)").fill("Galaxia de test 2027");
  await ev.getByLabel("Name (English)").fill("Test Galaxy 2027");
  await ev.getByLabel("Description (Romanian)").fill("Trei nopți la mare.");
  await ev.getByLabel("Description (English)").fill("Three nights by the sea.");
  await ev.getByLabel("Event address").fill("test-galaxy-2027");
  await ev.getByLabel("Starts").fill("2027-07-15T17:00");
  await ev.getByLabel("Ends").fill("2027-07-18T03:00");
  await ev.getByLabel("Opens").fill("17:00");
  await ev.getByLabel("Closes").fill("03:00");
  await ev.getByRole("button", { name: "Create event" }).click();
  await expect(page).toHaveURL(/\/studio\/events\/[0-9a-f-]{36}$/);
  const eventUrl = new URL(page.url()).pathname;
  await expect(page.getByTestId("day")).toHaveCount(3);
  await expect(page.getByTestId("days")).toContainText("closes the next morning");
  // Not ready yet: no products.
  await expect(page.locator('[data-testid=problems] [data-code="no_products"]')).toBeVisible();

  // Line-up on a day and stage.
  const lu = page.getByTestId("lineup-form");
  await lu.getByLabel("Artist").fill("Late Act");
  await lu.getByLabel("Stage").fill("Main");
  await lu.getByLabel("Starts at").fill("01:00");
  await lu.getByRole("button", { name: "Add artist" }).click();
  await expect(page.getByTestId("lineup")).toContainText("01:00 Late Act");

  // Buying rules: inconsistent values are refused, valid ones are saved.
  const rules = page.getByTestId("rules-form");
  await rules.locator("input[name=minAge]").fill("18");
  await rules.locator("input[name=consentUnderAge]").fill("16");
  await rules.getByRole("button", { name: "Save rules" }).click();
  await expect(rules.getByTestId("error-consentUnderAge")).toBeVisible();
  await rules.locator("input[name=minAge]").fill("16");
  await rules.locator("input[name=consentUnderAge]").fill("18");
  await rules.getByRole("button", { name: "Save rules" }).click();
  await expect(page.getByTestId("rules-summary")).toContainText("Minimum age 16");
  // Catalogue spec "Stand-up age limit": the saved values stay in the panel and its summary.
  await page.reload();
  await expect(page.locator("[data-testid=rules-form] input[name=minAge]")).toHaveValue("16");
  await expect(page.getByTestId("rules-summary")).toHaveText("Minimum age 16 · parental consent under 18");

  await page.goto(`${eventUrl}/pools`);
  const pool = page.getByTestId("new-pool-form");
  await pool.getByLabel("Name", { exact: true }).fill("Arena");
  await pool.getByLabel("Capacity").fill("40000");
  await pool.getByRole("button", { name: "Create pool" }).click();
  await expect(page.getByTestId("pool-name")).toHaveText("Arena");
  await pool.getByLabel("Name", { exact: true }).fill("Courts");
  await pool.getByLabel("Capacity").fill("4");
  await pool.getByLabel("Counts").selectOption("slot");
  await pool.getByRole("button", { name: "Create pool" }).click();
  await expect(page.getByTestId("pool")).toHaveCount(2);

  const types: [string, string, (f: import("@playwright/test").Locator) => Promise<void>][] = [
    ["pass", "Golden Circle", async (f) => { await f.getByLabel("Zone").fill("Golden Circle"); await f.getByLabel("Perks (English)").fill("Fast lane\nVIP toilets"); await f.getByLabel("Perks (Romanian)").fill("Acces rapid\nToalete VIP"); }],
    ["day_ticket", "Friday", async () => {}],
    ["flex_day", "Any Day", async () => {}],
    ["add_on", "Camping", async () => {}],
    ["voucher", "Drinks voucher", async (f) => { await f.getByLabel("Spend value").fill("25"); }],
    ["table", "Table A", async (f) => { await f.getByLabel("Category").fill("A"); await f.getByLabel("Seats").fill("4"); }],
    ["time_slot", "Padel", async (f) => { await f.getByLabel("Slot length (minutes)").fill("30"); await f.getByLabel("First start").fill("17:00"); await f.getByLabel("Last start").fill("02:30"); }],
  ];
  for (const [type, name, fill] of types) {
    await page.goto(`${eventUrl}/products`);
    await page.getByLabel("Product type").selectOption(type);
    await page.getByRole("button", { name: "Add product" }).click();
    const f = page.getByTestId("product-form");
    await expect(page.getByTestId("type-explanation")).toBeVisible();
    await f.getByLabel("Name (Romanian)").fill(name);
    await f.getByLabel("Name (English)").fill(name);
    await fill(f);
    await f.getByLabel(/^Price/).fill(type === "voucher" ? "25" : "539");
    await f.getByLabel("Capacity pool").selectOption({ label: type === "time_slot" ? "Courts" : "Arena" });
    await f.getByRole("button", { name: "Create product" }).click();
    await expect(page).toHaveURL(/\/products\/[0-9a-f-]{36}$/);
    await expect(page.getByTestId("product-title")).toHaveText(name);
  }
  // An invalid slot length is refused with the setting in error.
  await page.goto(`${eventUrl}/products`);
  await page.getByLabel("Product type").selectOption("time_slot");
  await page.getByRole("button", { name: "Add product" }).click();
  const bad = page.getByTestId("product-form");
  await bad.getByLabel("Slot length (minutes)").fill("45");
  await bad.getByLabel("First start").fill("17:00");
  await bad.getByLabel("Last start").fill("02:30");
  await bad.getByLabel(/^Price/).fill("50");
  await bad.getByRole("button", { name: "Create product" }).click();
  await expect(bad.getByTestId("error-config.slotMinutes")).toBeVisible();

  await page.goto(`${eventUrl}/products`);
  await expect(page.getByTestId("product")).toHaveCount(7);
  const gc = page.getByTestId("product").filter({ hasText: "Golden Circle" });
  await expect(gc.getByTestId("fan-pays")).toHaveText("Fan pays RON 552.48");
  await expect(gc.getByTestId("you-receive")).toHaveText("You receive RON 539.00");

  // Price ladder by date with a current and next step, and the fixed public note.
  await gc.getByRole("link", { name: "Golden Circle" }).click();
  const ladder = page.getByTestId("ladder-form");
  const steps = ladder.getByTestId("ladder-step");
  await steps.nth(0).getByLabel(/^Price/).fill("539");
  await steps.nth(1).getByLabel(/^From/).fill("2099-01-01T00:00");
  await steps.nth(1).getByLabel(/^Price/).fill("599");
  await ladder.getByRole("button", { name: "Save ladder" }).click();
  await expect(page.getByTestId("next-step")).toHaveText("Next: RON 599.00 from 2099-01-01 00:00");
  await expect(page.getByTestId("price-in-force")).toHaveText("RON 539.00");
  await expect(page.getByTestId("public-note")).toContainText("never on who you are");
  // An unordered ladder is refused.
  await steps.nth(2).getByLabel(/^From/).fill("2098-01-01T00:00");
  await steps.nth(2).getByLabel(/^Price/).fill("650");
  await ladder.getByRole("button", { name: "Save ladder" }).click();
  await expect(ladder.getByTestId("error-steps")).toBeVisible();

  // Instalments: 10 now + 3 monthly payments, schedule shown.
  await page.reload();
  const plan = page.getByTestId("plan-form");
  await plan.locator("input[name=deposit]").fill("10");
  await plan.locator("input[name=monthlyCount]").fill("3");
  await plan.getByRole("button", { name: "Save plan" }).click();
  await expect(page.getByTestId("deposit")).toHaveText("RON 10.00 now");
  await expect(page.getByTestId("payment")).toHaveCount(3);
  await expect(page.getByTestId("breakpoints")).toContainText("get 3 monthly payments");
  // A plan running past the event is refused with the latest possible count.
  await plan.locator("input[name=monthlyCount]").fill("24");
  await plan.getByRole("button", { name: "Save plan" }).click();
  await expect(plan.getByTestId("error-monthlyCount")).toContainText("at most");

  // Every product has a pool, names and a price: the event is ready.
  await page.goto(eventUrl);
  await expect(page.getByTestId("readiness")).toHaveAttribute("data-ready", "true");
  await expectNoHorizontalScroll(page);

  // Copy as a new edition.
  await page.getByTestId("copy-form").getByRole("button", { name: "Copy as new edition" }).click();
  await expect(page).toHaveURL(/\?copied=1$/);
  await expect(page.getByTestId("event-title")).toHaveText("Test Galaxy 2028");
  await page.goto(`${new URL(page.url()).pathname}/products`);
  await expect(page.getByTestId("product")).toHaveCount(7);
});
