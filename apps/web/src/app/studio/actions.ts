"use server";
// Studio's server actions (design D1.1): each one checks the session and the active organiser,
// validates through the shared zod schemas and domain functions, and runs in one transaction
// scoped to that organiser. Owner-only rules live in the domain functions (D1.6).
import { refresh } from "next/cache";
import { redirect } from "next/navigation";
import { after } from "next/server";
import { slugTag } from "@/lib/storefront/shop-cache";
import { revalidateShop } from "@/lib/storefront/shop-data";
import { getAppPool } from "@/lib/db/pool";
import { withTx } from "@/lib/db/tx";
import { DomainError } from "@/lib/errors";
import {
  acceptInvitation,
  changeRole,
  completeOnboarding,
  createBrand,
  createOrganiser,
  deleteBrand,
  inviteMember,
  newOrganiserId,
  removeMember,
  revokeInvitation,
  setActiveOrganiser,
  setEventFeePolicy,
  setFeePolicy,
  updateBrand,
  getBrand,
} from "@/lib/organisers/domain";
import {
  addLineupSlot,
  deleteEvent,
  deleteLineupSlot,
  deletePool,
  deleteProduct,
  getEvent,
  saveEvent,
  savePool,
  saveProduct,
  saveVenue,
  setDays,
  setInstalmentPlan,
  setLadder,
  setProductPools,
  setRules,
} from "@/lib/catalogue/domain";
import { copyAsEdition } from "@/lib/catalogue/editions";
import { PRODUCT_TYPES, type ProductType } from "@/lib/catalogue/schema";
import { dateRange, localDate, toLocalInput, zonedToUtc } from "@/lib/catalogue/time";
import { parseMajor } from "@/lib/pricing/money";
import { currentSession } from "@/lib/auth/session";
import { getStudio, requireActive, studioTx, type ActiveStudio } from "@/lib/studio/context";
import { errorState, formObject, indexed, type FormState } from "@/lib/studio/form";
import { ladderFromForm } from "@/lib/studio/ladder-form";
import * as z from "zod";
import type pg from "pg";

// Shapes of the forms that go through formObject (every field is still validated by the domain
// schemas afterwards; these only fix the structure).
const text = z.string().max(4000).optional().default("");
const eventTextForm = z.object({
  name: z.object({ ro: text, en: text }).default({ ro: "", en: "" }),
  description: z.object({ ro: text, en: text }).default({ ro: "", en: "" }),
});
const dayRow = z.object({ day: z.string().max(10), opens: z.string().max(5), closes: z.string().max(5), remove: z.string().max(3).optional() });
const daysForm = z.object({
  days: z.record(z.string().regex(/^\d+$/), dayRow).optional(),
  add: z.object({ day: z.string().max(10).optional(), opens: z.string().max(5).optional(), closes: z.string().max(5).optional() }).optional(),
});

type Out = FormState | { redirect: string } | void;

async function act(fn: (s: ActiveStudio, c: pg.PoolClient) => Promise<Out>): Promise<FormState> {
  let out: Out;
  try {
    const s = await requireActive();
    out = await studioTx(s, (c) => fn(s, c));
  } catch (err) {
    return errorState(err);
  }
  if (out && "redirect" in out) redirect(out.redirect);
  refresh();
  return { ok: true, at: Date.now(), ...(out ?? {}) };
}

const str = (fd: FormData, k: string) => String(fd.get(k) ?? "").trim();
const checked = (fd: FormData, k: string) => fd.get(k) === "on" || fd.get(k) === "true";

/** Major-unit money field → minor units; refuses anything else with the field in error. */
function money(fd: FormData, k: string, field = k): number {
  const v = parseMajor(str(fd, k));
  if (v === null) throw new DomainError("price_invalid", field);
  return v;
}

/** "10" or "12,5" percent → basis points (1000, 1250). */
function percentBps(v: string): number {
  const minor = parseMajor(v);
  if (minor === null || minor >= 10_000) throw new DomainError("number_invalid", "depositBps");
  return minor;
}

function local(value: string, tz: string, field: string): Date | null {
  if (!value) return null;
  const d = zonedToUtc(value, tz);
  if (!d) throw new DomainError("date_invalid", field);
  return d;
}

// ---------------------------------------------------------------- onboarding

