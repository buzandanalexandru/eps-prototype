// On-sale helpers (step 5): a fresh organiser's published event and a sale moment on it, set from
// Studio's sale settings screen.
import { expect, type Browser, type Page, devices } from "@playwright/test";
import { onboard, setLocale, signInFreshUser, uniqueSlug } from "./fixtures";
import { openBuilder } from "./builder-helpers";
import { createSellableEvent } from "./shop-helpers";

export type SaleOpts = {
  name?: string;
  /** Minutes from now (negative: already open). */
  opensIn: number;
  prequeueMinutes?: number;
  queueOn?: boolean;
  rate?: number;
  paused?: boolean;
  capPerOrder?: number;
  capPerPerson?: number;
  botDefence?: boolean;
  otpOn?: boolean;
  otpThreshold?: number;
};

/** "YYYY-MM-DDTHH:MM" in Europe/Bucharest (the events' default zone) `minutes` from now. */
export function bucharestInput(minutes: number): string {
  const d = new Date(Date.now() + minutes * 60_000);
  const p = Object.fromEntries(
    new Intl.DateTimeFormat("en-GB", { timeZone: "Europe/Bucharest", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit", hourCycle: "h23" })
      .formatToParts(d)
      .map((x) => [x.type, x.value]),
  );
  return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}`;
}

/** Signs in a fresh organiser, creates and publishes an event; returns its addresses. */
export async function publishedEvent(page: Page) {
  await setLocale(page, "en");
  await signInFreshUser(page);
  const { brandSlug } = await onboard(page, { brand: "On-sale Test" });
  const eventSlug = uniqueSlug("drop");
  const { eventUrl } = await createSellableEvent(page, eventSlug);
  await openBuilder(page);
  await page.getByTestId("tab-publish").click();
  await page.getByTestId("publish").click();
  await expect(page.getByTestId("publish-message")).toHaveText("Version 1 is live.");
  return { brandSlug, eventSlug, eventUrl };
}

/** Fills a sale form (new or existing) with the options given. */
export async function fillSale(form: ReturnType<Page["getByTestId"]>, o: Partial<SaleOpts>) {
  if (o.name !== undefined) await form.getByTestId("sale-new-name").fill(o.name);
  if (o.opensIn !== undefined) await form.getByTestId("sale-opens-at").fill(bucharestInput(o.opensIn));
  if (o.prequeueMinutes !== undefined) await form.getByTestId("sale-prequeue").fill(String(o.prequeueMinutes));
  if (o.queueOn !== undefined) await form.getByTestId("sale-queue-on").setChecked(o.queueOn);
  if (o.rate !== undefined) await form.getByTestId("sale-rate").fill(String(o.rate));
  if (o.paused !== undefined) await form.getByTestId("sale-paused").setChecked(o.paused);
  if (o.capPerOrder !== undefined) await form.getByTestId("sale-cap-order").fill(String(o.capPerOrder));
  if (o.capPerPerson !== undefined) await form.getByTestId("sale-cap-person").fill(String(o.capPerPerson));
  if (o.botDefence !== undefined) await form.getByTestId("sale-bot-defence").setChecked(o.botDefence);
  if (o.otpOn !== undefined) await form.getByTestId("sale-otp-on").setChecked(o.otpOn);
  if (o.otpThreshold !== undefined) await form.getByTestId("sale-otp-threshold").fill(String(o.otpThreshold));
}

/** Adds a sale moment from Studio; returns its id. */
export async function addSale(page: Page, eventUrl: string, o: SaleOpts): Promise<string> {
  await page.goto(`${eventUrl}/sales`);
  const form = page.getByTestId("new-sale-form");
  await fillSale(form, { name: o.name ?? "General sale", ...o });
  await form.getByTestId("sale-create").click();
  await expect(page.getByTestId("sale-saved")).toHaveText("Saved as version 1.");
  return (await page.getByTestId("sale").last().getAttribute("data-sale-id"))!;
}

const errors = new WeakMap<Page, string[]>();

/** A fan in a fresh browser (desktop or phone) in a language; its console errors are collected. */
export async function fan(browser: Browser, opts: { phone?: boolean; locale?: "ro" | "en" } = {}) {
  const ctx = await browser.newContext(opts.phone ? { ...devices["Pixel 7"] } : {});
  const p = await ctx.newPage();
  const list: string[] = [];
  errors.set(p, list);
  p.on("console", (m) => {
    if (m.type() === "error") list.push(m.text());
  });
  p.on("pageerror", (e) => list.push(e.message));
  await setLocale(p, opts.locale ?? "en");
  return p;
}

/** Fails on any console error the fan's page logged. */
export function expectNoConsoleErrors(p: Page) {
  expect(errors.get(p) ?? [], "console errors").toEqual([]);
}

/** On the event page: picks `qty` General Access tickets and presses Continue. */
export async function pickAndContinue(page: Page, brandSlug: string, eventSlug: string, qty: number, locale: "ro" | "en" = "en") {
  await page.goto(`/s/${brandSlug}/${locale}/e/${eventSlug}`);
  await expect(page.getByTestId("shop-tickets")).toHaveAttribute("data-availability", "ok");
  const card = page.locator("[data-testid=ticket-card]").first();
  for (let i = 0; i < qty; i++) await card.getByTestId("plus").click();
  await page.getByTestId("continue").click();
}
