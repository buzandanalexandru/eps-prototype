// Organisers spec "Team roles screen": owner invites box office for one event; the link adds the member.
import { DEMO_DOMAIN, desktopOnly, expect, expectNoHorizontalScroll, onboard, setLocale, signInFreshUser, signInWithMailbox, test } from "./fixtures";

test("owner invites a box office member for one event; opening the link with that address adds them", async ({ page, browser }, info) => {
  test.skip(desktopOnly(info.project.name), "creates data");
  await setLocale(page, "en");
  await signInFreshUser(page);
  await onboard(page, { org: "Team Org" });
  // An event to scope the role to.
  await page.goto("/studio/events/new");
  const ev = page.getByTestId("new-event-form");
  await ev.getByLabel("Name (English)").fill("Door Night");
  await ev.getByLabel("Name (Romanian)").fill("Seara de la intrare");
  await ev.getByLabel("Event address").fill("door-night");
  await ev.getByLabel("Starts").fill("2027-07-01T20:00");
  await ev.getByLabel("Ends").fill("2027-07-01T23:00");
  await ev.getByRole("button", { name: "Create event" }).click();
  await expect(page).toHaveURL(/\/studio\/events\/[0-9a-f-]{36}$/);

  const door = `door-${Date.now()}@${DEMO_DOMAIN}`;
  await page.goto("/studio/team");
  const form = page.getByTestId("invite-form");
  await form.getByLabel("Email").fill(door);
  await form.getByLabel("Role").selectOption("box_office");
  await form.getByLabel("Scope").selectOption({ label: "Event: Door Night" });
  await form.getByRole("button", { name: "Create invitation" }).click();
  const url = await page.getByTestId("invite-url").innerText();
  expect(url).toMatch(/\/studio\/invite\/[0-9a-f-]{36}\/[A-Za-z0-9_-]+$/);
  await expect(page.getByTestId("scope-note")).toContainText("not enforced in the prototype");
  await page.reload();
  await expect(page.getByTestId("invitation")).toContainText(`${door} · Box office · Event: Door Night · pending`);
  await expectNoHorizontalScroll(page);

  // The invited person signs in with that address and opens the link.
  const ctx = await browser.newContext();
  const invitee = await ctx.newPage();
  await setLocale(invitee, "en");
  const path = new URL(url).pathname;
  await signInWithMailbox(invitee, door, path);
  await expect(invitee).toHaveURL(new RegExp(`${path}$`));
  await invitee.getByRole("button", { name: "Accept invitation" }).click();
  await expect(invitee).toHaveURL(/\/studio$/);
  await expect(invitee.getByTestId("active-organiser")).toHaveText("Team Org");
  // A non-owner sees the team but cannot change it.
  await invitee.goto("/studio/team");
  await expect(invitee.getByTestId("owner-only-note")).toHaveText("Only the owner can change the team.");
  await ctx.close();

  await page.reload();
  await expect(page.getByTestId("member")).toHaveCount(2);
  await expect(page.getByTestId("members")).toContainText("Box office · Event: Door Night");
});