export async function createOrganiserAction(_: FormState, fd: FormData): Promise<FormState> {
  const session = await currentSession();
  if (!session) return { errors: { _: "signed_out" } };
  const studio = await getStudio();
  if (studio?.organisers.some((o) => o.onboarding_state === "brand" && o.role === "owner")) redirect("/studio/onboarding");
  const id = newOrganiserId();
  try {
    await withTx(getAppPool(), { organiserId: id, userId: session.user.id }, async (c) => {
      await createOrganiser(c, { id, userId: session.user.id, input: { name: str(fd, "name"), slug: str(fd, "slug") } });
      await setActiveOrganiser(c, { sessionId: session.session.id, userId: session.user.id, organiserId: id });
    });
  } catch (err) {
    return errorState(err);
  }
  redirect("/studio/onboarding");
}

export async function completeOnboardingAction(_: FormState, fd: FormData): Promise<FormState> {
  const studio = await getStudio();
  if (!studio) return { errors: { _: "signed_out" } };
  const org = studio.organisers.find((o) => o.onboarding_state === "brand" && o.role === "owner");
  if (!org) redirect("/studio");
  let brandId: string;
  try {
    brandId = await withTx(getAppPool(), { organiserId: org.id, userId: studio.user.id }, async (c) => {
      const r = await completeOnboarding(c, org.id, {
        name: str(fd, "name"),
        slug: str(fd, "slug"),
        languages: fd.getAll("languages").map(String) as ("ro" | "en")[],
        currency: str(fd, "currency") as "RON",
        template: (str(fd, "template") || "festival") as "festival",
      });
      await setActiveOrganiser(c, { sessionId: studio.sessionId, userId: studio.user.id, organiserId: org.id });
      return r.brandId;
    });
  } catch (err) {
    return errorState(err);
  }
  redirect(`/studio/brands/${brandId}?welcome=1`);
}

// ---------------------------------------------------------------- active organiser

export async function switchOrganiserAction(fd: FormData): Promise<void> {
  const studio = await getStudio();
  if (!studio) redirect("/sign-in?next=%2Fstudio");
  const organiserId = str(fd, "organiserId");
  await withTx(getAppPool(), { userId: studio.user.id }, (c) =>
    setActiveOrganiser(c, { sessionId: studio.sessionId, userId: studio.user.id, organiserId }),
  ).catch(() => undefined);
  redirect("/studio");
}

// ---------------------------------------------------------------- brands

function brandFields(fd: FormData) {
  return {
    name: str(fd, "name"),
    slug: str(fd, "slug"),
    languages: fd.getAll("languages").map(String) as ("ro" | "en")[],
    currency: str(fd, "currency") as "RON",
    template: (str(fd, "template") || "festival") as "festival",
    logoUrl: str(fd, "logoUrl"),
  };
}

export async function saveBrandAction(brandId: string | null, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => {
    if (brandId) {
      const before = await getBrand(c, brandId);
      const b = await updateBrand(c, s.active.id, brandId, brandFields(fd));
      // A changed address must stop serving the cached shop at the old one (after commit).
      if (before && before.slug !== b.slug) after(() => revalidateShop([slugTag(before.slug), slugTag(b.slug)]));
      return;
    }
    const b = await createBrand(c, s.active.id, brandFields(fd));
    return { redirect: `/studio/brands/${b.id}` };
  });
}

export async function deleteBrandAction(brandId: string): Promise<FormState> {
  return act(async (s, c) => {
    await deleteBrand(c, s.active.role, brandId);
    return { redirect: "/studio/brands" };
  });
}

// ---------------------------------------------------------------- team

/** "organiser", "brand:<id>" or "event:<id>" → scopeType and scopeId. */
function splitScope(v: string): { scopeType: string; scopeId?: string } {
  const [scopeType, scopeId] = v.split(":");
  return { scopeType: scopeType || "organiser", scopeId };
}

export async function inviteAction(prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => {
    const inv = await inviteMember(c, {
      role: s.active.role,
      userId: s.user.id,
      input: { email: str(fd, "email"), role: str(fd, "role"), ...splitScope(str(fd, "scopeCombined")) },
    });
    return { message: "invited", data: { path: `/studio/invite/${s.active.id}/${inv.token}` } };
  });
}

export async function revokeInvitationAction(id: string): Promise<FormState> {
  return act(async (s, c) => revokeInvitation(c, s.active.role, id));
}

export async function changeRoleAction(targetUserId: string, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => {
    await changeRole(c, { role: s.active.role, targetUserId, input: { role: str(fd, "role"), ...splitScope(str(fd, "scope")) } });
  });
}

export async function removeMemberAction(targetUserId: string): Promise<FormState> {
  return act(async (s, c) => removeMember(c, { role: s.active.role, targetUserId }));
}

