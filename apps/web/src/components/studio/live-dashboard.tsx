"use client";
// The live on-sale dashboard (step 6, E8, design D-7/D-8). It opens the gateway's server-sent
// events straight from the browser with fetch (EventSource cannot send the organiser's token), a
// fresh 5-minute core token on every (re)connect, shows "stale" after 5 s without a frame and
// reconnects with back-off. Every number is the frame's; the browser only draws it.
import { useCallback, useEffect, useRef, useState, useTransition } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Card, Input } from "@/components/ui/field";
import { degradeAction, liveChangeAction } from "@/app/(platform)/studio/sales/[saleId]/actions";
import { parseEvents } from "@/lib/studio/sse";

type Provider = { state: "closed" | "open" | "half_open"; attempts: number; succeeded: number; failures: number };
type StockLine = { instance: string; pool: string; capacity: number; left: number; held: number; sold: number };
type Ev = { ts: number; kind: string; provider?: string; from?: string; to?: string; n?: number };
export type Frame = {
  ts: number;
  sale: string;
  name: string;
  rehearsal: boolean;
  phase: string;
  queue: { state: string; waiting: number; prequeue: number; slow: number; admittedTotal: number; admittedPerMin: number; active: number; rate: number; activeLimit: number; paused: boolean };
  sales: { sold: number; capacity: number; paidOrders: number; revenueMinor: number; currency: string; soldPerMin: number };
  payments: { attemptsPerMin: number; failuresPerMin: number; successRate: number | null; failoversTotal: number; providers: Record<string, Provider> };
  bots: { flagged: number; sentBack: number; passesRejected: number; otpChallenged: number; otpFailed: number };
  stock: StockLine[];
  stockReady: boolean;
  system: { gatewayRps: number | null; coreP95Ms: number | null; coreErrorShare: number | null; pods: Record<string, number> } | null;
  degraded: boolean;
  suggestDegrade: boolean;
  run?: { runId?: string; state?: string; truth?: Record<string, number> };
  timeline: Ev[];
};
type Point = { t: number; q: number; a: number; s: number; f: number };

const STALE_MS = 5000;
const ENDED = ["done", "failed", "stopped"];
const KEEP_MS = 15 * 60_000;

function Spark({ points, pick, label, markers, testId, fmt }: { points: Point[]; pick: (p: Point) => number; label: string; markers?: number[]; testId: string; fmt: (n: number) => string }) {
  const [hover, setHover] = useState<number | null>(null);
  const w = 300;
  const h = 64;
  const vals = points.map(pick);
  const max = Math.max(1, ...vals);
  const t0 = points[0]?.t ?? 0;
  const t1 = points[points.length - 1]?.t ?? 1;
  const x = (t: number) => (t1 === t0 ? w : ((t - t0) / (t1 - t0)) * w);
  const y = (v: number) => h - 2 - (v / max) * (h - 6);
  const line = points.map((p) => `${x(p.t).toFixed(1)},${y(pick(p)).toFixed(1)}`).join(" ");
  const hp = hover != null ? points[hover] : null;
  return (
    <figure className="min-w-0 space-y-1" data-testid={testId} data-points={points.length}>
      <figcaption className="flex justify-between gap-2 text-xs text-muted-foreground">
        <span>{label}</span>
        <span aria-live="polite">{hp ? fmt(pick(hp)) : vals.length ? fmt(vals[vals.length - 1]) : "–"}</span>
      </figcaption>
      <svg
        viewBox={`0 0 ${w} ${h}`}
        className="h-16 w-full"
        role="img"
        aria-label={label}
        preserveAspectRatio="none"
        onMouseMove={(e) => {
          if (!points.length) return;
          const r = e.currentTarget.getBoundingClientRect();
          const tx = t0 + ((e.clientX - r.left) / r.width) * (t1 - t0);
          let best = 0;
          points.forEach((p, i) => {
            if (Math.abs(p.t - tx) < Math.abs(points[best].t - tx)) best = i;
          });
          setHover(best);
        }}
        onMouseLeave={() => setHover(null)}
      >
        <line x1={0} y1={h - 1} x2={w} y2={h - 1} stroke="var(--border)" strokeWidth={1} vectorEffect="non-scaling-stroke" />
        {(markers ?? []).map((t) => (
          <line key={t} x1={x(t)} x2={x(t)} y1={0} y2={h} stroke="var(--muted-foreground)" strokeDasharray="3 3" strokeWidth={1} vectorEffect="non-scaling-stroke" data-testid={`${testId}-marker`} />
        ))}
        {points.length > 1 && <polyline points={line} fill="none" stroke="var(--primary)" strokeWidth={2} vectorEffect="non-scaling-stroke" strokeLinejoin="round" />}
        {hp && <line x1={x(hp.t)} x2={x(hp.t)} y1={0} y2={h} stroke="var(--foreground)" strokeWidth={1} vectorEffect="non-scaling-stroke" />}
      </svg>
    </figure>
  );
}

