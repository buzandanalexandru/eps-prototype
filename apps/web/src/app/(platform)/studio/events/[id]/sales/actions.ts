"use server";
// Studio's sale settings (step 5, E1): every save is a new version in the core's sales schema, so
// the gateway's admission dial follows within 2 seconds.
import { redirect } from "next/navigation";
import { organiserCore } from "@/lib/checkout/studio-core";
import { zonedToUtc } from "@/lib/catalogue/time";
import { requireActive } from "@/lib/studio/context";

const int = (fd: FormData, k: string) => {
  const n = Number(String(fd.get(k) ?? "").trim());
  return Number.isFinite(n) ? Math.round(n) : 0;
};

function validZone(tz: string): string {
  try {
    new Intl.DateTimeFormat("en", { timeZone: tz });
    return tz;
  } catch {
    return "Europe/Bucharest";
  }
}

function settingsFrom(fd: FormData) {
  const tz = validZone(String(fd.get("tz") ?? ""));
  const at = zonedToUtc(String(fd.get("opensAt") ?? ""), tz);
  return {
    opensAt: at ? at.toISOString() : "",
    prequeueMinutes: int(fd, "prequeueMinutes"),
    queueOn: fd.get("queueOn") === "on",
    ratePerMinute: int(fd, "ratePerMinute"),
    activeLimit: int(fd, "activeLimit"),
    paused: fd.get("paused") === "on",
    holdMinutes: int(fd, "holdMinutes"),
    capPerOrder: int(fd, "capPerOrder"),
    capPerPerson: int(fd, "capPerPerson"),
    botDefence: fd.get("botDefence") === "on",
    otpOn: fd.get("otpOn") === "on",
    otpThreshold: int(fd, "otpThreshold"),
    accountsOnly: fd.get("accountsOnly") === "on",
  };
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

type Saved = { id: string; version: number };
type Fail = { ok: false; status: number; code: string; data?: Record<string, unknown> };

function errorQuery(r: Fail): string {
  const q = new URLSearchParams({ error: r.code });
  for (const k of ["field", "min", "max"]) {
    const v = r.data?.[k];
    if ((typeof v === "string" && /^[A-Za-z]{1,40}$/.test(v)) || typeof v === "number") q.set(k, String(v));
  }
  return q.toString();
}

function back(eventId: string, r: { ok: true; data: Saved } | Fail, saleId?: string): never {
  if (r.ok) redirect(`/studio/events/${eventId}/sales?saved=${r.data.id}&v=${r.data.version}#sale-${r.data.id}`);
  redirect(`/studio/events/${eventId}/sales?${errorQuery(r)}${saleId ? `&sale=${saleId}#sale-${saleId}` : ""}`);
}

export async function createSaleAction(eventId: string, fd: FormData) {
  await requireActive();
  if (!UUID.test(eventId)) redirect("/studio/events");
  const name = String(fd.get("name") ?? "").trim().slice(0, 80);
  back(eventId, await organiserCore<Saved>("POST", `/v1/organiser/events/${eventId}/sales`, { name, settings: settingsFrom(fd) }));
}

export async function saveSaleAction(eventId: string, saleId: string, fd: FormData) {
  await requireActive();
  if (!UUID.test(eventId) || !UUID.test(saleId)) redirect("/studio/events");
  // The version the form was made on: a save on top of someone else's newer save is refused.
  const base = int(fd, "baseVersion");
  back(eventId, await organiserCore<Saved>("PUT", `/v1/organiser/sales/${saleId}/settings${base > 0 ? `?baseVersion=${base}` : ""}`, settingsFrom(fd)), saleId);
}

/** Pause or resume at once, on top of whatever version is current (review D-l). */
export async function pauseSaleAction(eventId: string, saleId: string, paused: boolean) {
  await requireActive();
  if (!UUID.test(eventId) || !UUID.test(saleId)) redirect("/studio/events");
  back(eventId, await organiserCore<Saved>("PATCH", `/v1/organiser/sales/${saleId}/live`, { paused }), saleId);
}

/** Delete a sale moment before its pre-queue opens (review D-k). */
export async function deleteSaleAction(eventId: string, saleId: string) {
  await requireActive();
  if (!UUID.test(eventId) || !UUID.test(saleId)) redirect("/studio/events");
  const r = await organiserCore<unknown>("DELETE", `/v1/organiser/sales/${saleId}`);
  if (r.ok) redirect(`/studio/events/${eventId}/sales?deleted=1`);
  back(eventId, r as Fail, saleId);
}