export async function acceptInvitationAction(organiserId: string, token: string): Promise<FormState> {
  const studio = await getStudio();
  if (!studio) return { errors: { _: "signed_out" } };
  try {
    await withTx(getAppPool(), { organiserId, userId: studio.user.id }, async (c) => {
      await acceptInvitation(c, { organiserId, token, userId: studio.user.id, email: studio.user.email });
      await setActiveOrganiser(c, { sessionId: studio.sessionId, userId: studio.user.id, organiserId });
    });
  } catch (err) {
    return errorState(err);
  }
  redirect("/studio");
}

// ---------------------------------------------------------------- fees

export async function setFeePolicyAction(prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => setFeePolicy(c, s.active.role, str(fd, "policy")));
}

export async function setEventFeePolicyAction(eventId: string, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => setEventFeePolicy(c, s.active.role, eventId, str(fd, "policy") || null));
}

// ---------------------------------------------------------------- venues

export async function saveVenueAction(venueId: string | null, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => {
    await saveVenue(
      c,
      {
        name: str(fd, "name"),
        address: str(fd, "address"),
        lat: str(fd, "lat"),
        lng: str(fd, "lng"),
        stages: str(fd, "stages").split(",").map((x) => x.trim()).filter(Boolean),
      },
      venueId ?? undefined,
    );
  });
}

// ---------------------------------------------------------------- events

export async function saveEventAction(eventId: string | null, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => {
    const tz = str(fd, "timezone") || "Europe/Bucharest";
    const o = eventTextForm.parse(formObject(fd));
    const ev = await saveEvent(
      c,
      {
        brandId: str(fd, "brandId"),
        venueId: str(fd, "venueId"),
        slug: str(fd, "slug"),
        name: o.name,
        description: o.description,
        timezone: tz,
        startsAt: local(str(fd, "startsAt"), tz, "startsAt") ?? ("" as unknown as Date),
        endsAt: local(str(fd, "endsAt"), tz, "endsAt") ?? ("" as unknown as Date),
      },
      eventId ?? undefined,
    );
    if (!eventId) {
      // A new event gets one day per date with the hours given (editable afterwards).
      const opens = str(fd, "opens") || "18:00";
      const closes = str(fd, "closes") || "23:00";
      const first = localDate(ev.starts_at, ev.timezone);
      const lastDate = localDate(ev.ends_at, ev.timezone);
      // An end in the small hours before the opening time is the last night's close, not a new day.
      const endTime = toLocalInput(ev.ends_at, ev.timezone).slice(11);
      const keep = dateRange(first, lastDate).filter((d) => d === first || d !== lastDate || endTime >= opens);
      await setDays(c, ev.id, keep.map((day) => ({ day, opens, closes })));
      return { redirect: `/studio/events/${ev.id}` };
    }
  });
}

export async function setDaysAction(eventId: string, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => {
    const o = daysForm.parse(formObject(fd));
    const days = indexed<z.infer<typeof dayRow>>(o.days).filter((d) => d.remove !== "on").map(({ day, opens, closes }) => ({ day, opens, closes }));
    if (o.add?.day) days.push({ day: o.add.day, opens: o.add.opens || "18:00", closes: o.add.closes || "23:00" });
    await setDays(c, eventId, days.sort((a, b) => a.day.localeCompare(b.day)));
  });
}

export async function addLineupAction(eventId: string, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => {
    await addLineupSlot(c, eventId, { dayId: str(fd, "dayId"), artist: str(fd, "artist"), stage: str(fd, "stage"), starts: str(fd, "starts") });
  });
}

export async function deleteLineupAction(eventId: string, slotId: string): Promise<FormState> {
  return act(async (s, c) => deleteLineupSlot(c, eventId, slotId));
}

export async function setRulesAction(eventId: string, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => setRules(c, eventId, { minAge: str(fd, "minAge"), consentUnderAge: str(fd, "consentUnderAge"), maxPerAccount: str(fd, "maxPerAccount") }));
}

export async function copyEditionAction(eventId: string, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => {
    const raw = str(fd, "offsetDays");
    const n = raw ? Number(raw) : undefined;
    if (raw && !Number.isInteger(n)) throw new DomainError("number_invalid", "offsetDays");
    const copy = await copyAsEdition(c, eventId, { offsetDays: n });
    return { redirect: `/studio/events/${copy.id}?copied=1` };
  });
}

export async function deleteEventAction(eventId: string): Promise<FormState> {
  return act(async (s, c) => {
    await deleteEvent(c, eventId);
    return { redirect: "/studio/events" };
  });
}

// ---------------------------------------------------------------- products

function lines(v: string): string[] {
  return v.split("\n").map((x) => x.trim()).filter(Boolean);
}