export function LiveDashboard({ saleId, gateway, admin, initialRate, initialPaused, initialDegraded }: { saleId: string; gateway: string; admin: boolean; initialRate: number; initialPaused: boolean; initialDegraded: boolean }) {
  const t = useTranslations("live");
  const locale = useLocale();
  const [frame, setFrame] = useState<Frame | null>(null);
  const [points, setPoints] = useState<Point[]>([]);
  const [last, setLast] = useState(0);
  const [now, setNow] = useState(() => Date.now());
  const [rate, setRate] = useState(initialRate);
  const [paused, setPaused] = useState(initialPaused);
  const [degraded, setDegraded] = useState(initialDegraded);
  const [notice, setNotice] = useState<string | null>(null);
  const [pending, start] = useTransition();
  const stop = useRef(false);
  // Timeline values in the page's language (review spec L2): breaker states have their own words.
  const val = (kind: string, v?: string) => {
    if (!v) return t("val.none");
    if (kind === "breaker" && t.has(`state_${v}`)) return t(`state_${v}` as "state_closed");
    return t.has(`val.${v}`) ? t(`val.${v}` as "val.on") : v;
  };
  const nf = useCallback((n: number, d = 0) => new Intl.NumberFormat(locale === "ro" ? "ro-RO" : "en-GB", { maximumFractionDigits: d }).format(n), [locale]);

  const lastTs = useRef(0);
  const onFrame = useCallback((f: Frame) => {
    setFrame(f);
    // Staleness is judged from the frame's own time (review D-g): a replayed or repeated frame
    // does not count as fresh.
    if (f.ts > lastTs.current) {
      lastTs.current = f.ts;
      setLast(Date.now());
    }
    setPaused(f.queue.paused);
    setDegraded(f.degraded);
    setPoints((ps) => {
      const p = { t: f.ts, q: f.queue.waiting, a: f.queue.admittedPerMin, s: f.sales.soldPerMin, f: f.payments.failoversTotal };
      const next = ps.length && ps[ps.length - 1].t >= p.t ? ps : [...ps, p];
      return next.filter((x) => p.t - x.t <= KEEP_MS);
    });
  }, []);

  useEffect(() => {
    stop.current = false;
    let abort: AbortController | null = null;
    let backoff = 1000;
    let lastFrame = Date.now();
    // A connection that went quiet (no frame or heartbeat-carried frame for 5 s) is dropped and
    // opened again: half-open connections are common on mobile networks.
    const watchdog = setInterval(() => {
      if (Date.now() - lastFrame > STALE_MS * 2) abort?.abort();
    }, 1000);
    // "eps-live-drop" cuts the connection as a network drop would (the e2e test of reconnecting).
    const drop = () => abort?.abort();
    window.addEventListener("eps-live-drop", drop);
    const loop = async () => {
      while (!stop.current) {
        abort = new AbortController();
        try {
          const tk = await fetch("/api/core-token", { cache: "no-store", credentials: "same-origin" });
          const { token } = (await tk.json()) as { token?: string };
          if (!token || !gateway) throw new Error("no token");
          const res = await fetch(`${gateway}/v1/live/${saleId}`, { headers: { Authorization: `Bearer ${token}`, Accept: "text/event-stream" }, signal: abort.signal, cache: "no-store" });
          if (!res.ok || !res.body) throw new Error(String(res.status));
          backoff = 1000;
          lastFrame = Date.now();
          const reader = res.body.getReader();
          const dec = new TextDecoder();
          let buf = "";
          for (;;) {
            const { value, done } = await reader.read();
            if (done) break;
            buf += dec.decode(value, { stream: true }).replace(/\r\n/g, "\n");
            const { events, rest } = parseEvents(buf);
            buf = rest;
            for (const e of events) {
              if (e.event === "history") {
                const hist = JSON.parse(e.data) as Point[];
                setPoints((ps) => (ps.length ? ps : hist));
              } else if (e.event === "frame") {
                lastFrame = Date.now();
                onFrame(JSON.parse(e.data) as Frame);
              }
            }
          }
        } catch {
          // dropped or refused: reconnect below
        }
        if (stop.current) return;
        await new Promise((r) => setTimeout(r, backoff + Math.random() * 500));
        backoff = Math.min(10_000, backoff * 2);
      }
    };
    void loop();
    const tick = setInterval(() => setNow(Date.now()), 1000);
    return () => {
      stop.current = true;
      abort?.abort();
      clearInterval(tick);
      clearInterval(watchdog);
      window.removeEventListener("eps-live-drop", drop);
    };
  }, [gateway, saleId, onFrame]);

  const stale = last > 0 && now - last > STALE_MS;
  const markers = points.filter((p, i) => i > 0 && p.f > points[i - 1].f).map((p) => p.t);
  const money = (minor: number, cur: string) =>
    new Intl.NumberFormat(locale === "ro" ? "ro-RO" : "en-GB", { style: "currency", currency: cur || "RON", maximumFractionDigits: 0 }).format(minor / 100);
  const act = (fn: () => Promise<{ ok: boolean; code?: string }>) =>
    start(async () => {
      const r = await fn();
      setNotice(r.ok ? t("saved") : t("failed", { code: r.code ?? "" }));
    });
  const f = frame;
  const tile = (k: string, v: string, id: string) => (
    <div className="min-w-0 rounded-md border p-3" key={id}>
      <p className="text-xs text-muted-foreground">{t(k)}</p>
      <p className="text-2xl font-semibold tabular-nums" data-testid={`tile-${id}`}>
        {v}
      </p>
    </div>
  );
  const lights: Record<string, string> = { closed: "●", open: "■", half_open: "◐" };
  return (
    <div className="space-y-4" data-testid="live-dashboard" data-connected={f ? "yes" : "no"} data-stale={stale ? "yes" : "no"}>
      <p className="text-sm" role="status" data-testid="live-status">
        {!f ? (
          t("connecting")
        ) : stale ? (
          <strong className="text-destructive" data-testid="live-stale">
            {t("stale", { n: nf(Math.round((now - last) / 1000)) })}
          </strong>
        ) : (
          <>
            <strong data-testid="live-ok">{t("live")}</strong> · {t("frameAt", { time: new Date(f.ts).toLocaleTimeString(locale === "ro" ? "ro-RO" : "en-GB") })}
          </>
        )}
      </p>
      {f?.suggestDegrade && (
        <p role="alert" className="rounded-md border p-2 text-sm font-semibold" data-testid="live-suggest">
          {t("suggest")}
        </p>
      )}
      {f && (
        <>
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3 lg:grid-cols-5" data-testid="live-tiles" data-frame-ts={f.ts}>
            {tile("inQueue", nf(f.queue.waiting), "queue")}
            {tile("prequeue", nf(f.queue.prequeue), "prequeue")}
            {tile("admittedPerMin", nf(f.queue.admittedPerMin), "admitted")}
            {tile("shopping", nf(f.queue.active), "shopping")}
            {tile("sold", `${nf(f.sales.sold)} / ${nf(f.sales.capacity)}`, "sold")}
            {tile("revenue", money(f.sales.revenueMinor, f.sales.currency), "revenue")}
            {tile("success", f.payments.successRate == null ? "–" : `${nf(f.payments.successRate * 100, 1)}%`, "success")}
            {tile("failuresPerMin", nf(f.payments.failuresPerMin), "failures")}
            {tile("failovers", nf(f.payments.failoversTotal), "failovers")}
            {tile("botsFlagged", nf(f.bots.flagged), "bots")}
          </div>
          <Card>
            <div className="grid gap-4 sm:grid-cols-3">
              <Spark points={points} pick={(p) => p.q} label={t("queueChart")} testId="spark-queue" fmt={nf} />
              <Spark points={points} pick={(p) => p.a} label={t("admittedChart")} testId="spark-admitted" fmt={nf} />
              <Spark points={points} pick={(p) => p.s} label={t("soldChart")} markers={markers} testId="spark-sold" fmt={nf} />
            </div>
          </Card>
          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <h3 className="font-semibold">{t("providers")}</h3>
              <ul className="space-y-2 text-sm" data-testid="live-providers">
                {(["a", "b"] as const).map((p) => {
                  const pv = f.payments.providers[p] ?? { state: "closed", attempts: 0, succeeded: 0, failures: 0 };
                  return (
                    <li key={p} data-testid={`provider-${p}`} data-state={pv.state}>
                      <span aria-hidden className={pv.state === "open" ? "text-destructive" : ""}>
                        {lights[pv.state] ?? "●"}
                      </span>{" "}
                      <strong>{t("provider", { p: p.toUpperCase() })}</strong>: {t(`state_${pv.state}`)}
                      <span className="block text-xs text-muted-foreground">{t("lastMinute", { n: nf(pv.attempts), f: nf(pv.failures) })}</span>
                    </li>
                  );
                })}
              </ul>
            </Card>
            <Card>
              <h3 className="font-semibold">{t("bots")}</h3>
              <dl className="grid grid-cols-2 gap-2 text-sm" data-testid="live-bots">
                {(
                  [
                    ["botsFlagged", f.bots.flagged, "flagged"],
                    ["sentBack", f.bots.sentBack, "sent-back"],
                    ["otpChallenged", f.bots.otpChallenged, "otp"],
                    ["otpFailed", f.bots.otpFailed, "otp-failed"],
                    ["passesRejected", f.bots.passesRejected, "passes"],
                  ] as const
                ).map(([k, v, id]) => (
                  <div key={id}>
                    <dt className="text-xs text-muted-foreground">{t(k)}</dt>
                    <dd className="font-semibold" data-testid={`bots-${id}`}>
                      {nf(v)}
                    </dd>
                  </div>
                ))}
              </dl>
              {f.run && !ENDED.includes(f.run.state ?? "") && (
                <p className="mt-2 text-xs text-muted-foreground" data-testid="live-truth-later">
                  {t("truthLater")}
                </p>
              )}
              {f.run?.truth && ENDED.includes(f.run.state ?? "") && (
                <p className="mt-2 text-sm" data-testid="live-truth">
                  <span className="font-semibold">{t("truth")}:</span>{" "}
                  {t("truthLine", {
                    flagged: nf(f.bots.flagged),
                    bots: nf(f.run.truth.botsSent ?? 0),
                    admitted: nf(f.run.truth.botsAdmitted ?? 0),
                    tickets: nf(f.run.truth.botTickets ?? 0),
                    otp: nf(f.run.truth.fansAskedOtp ?? 0),
                  })}
                </p>
              )}
            </Card>
          </div>
          <Card>
            <h3 className="font-semibold">{t("stock")}</h3>
            {!f.stockReady && <p className="text-xs text-muted-foreground">{t("stockNotReady")}</p>}
            <ul className="space-y-2" data-testid="live-stock">
              {f.stock.map((l) => {
                const pct = (n: number) => `${l.capacity > 0 ? (n / l.capacity) * 100 : 0}%`;
                return (
                  <li key={l.instance} className="min-w-0 text-sm" data-testid="stock-line" data-left={l.left} data-held={l.held} data-sold={l.sold}>
                    <div className="flex flex-wrap justify-between gap-2">
                      <span className="truncate font-semibold">
                        {l.pool}
                        {l.instance.includes("@") ? ` (${l.instance.split("@")[1]})` : ""}
                      </span>
                      <span className="tabular-nums">
                        {nf(l.left)} {t("left")} · {nf(l.held)} {t("held")} · {nf(l.sold)} {t("soldShort")}
                      </span>
                    </div>
                    <div className="mt-1 flex h-2 w-full gap-0.5 overflow-hidden rounded bg-muted" aria-hidden>
                      <span className="h-full rounded-l bg-primary" style={{ width: pct(l.sold) }} />
                      <span className="h-full bg-primary/50" style={{ width: pct(l.held) }} />
                    </div>
                  </li>
                );
              })}
            </ul>
          </Card>
          <div className="grid gap-4 md:grid-cols-2">
            <Card>
              <h3 className="font-semibold">{t("timeline")}</h3>
              {f.timeline.length === 0 ? (
                <p className="text-sm">{t("noEvents")}</p>
              ) : (
                <ol className="space-y-1 text-sm" data-testid="live-timeline">
                  {[...f.timeline].reverse().map((e, i) => (
                    <li key={`${e.ts}-${i}`} data-kind={e.kind} className="break-words">
                      <span className="tabular-nums text-muted-foreground">{new Date(e.ts).toLocaleTimeString(locale === "ro" ? "ro-RO" : "en-GB")}</span>{" "}
                      {t(`ev_${e.kind}` as "ev_breaker", { provider: (e.provider ?? "").toUpperCase(), from: val(e.kind, e.from), to: val(e.kind, e.to), n: nf(e.n ?? 0) })}
                    </li>
                  ))}
                </ol>
              )}
            </Card>
            <Card>
              <h3 className="font-semibold">{t("system")}</h3>
              {f.system ? (
                <dl className="grid grid-cols-3 gap-2 text-sm" data-testid="live-system">
                  <div>
                    <dt className="text-xs text-muted-foreground">{t("gatewayRps")}</dt>
                    <dd className="font-semibold">{f.system.gatewayRps == null ? "–" : nf(f.system.gatewayRps, 1)}</dd>
                  </div>
                  <div>
                    <dt className="text-xs text-muted-foreground">{t("coreP95")}</dt>
                    <dd className="font-semibold">{f.system.coreP95Ms == null ? "–" : `${nf(f.system.coreP95Ms)} ms`}</dd>
                  </div>
                  <div>
                    <dt className="text-xs text-muted-foreground">{t("coreErrors")}</dt>
                    <dd className="font-semibold">{f.system.coreErrorShare == null ? "–" : `${nf(f.system.coreErrorShare * 100, 1)}%`}</dd>
                  </div>
                  <p className="col-span-3 text-xs text-muted-foreground">
                    {t("pods", { gateway: nf(f.system.pods.gateway ?? 0), core: nf(f.system.pods.core ?? 0), worker: nf(f.system.pods.worker ?? 0) })}
                  </p>
                </dl>
              ) : (
                <p className="text-sm text-muted-foreground">{t("noSystem")}</p>
              )}
            </Card>
          </div>
        </>
      )}
      {admin && (
        <Card>
          <h3 className="font-semibold">{t("controls")}</h3>
          <div className="space-y-3">
            <form
              className="flex flex-wrap items-end gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                act(() => liveChangeAction(saleId, { ratePerMinute: rate }));
              }}
            >
              <label className="grid gap-1 text-sm">
                {t("rate")}
                <Input type="number" min={10} max={60000} value={rate} onChange={(e) => setRate(Number(e.target.value))} className="w-32" data-testid="live-rate" />
              </label>
              <input type="range" min={10} max={60000} step={10} value={rate} onChange={(e) => setRate(Number(e.target.value))} className="min-w-0 flex-1 accent-[var(--primary)]" aria-label={t("rate")} data-testid="live-rate-dial" />
              <Button type="submit" disabled={pending} data-testid="live-rate-apply">
                {t("apply")}
              </Button>
            </form>
            <div className="flex flex-wrap gap-2">
              <Button type="button" variant="outline" disabled={pending} onClick={() => act(() => liveChangeAction(saleId, { paused: !paused }))} data-testid="live-pause">
                {paused ? t("resume") : t("pause")}
              </Button>
              <Button type="button" variant={degraded ? "outline" : "destructive"} disabled={pending} onClick={() => act(() => degradeAction(saleId, !degraded))} data-testid="live-degrade" data-degraded={degraded ? "on" : "off"}>
                {degraded ? t("degradeOff") : t("degradeOn")}
              </Button>
            </div>
            {degraded && (
              <p className="text-sm font-semibold" data-testid="live-degraded">
                {t("degradedNow")}
              </p>
            )}
            <p className="text-xs text-muted-foreground">{t("degradeHint")}</p>
            {notice && (
              <p role="status" className="text-sm" data-testid="live-notice">
                {notice}
              </p>
            )}
          </div>
        </Card>
      )}
      {f && <p className="sr-only" data-testid="live-rate-setting">{f.queue.rate}</p>}
    </div>
  );
}
