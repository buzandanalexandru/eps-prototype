"use server";
// Studio's rehearsal, simulator and live controls (step 6): every call goes to the core with the
// signed-in organiser's own core token; the core checks the role and the organiser.
import { redirect } from "next/navigation";
import { organiserCore, type CoreAnswer } from "@/lib/checkout/studio-core";
import { requireActive } from "@/lib/studio/context";
import type { components } from "@/lib/core/schema";

export type Run = components["schemas"]["SimulationRun"];
export type Knobs = components["schemas"]["SimulationKnobs"];
export type SaleView = components["schemas"]["SaleView"];

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

function bad<T>(): CoreAnswer<T> {
  return { ok: false, status: 400, code: "invalid_request" };
}

export async function createRehearsalAction(saleId: string) {
  await requireActive();
  if (!UUID.test(saleId)) redirect("/studio/events");
  const r = await organiserCore<SaleView>("POST", `/v1/organiser/sales/${saleId}/rehearsals`);
  redirect(`/studio/sales/${saleId}/simulator${r.ok ? "" : `?error=${encodeURIComponent(r.code)}`}`);
}

export async function resetRehearsalAction(saleId: string, rehearsalId: string) {
  await requireActive();
  if (!UUID.test(saleId) || !UUID.test(rehearsalId)) redirect("/studio/events");
  const r = await organiserCore<SaleView>("POST", `/v1/organiser/sales/${rehearsalId}/rehearsal/reset`);
  redirect(`/studio/sales/${saleId}/simulator${r.ok ? "" : `?error=${encodeURIComponent(r.code)}`}`);
}

export async function startRunAction(rehearsalId: string, knobs: Knobs): Promise<CoreAnswer<Run>> {
  await requireActive();
  if (!UUID.test(rehearsalId)) return bad();
  return organiserCore<Run>("POST", `/v1/organiser/sales/${rehearsalId}/simulations`, knobs);
}

export async function runAction(runId: string): Promise<CoreAnswer<Run>> {
  await requireActive();
  if (!UUID.test(runId)) return bad();
  return organiserCore<Run>("GET", `/v1/organiser/simulations/${runId}`);
}

export async function stopRunAction(runId: string): Promise<CoreAnswer<Run>> {
  await requireActive();
  if (!UUID.test(runId)) return bad();
  return organiserCore<Run>("POST", `/v1/organiser/simulations/${runId}/stop`);
}

/** The dashboard's dial and pause (step 5's live patch, on top of the current version). */
export async function liveChangeAction(saleId: string, change: { ratePerMinute?: number; paused?: boolean }): Promise<CoreAnswer<SaleView>> {
  await requireActive();
  if (!UUID.test(saleId)) return bad();
  const body: { ratePerMinute?: number; paused?: boolean } = {};
  if (typeof change.ratePerMinute === "number" && Number.isFinite(change.ratePerMinute)) body.ratePerMinute = Math.round(change.ratePerMinute);
  if (typeof change.paused === "boolean") body.paused = change.paused;
  return organiserCore<SaleView>("PATCH", `/v1/organiser/sales/${saleId}/live`, body);
}

export async function degradeAction(saleId: string, degraded: boolean): Promise<CoreAnswer<SaleView>> {
  await requireActive();
  if (!UUID.test(saleId)) return bad();
  return organiserCore<SaleView>("PATCH", `/v1/organiser/sales/${saleId}/degrade`, { degraded: degraded === true });
}