function productConfig(type: ProductType, fd: FormData) {
  switch (type) {
    case "pass":
      return { zone: str(fd, "config.zone"), days: fd.getAll("config.days").map(String) };
    case "day_ticket":
      return { day: str(fd, "config.day") || null };
    case "flex_day":
      return { days: fd.getAll("config.days").map(String) };
    case "add_on": {
      const mode = str(fd, "config.requiresMode") || "any_ticket";
      return { requires: mode === "products" ? fd.getAll("config.requires").map(String) : mode };
    }
    case "voucher":
      return { spendValueMinor: str(fd, "config.spendValue") ? money(fd, "config.spendValue", "config.spendValueMinor") : undefined };
    case "table":
      return { category: str(fd, "config.category"), seats: str(fd, "config.seats") };
    case "time_slot":
      return {
        slotMinutes: str(fd, "config.slotMinutes"),
        firstStart: str(fd, "config.firstStart"),
        lastStart: str(fd, "config.lastStart"),
        maxConsecutiveSlots: str(fd, "config.maxConsecutiveSlots"),
      };
  }
}

export async function saveProductAction(eventId: string, productId: string | null, type: string, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => {
    if (!(PRODUCT_TYPES as readonly string[]).includes(type)) throw new DomainError("invalid", "type");
    const ev = await getEvent(c, eventId);
    if (!ev) throw new DomainError("not_found");
    const t = type as ProductType;
    const perksRo = lines(str(fd, "perks.ro"));
    const perksEn = lines(str(fd, "perks.en"));
    const perks = Array.from({ length: Math.max(perksRo.length, perksEn.length) }, (_, i) => ({ ro: perksRo[i] ?? "", en: perksEn[i] ?? "" }));
    const p = await saveProduct(
      c,
      eventId,
      {
        type: t,
        name: { ro: str(fd, "name.ro"), en: str(fd, "name.en") },
        perks,
        config: productConfig(t, fd),
        named: checked(fd, "named"),
        visible: checked(fd, "visible"),
        salesFrom: local(str(fd, "salesFrom"), ev.timezone, "salesFrom"),
        salesUntil: local(str(fd, "salesUntil"), ev.timezone, "salesUntil"),
        maxPerOrder: str(fd, "maxPerOrder") || "10",
        sort: str(fd, "sort") || "0",
        personaliseDeadline: local(str(fd, "personaliseDeadline"), ev.timezone, "personaliseDeadline"),
        checkinDeadline: local(str(fd, "checkinDeadline"), ev.timezone, "checkinDeadline"),
        lateCheckinFeeMinor: str(fd, "lateCheckinFee") ? money(fd, "lateCheckinFee", "lateCheckinFeeMinor") : null,
      } as never,
      productId ?? undefined,
    );
    if (!productId) {
      // A new product starts with its first price (required) and optionally a pool.
      await setLadder(c, p.id, { mode: "by_date", steps: [{ priceMinor: money(fd, "price") }] });
      const pool = str(fd, "poolId");
      if (pool) await setProductPools(c, p.id, [pool]);
      return { redirect: `/studio/events/${eventId}/products/${p.id}` };
    }
  });
}

export async function deleteProductAction(eventId: string, productId: string): Promise<FormState> {
  return act(async (s, c) => {
    await deleteProduct(c, eventId, productId);
    return { redirect: `/studio/events/${eventId}/products` };
  });
}

export async function setProductPoolsAction(productId: string, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => setProductPools(c, productId, fd.getAll("pools").map(String)));
}

export async function setLadderAction(productId: string, tz: string, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => setLadder(c, productId, ladderFromForm(fd, tz)));
}

export async function setPlanAction(productId: string, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => {
    if (checked(fd, "remove")) return setInstalmentPlan(c, productId, null);
    const percent = str(fd, "depositKind") === "percent";
    await setInstalmentPlan(c, productId, {
      depositMinor: percent ? null : money(fd, "deposit", "depositMinor"),
      depositBps: percent ? percentBps(str(fd, "depositPercent")) : null,
      monthlyCount: str(fd, "monthlyCount"),
      dayOfMonth: str(fd, "dayOfMonth") || "1",
    });
  });
}

// ---------------------------------------------------------------- pools

export async function savePoolAction(eventId: string, poolId: string | null, prev: FormState, fd: FormData): Promise<FormState> {
  return act(async (s, c) => {
    await savePool(c, eventId, { name: str(fd, "name"), capacity: str(fd, "capacity"), scope: (str(fd, "scope") || "event") as "event" }, poolId ?? undefined);
  });
}

export async function deletePoolAction(eventId: string, poolId: string): Promise<FormState> {
  return act(async (s, c) => deletePool(c, eventId, poolId));
}
