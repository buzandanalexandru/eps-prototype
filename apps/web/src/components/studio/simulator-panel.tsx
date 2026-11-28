"use client";
// The simulator's run settings, Start and Stop, and the latest run (step 6, E7). The run's state,
// its fans by state and, once ended, its summary are read from the core every 2 seconds while it is
// active. Every figure is what the simulator and the system measured; nothing is computed here.
import { useEffect, useRef, useState, useTransition } from "react";
import { useRouter } from "next/navigation";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Card, Input, Label, Select } from "@/components/ui/field";
import { runAction, startRunAction, stopRunAction, type Knobs, type Run } from "@/app/(platform)/studio/sales/[saleId]/actions";

type Limits = { maxFans: number; maxDurationSeconds: number };
type StepStats = { count: number; errors: number; p50Ms: number; p95Ms: number };
type Truth = Record<string, number | string | undefined>;
type Report = { phase?: string; fans?: Record<string, number>; bots?: Record<string, number>; truth?: Truth; rps?: number; peakRps?: number; requests?: Record<string, StepStats> };
type Summary = {
  truth?: Truth;
  facts?: { paidOrders?: number; ticketsSold?: number; failovers?: number; attempts?: number; paidAtBDuringOutage?: number };
  paymentSuccessRate?: number;
  sellOutSeconds?: number;
  botsFlagged?: number;
  fansFlagged?: number;
  rps?: number;
  peakRps?: number;
  requests?: Record<string, StepStats>;
};

const ACTIVE = ["pending", "starting", "running", "finishing"];

function num(fd: FormData, k: string): number {
  const n = Number(String(fd.get(k) ?? "").replace(",", "."));
  return Number.isFinite(n) ? n : 0;
}

function knobsFrom(fd: FormData): Knobs {
  const mode = String(fd.get("providerA") ?? "outage") as "none" | "outage" | "failure_rate";
  return {
    fans: Math.round(num(fd, "fans")),
    botShare: num(fd, "botShare"),
    prequeueShare: num(fd, "prequeueShare"),
    arrival: fd.get("arrival") === "flat" ? "flat" : "spike",
    opensInSeconds: Math.round(num(fd, "opensInSeconds")),
    qtyMix: { "1": num(fd, "q1"), "2": num(fd, "q2"), "3": num(fd, "q3"), "4": num(fd, "q4") },
    paymentMix: { hosted_card: num(fd, "pCard"), apple_pay: num(fd, "pApple"), google_pay: num(fd, "pGoogle") },
    abandonRate: num(fd, "abandonRate"),
    declineRate: num(fd, "declineRate"),
    pollSeconds: Math.round(num(fd, "pollSeconds")),
    providerA: { mode, atSeconds: Math.round(num(fd, "atSeconds")), forSeconds: Math.round(num(fd, "forSeconds")), ...(mode === "failure_rate" ? { failureRate: num(fd, "failureRate") } : {}) },
    durationSeconds: Math.round(num(fd, "durationSeconds")),
  };
}

