"use server";
// Studio's step 4 actions: the simulated providers' knobs, stand payments and the season close.
import { redirect } from "next/navigation";
import { organiserCore } from "@/lib/checkout/studio-core";
import { requireActive } from "@/lib/studio/context";

const num = (fd: FormData, k: string, max: number) => {
  const n = Number(String(fd.get(k) ?? "0").replace(",", "."));
  return Number.isFinite(n) ? Math.min(Math.max(n, 0), max) : 0;
};

export async function setKnobsAction(fd: FormData) {
  await requireActive();
  const provider = fd.get("provider") === "b" ? "b" : "a";
  const body = {
    outage: fd.get("outage") === "on",
    outage_mode: fd.get("outage_mode") === "hang" ? "hang" : "503",
    failure_rate: num(fd, "failure_rate", 1),
    decline_rate: num(fd, "decline_rate", 1),
    challenge_rate: num(fd, "challenge_rate", 1),
    timeout_rate: num(fd, "timeout_rate", 1),
    webhook_drop_rate: num(fd, "webhook_drop_rate", 1),
    latency_ms: Math.round(num(fd, "latency_ms", 30000)),
    latency_jitter_ms: Math.round(num(fd, "latency_jitter_ms", 30000)),
    webhook_delay_ms: Math.round(num(fd, "webhook_delay_ms", 300000)),
  };
  const r = await organiserCore("PUT", `/v1/organiser/payments/providers/${provider}/knobs`, body);
  redirect(`/studio/payments?${r.ok ? `saved=${provider}` : `error=${encodeURIComponent(r.code)}`}`);
}

export async function closeSeasonAction(fd: FormData) {
  await requireActive();
  const date = String(fd.get("expires") ?? "");
  const at = /^\d{4}-\d{2}-\d{2}$/.test(date) ? `${date}T23:59:59Z` : "";
  const r = await organiserCore<{ fans: number; totalMinor: number }>("POST", "/v1/organiser/wallet/season-close", { expiresAt: at });
  redirect(`/studio/payments?${r.ok ? `closed=${r.data.fans}&total=${r.data.totalMinor}` : `error=${encodeURIComponent(r.code)}`}`);
}

export async function standPaymentAction(fd: FormData) {
  await requireActive();
  const code = String(fd.get("code") ?? "").trim().toUpperCase();
  const amount = Math.round(num(fd, "amount", 2000) * 100);
  const label = String(fd.get("label") ?? "").slice(0, 80);
  const r = await organiserCore<{ balanceMinor: number; creditMinor: number; currency: string }>("POST", "/v1/organiser/wallet/stand-payments", { code, amountMinor: amount, label });
  redirect(`/studio/stand?${r.ok ? `charged=${amount}&left=${r.data.balanceMinor + r.data.creditMinor}&currency=${r.data.currency}` : `error=${encodeURIComponent(r.code)}`}`);
}