export function SimulatorPanel({ rehearsalId, limits, initial, admin }: { sourceSaleId: string; rehearsalId: string; limits: Limits; initial: Run | null; admin: boolean }) {
  const t = useTranslations("simulator");
  const locale = useLocale();
  const router = useRouter();
  const [run, setRun] = useState<Run | null>(initial);
  const [error, setError] = useState<string | null>(null);
  const [pending, start] = useTransition();
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const active = !!run && ACTIVE.includes(run.state);
  const fmt = (n: number | undefined, digits = 0) => new Intl.NumberFormat(locale === "ro" ? "ro-RO" : "en-GB", { maximumFractionDigits: digits }).format(n ?? 0);

  useEffect(() => {
    if (!run || !ACTIVE.includes(run.state)) return;
    timer.current = setTimeout(async () => {
      const r = await runAction(run.id);
      if (r.ok) {
        setRun(r.data);
        if (!ACTIVE.includes(r.data.state)) router.refresh();
      } else setRun({ ...run }); // poll again
    }, 2000);
    return () => {
      if (timer.current) clearTimeout(timer.current);
    };
  }, [run, router]);

  const explain = (code: string, data?: Record<string, unknown>) => {
    if (code === "EPS-SIM-INVALID" && typeof data?.field === "string") return t("errorRange", { field: data.field, min: String(data.min ?? ""), max: String(data.max ?? "") });
    if (code === "EPS-SIM-ACTIVE") return t("errorActive");
    if (code === "EPS-SIM-BUSY") return t("errorBusy");
    return code;
  };

  const onStart = (fd: FormData) =>
    start(async () => {
      setError(null);
      const r = await startRunAction(rehearsalId, knobsFrom(fd));
      if (!r.ok) {
        setError(explain(r.code, r.data));
        return;
      }
      setRun(r.data);
      router.refresh(); // the run may be on a fresh rehearsal
    });
  const onStop = () =>
    start(async () => {
      if (!run) return;
      const r = await stopRunAction(run.id);
      if (r.ok) setRun(r.data);
    });

  const report = (run?.report ?? null) as Report | null;
  const summary = (run?.summary ?? null) as Summary | null;
  const req = summary?.requests ?? report?.requests;
  const rps = summary?.rps ?? report?.rps;
  const peak = summary?.peakRps ?? report?.peakRps;
  const arrived = (sent: string, planned: string) =>
    t("ofPlanned", { sent: fmt(Number(summary?.truth?.[sent] ?? 0)), planned: fmt(Number(summary?.truth?.[planned] ?? 0)) });
  const field = "grid gap-1 text-sm";
  return (
    <>
      {admin && (
        <Card>
          <form action={onStart} className="space-y-3" data-testid="simulator-form">
            <h3 className="font-semibold">{t("knobs")}</h3>
            <div className="grid gap-3 sm:grid-cols-2">
              <Label className={field}>
                {t("fans", { max: fmt(limits.maxFans) })}
                <Input type="number" name="fans" min={10} max={limits.maxFans} defaultValue={Math.min(1000, limits.maxFans)} data-testid="knob-fans" />
              </Label>
              <Label className={field}>
                {t("botShare")}
                <Input type="number" name="botShare" min={0} max={0.5} step={0.01} defaultValue={0.1} data-testid="knob-bot-share" />
              </Label>
              <Label className={field}>
                {t("prequeueShare")}
                <Input type="number" name="prequeueShare" min={0} max={1} step={0.05} defaultValue={0.6} data-testid="knob-prequeue" />
              </Label>
              <Label className={field}>
                {t("arrival")}
                <Select name="arrival" defaultValue="spike" data-testid="knob-arrival">
                  <option value="spike">{t("arrival_spike")}</option>
                  <option value="flat">{t("arrival_flat")}</option>
                </Select>
              </Label>
              <Label className={field}>
                {t("opensIn")}
                <Input type="number" name="opensInSeconds" min={30} max={600} defaultValue={60} data-testid="knob-opens-in" />
              </Label>
              <Label className={field}>
                {t("duration", { max: limits.maxDurationSeconds })}
                <Input type="number" name="durationSeconds" min={60} max={limits.maxDurationSeconds} defaultValue={300} data-testid="knob-duration" />
              </Label>
              <fieldset className="grid gap-1 text-sm sm:col-span-2">
                <legend>{t("qtyMix")}</legend>
                <div className="grid grid-cols-4 gap-2">
                  {(
                    [
                      ["q1", 35],
                      ["q2", 40],
                      ["q3", 10],
                      ["q4", 15],
                    ] as const
                  ).map(([n, v], i) => (
                    <Input key={n} type="number" name={n} min={0} max={100} defaultValue={v} aria-label={`${t("qtyMix")} ${i + 1}`} data-testid={`knob-${n}`} />
                  ))}
                </div>
              </fieldset>
              <fieldset className="grid gap-1 text-sm sm:col-span-2">
                <legend>{t("paymentMix")}</legend>
                <div className="grid grid-cols-3 gap-2">
                  {(
                    [
                      ["pCard", 60, "Card"],
                      ["pApple", 20, "Apple Pay"],
                      ["pGoogle", 20, "Google Pay"],
                    ] as const
                  ).map(([n, v, l]) => (
                    <Input key={n} type="number" name={n} min={0} max={100} defaultValue={v} aria-label={l} data-testid={`knob-${n}`} />
                  ))}
                </div>
              </fieldset>
              <Label className={field}>
                {t("abandonRate")}
                <Input type="number" name="abandonRate" min={0} max={0.5} step={0.01} defaultValue={0.08} data-testid="knob-abandon" />
              </Label>
              <Label className={field}>
                {t("declineRate")}
                <Input type="number" name="declineRate" min={0} max={0.5} step={0.01} defaultValue={0.03} data-testid="knob-decline" />
              </Label>
              <Label className={field}>
                {t("pollSeconds")}
                <Input type="number" name="pollSeconds" min={5} max={30} defaultValue={10} data-testid="knob-poll" />
              </Label>
              <Label className={field}>
                {t("providerA")}
                <Select name="providerA" defaultValue="outage" data-testid="knob-provider-a">
                  <option value="none">{t("providerA_none")}</option>
                  <option value="outage">{t("providerA_outage")}</option>
                  <option value="failure_rate">{t("providerA_failure_rate")}</option>
                </Select>
              </Label>
              <Label className={field}>
                {t("atSeconds")}
                <Input type="number" name="atSeconds" min={0} max={600} defaultValue={60} data-testid="knob-at" />
              </Label>
              <Label className={field}>
                {t("forSeconds")}
                <Input type="number" name="forSeconds" min={10} max={600} defaultValue={90} data-testid="knob-for" />
              </Label>
              <Label className={field}>
                {t("failureRate")}
                <Input type="number" name="failureRate" min={0} max={1} step={0.05} defaultValue={0.5} data-testid="knob-failure-rate" />
              </Label>
            </div>
            {error && (
              <p role="alert" className="text-sm font-semibold text-destructive" data-testid="simulator-start-error">
                {error}
              </p>
            )}
            <div className="flex flex-wrap gap-2">
              <Button type="submit" disabled={pending || active} data-testid="simulator-start">
                {pending ? t("starting") : t("start")}
              </Button>
              {active && (
                <Button type="button" variant="destructive" onClick={onStop} disabled={pending} data-testid="simulator-stop">
                  {t("stop")}
                </Button>
              )}
            </div>
          </form>
        </Card>
      )}
      <Card>
        <section className="space-y-3" data-testid="simulator-run" data-state={run?.state ?? "none"} data-run-id={run?.id ?? ""}>
          <h3 className="font-semibold">{t("run")}</h3>
          {!run && <p className="text-sm">{t("noRun")}</p>}
          {run && (
            <>
              <p className="text-sm">
                {t("state")}:{" "}
                <strong data-testid="simulator-state">{t(`state_${run.state}`)}</strong>
                {run.error ? ` · ${run.error}` : ""}
              </p>
              <p className="text-sm">
                {fmt(run.knobs.fans)} · {t("botShare")}: {fmt(run.knobs.botShare, 2)}
              </p>
              {report?.fans && Object.keys(report.fans).length > 0 && (
                <p className="text-xs break-words" data-testid="simulator-fans">
                  {t("fansByState")}:{" "}
                  {Object.entries(report.fans)
                    .map(([k, v]) => `${k} ${fmt(v)}`)
                    .join(" · ")}
                </p>
              )}
              {report?.bots && Object.keys(report.bots).length > 0 && (
                <p className="text-xs break-words" data-testid="simulator-bots">
                  {t("botsByState")}:{" "}
                  {Object.entries(report.bots)
                    .map(([k, v]) => `${k} ${fmt(v)}`)
                    .join(" · ")}
                </p>
              )}
            </>
          )}
          {summary && (
            <dl className="grid grid-cols-2 gap-2 text-sm sm:grid-cols-3" data-testid="simulator-summary">
              {(
                [
                  ["sFansSent", arrived("fansSent", "fansPlanned")],
                  ["sBotsSent", arrived("botsSent", "botsPlanned")],
                  ["sAdmitted", fmt(Number(summary.truth?.fansAdmitted ?? 0))],
                  ["sBotsAdmitted", fmt(Number(summary.truth?.botsAdmitted ?? 0))],
                  ["sTickets", fmt(summary.facts?.ticketsSold)],
                  ["sBotTickets", fmt(Number(summary.truth?.botTickets ?? 0))],
                  ["sSellOut", summary.sellOutSeconds != null ? t("seconds", { n: fmt(summary.sellOutSeconds) }) : t("sNoSellOut")],
                  ["sSuccess", `${fmt((summary.paymentSuccessRate ?? 0) * 100, 1)}%`],
                  ["sFailovers", fmt(summary.facts?.failovers)],
                  ["sPaidAtB", fmt(summary.facts?.paidAtBDuringOutage ?? 0)],
                  ["sBotsFlagged", fmt(summary.botsFlagged)],
                  ["sFansFlagged", fmt(summary.fansFlagged)],
                  ["sFansOtp", fmt(Number(summary.truth?.fansAskedOtp ?? 0))],
                  ["sSkips", fmt(Number(summary.truth?.skipsRefused ?? 0))],
                  ["sRps", fmt(summary.rps, 1)],
                  ["sPeakRps", fmt(summary.peakRps ?? 0)],
                ] as const
              ).map(([k, v]) => (
                <div key={k} className="min-w-0">
                  <dt className="text-xs text-muted-foreground">{t(k)}</dt>
                  <dd className="font-semibold" data-testid={`summary-${k}`}>
                    {v}
                  </dd>
                </div>
              ))}
            </dl>
          )}
          <h4 className="text-sm font-semibold">{t("loadRunTitle")}</h4>
          <p className="text-sm" data-testid="simulator-load-run">
            {rps
              ? t("loadMeasuredRun", { rps: fmt(rps, 1), peak: fmt(peak ?? 0), join: fmt(req?.join?.p95Ms, 1), status: fmt(req?.status?.p95Ms, 1), hold: fmt(req?.hold?.p95Ms, 1) })
              : t("loadNoRun")}
          </p>
        </section>
      </Card>
    </>
  );
}
