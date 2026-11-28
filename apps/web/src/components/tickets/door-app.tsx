"use client";
// The door page (G2, design D5 to D8): a phone of the organiser's door staff that, once prepared
// online, checks rotating codes with no network (signature, event, time window by the corrected
// clock, revocations, suspensions, one entry per ticket), stores scans first and uploads them
// idempotently, and syncs the event's door feed every 30 seconds while online.
import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslations } from "next-intl";
import { applyFeed, applyManifest, admit, checkScan, correctedNow, emptyState, recordSync, syncAge, type FeedRow, type ManifestItem, type ScanResult } from "@/lib/door/verify";
import { forget, load, save, type Loaded, type LogLine } from "@/lib/door/store";
import { fromBase64Url } from "@/lib/tickets/token";
import { call } from "./api";

type KeysAnswer = { serverTime: string; keys: { keyId: number; publicKey: string }[] };
type SyncAnswer = {
  serverTime: string;
  cursor: number;
  more: boolean;
  feed: FeedRow[];
  manifest?: ManifestItem[];
  manifestNext?: string;
  products?: Record<string, { ro: string; en: string }>;
  event?: { name: { ro: string; en: string }; timezone: string };
};
type Detector = { detect: (src: CanvasImageSource | ImageBitmap) => Promise<{ rawValue: string }[]> };

const SYNC_EVERY_MS = 30_000;

function fmtClock(ms: number, locale: string, tz: string) {
  try {
    return new Intl.DateTimeFormat(locale === "ro" ? "ro-RO" : "en-GB", { hour: "2-digit", minute: "2-digit", hour12: false, timeZone: tz }).format(new Date(ms));
  } catch {
    return new Date(ms).toISOString().slice(11, 16);
  }
}

function beep(ok: boolean) {
  try {
    const AC = window.AudioContext ?? (window as unknown as { webkitAudioContext?: typeof AudioContext }).webkitAudioContext;
    if (!AC) return;
    const ctx = new AC();
    const o = ctx.createOscillator();
    o.frequency.value = ok ? 880 : 220;
    o.connect(ctx.destination);
    o.start();
    o.stop(ctx.currentTime + (ok ? 0.12 : 0.35));
    o.onended = () => void ctx.close();
  } catch {
    // no sound on this device
  }
  try {
    navigator.vibrate?.(ok ? 80 : [200, 80, 200]);
  } catch {
    // no vibration
  }
}

export function DoorApp({ eventId, locale }: { eventId: string; locale: "ro" | "en" }) {
  const t = useTranslations("door");
  const data = useRef<Loaded | null>(null);
  // What the page renders: a fresh copy of the stored record after every change.
  const [view, setView] = useState<Loaded | null>(null);
  const [hasDetector, setHasDetector] = useState(false);
  const [phase, setPhase] = useState<"loading" | "unprepared" | "preparing" | "ready" | "denied">("loading");
  const [gate, setGate] = useState("Gate A");
  const [progress, setProgress] = useState(0);
  const [online, setOnline] = useState(true);
  const [now, setNow] = useState(() => Date.now());
  const [result, setResult] = useState<(ScanResult & { at: number }) | null>(null);
  const [code, setCode] = useState("");
  const [camera, setCamera] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const syncing = useRef(false);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const lastCode = useRef<{ code: string; at: number } | null>(null);

  const persist = useCallback(async () => {
    if (data.current) await save(data.current).catch(() => undefined);
    setView(data.current ? { ...data.current } : null);
  }, []);

  // ---- sync: upload waiting scans, then the feed since the cursor, then the clock offset
  const sync = useCallback(async () => {
    const d = data.current;
    if (!d || !d.device.prepared || syncing.current) return;
    syncing.current = true;
    try {
      while (d.pending.length > 0) {
        const batch = d.pending.slice(0, 200);
        const r = await call<{ results: { id: string; result: string; firstAt?: string; firstGate?: string }[] }>("POST", `/api/door/events/${eventId}/door/scans`, {
          deviceId: d.device.deviceId,
          scans: batch,
        });
        if (r.status !== 200) break;
        const done = new Set(r.body.results.map((x) => x.id));
        d.pending = d.pending.filter((p) => !done.has(p.id));
        await persist();
      }
      for (let i = 0; i < 20; i++) {
        const sent = Date.now();
        const r = await call<SyncAnswer>("GET", `/api/door/events/${eventId}/door-sync?since=${d.state.cursor}&device=${d.device.deviceId}`);
        const got = Date.now();
        if (r.status !== 200) {
          if (r.status === 404) setPhase("denied");
          break;
        }
        applyFeed(d.state, r.body.feed);
        recordSync(d.state, r.body.serverTime, sent, got);
        await persist();
        if (!r.body.more) break;
      }
    } finally {
      syncing.current = false;
    }
  }, [eventId, persist]);

  useEffect(() => {
    let alive = true;
    void load(eventId)
      .then((l) => {
        if (!alive) return;
        if (l && l.device.prepared) {
          data.current = l;
          setView({ ...l });
          setGate(l.device.gate);
          setPhase("ready");
        } else setPhase("unprepared");
      })
      .catch(() => alive && setPhase("unprepared"));
    if ("serviceWorker" in navigator) navigator.serviceWorker.register("/door-sw.js", { scope: "/door/" }).catch(() => undefined);
    const first = window.setTimeout(() => {
      setOnline(navigator.onLine);
      setHasDetector("BarcodeDetector" in window);
    }, 0);
    const on = () => setOnline(true);
    const off = () => setOnline(false);
    window.addEventListener("online", on);
    window.addEventListener("offline", off);
    const tick = window.setInterval(() => setNow(Date.now()), 5000);
    return () => {
      alive = false;
      window.removeEventListener("online", on);
      window.removeEventListener("offline", off);
      window.clearInterval(tick);
      window.clearTimeout(first);
    };
  }, [eventId]);

  useEffect(() => {
    if (phase !== "ready" || !online) return;
    void sync();
    const id = window.setInterval(() => void sync(), SYNC_EVERY_MS);
    return () => window.clearInterval(id);
  }, [phase, online, sync]);

  // ---- prepare: device, keys, manifest pages, server time; then keep the page for offline use
  const prepare = async () => {
    setError(null);
    setPhase("preparing");
    setProgress(0);
    const dev = await call<{ deviceId: string; gate: string }>("POST", `/api/door/events/${eventId}/door/devices`, { gate });
    if (dev.status === 404 || dev.status === 401) return setPhase("denied");
    if (dev.status !== 201) {
      setError(t("unreachable"));
      return setPhase("unprepared");
    }
    const keys = await call<KeysAnswer>("GET", "/api/fan/ticket-keys");
    if (keys.status !== 200) {
      setError(t("unreachable"));
      return setPhase("unprepared");
    }
    const state = emptyState(eventId);
    for (const k of keys.body.keys) {
      const raw = fromBase64Url(k.publicKey);
      if (raw && raw.length === 32) state.keys.set(k.keyId, raw);
    }
    let after = "";
    for (let page = 0; page < 200; page++) {
      const sent = Date.now();
      const r = await call<SyncAnswer>("GET", `/api/door/events/${eventId}/door-sync?manifest=true${after ? `&after=${after}` : ""}&device=${dev.body.deviceId}`);
      const got = Date.now();
      if (r.status !== 200) {
        setError(t("unreachable"));
        return setPhase(r.status === 404 ? "denied" : "unprepared");
      }
      if (page === 0) {
        state.cursor = r.body.cursor;
        state.products = r.body.products ?? {};
        if (r.body.event) state.products["__event"] = r.body.event.name;
        if (r.body.event?.timezone) state.products["__tz"] = { ro: r.body.event.timezone, en: r.body.event.timezone };
      }
      applyManifest(state, r.body.manifest ?? []);
      recordSync(state, r.body.serverTime, sent, got);
      setProgress(state.tickets.size);
      if (!r.body.manifestNext) break;
      after = r.body.manifestNext;
    }
    data.current = { state, device: { deviceId: dev.body.deviceId, gate: dev.body.gate, prepared: true }, pending: [], log: [] };
    await persist();
    // The page and its scripts are kept for offline reloads.
    try {
      const reg = await navigator.serviceWorker?.ready;
      const urls = [window.location.pathname, ...performance.getEntriesByType("resource").map((e) => e.name).filter((u) => u.startsWith(window.location.origin) && new URL(u).pathname.startsWith("/_next/static/"))];
      reg?.active?.postMessage({ type: "cache", urls });
    } catch {
      // without a service worker the page still works while it stays open
    }
    setPhase("ready");
    void sync();
  };

  // ---- a scan
  const handle = useCallback(
    async (raw: string) => {
      const d = data.current;
      if (!d) return;
      const text = raw.trim();
      if (!text) return;
      const nowMs = Date.now();
      if (lastCode.current && lastCode.current.code === text && nowMs - lastCode.current.at < 2500) return;
      lastCode.current = { code: text, at: nowMs };
      const r = await checkScan(text, d.state, nowMs);
      const at = correctedNow(d.state, nowMs);
      if (r.ok) {
        admit(d.state, r.token.ticketId, { at, gate: d.device.gate, device: d.device.deviceId });
        d.pending.push({ id: crypto.randomUUID(), ticketId: r.token.ticketId, generation: r.token.generation, scannedAt: new Date(at).toISOString() });
      }
      const line: LogLine = { at, ok: r.ok, reason: r.ok ? "valid" : r.reason, product: r.ok ? r.product : undefined, name: r.ok ? r.name : undefined };
      d.log.push(line);
      await persist();
      beep(r.ok);
      setResult({ ...r, at });
      window.setTimeout(() => setResult((cur) => (cur && cur.at === at ? null : cur)), 1800);
      if (r.ok && navigator.onLine) void sync();
    },
    [persist, sync],
  );

  // ---- the camera, where the browser reads QR codes
  const stopCamera = () => {
    streamRef.current?.getTracks().forEach((tr) => tr.stop());
    streamRef.current = null;
    setCamera(false);
  };
  const startCamera = async () => {
    if (!hasDetector) return;
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: "environment" } });
      streamRef.current = stream;
      setCamera(true);
      const det = new (window as unknown as { BarcodeDetector: new (o: { formats: string[] }) => Detector }).BarcodeDetector({ formats: ["qr_code"] });
      const loop = async () => {
        if (!streamRef.current || !videoRef.current) return;
        try {
          const found = await det.detect(videoRef.current);
          if (found[0]?.rawValue) await handle(found[0].rawValue);
        } catch {
          // a frame that could not be read
        }
        window.setTimeout(() => void loop(), 250);
      };
      window.setTimeout(() => {
        if (videoRef.current) {
          videoRef.current.srcObject = stream;
          void videoRef.current.play().catch(() => undefined);
        }
        void loop();
      }, 50);
    } catch {
      setError(t("noCamera"));
    }
  };
  useEffect(() => () => streamRef.current?.getTracks().forEach((tr) => tr.stop()), []);
  const scanImage = async (file: File | undefined) => {
    if (!file || !hasDetector) return;
    try {
      const det = new (window as unknown as { BarcodeDetector: new (o: { formats: string[] }) => Detector }).BarcodeDetector({ formats: ["qr_code"] });
      const found = await det.detect(await createImageBitmap(file));
      if (found[0]?.rawValue) await handle(found[0].rawValue);
      else setResult({ ok: false, reason: "invalid", at: Date.now() });
    } catch {
      setResult({ ok: false, reason: "invalid", at: Date.now() });
    }
  };

  const d = view;
  const tz = d?.state.products["__tz"]?.en ?? "Europe/Bucharest";
  const age = d ? syncAge(d.state, now) : null;
  const reasonText = (r: ScanResult) =>
    r.ok
      ? t("valid")
      : r.reason === "already" && r.entry
        ? t("reason_already", { time: fmtClock(r.entry.at, locale, tz), gate: r.entry.gate || "—" })
        : t(`reason_${r.reason}` as "reason_invalid");
  const productName = (id?: string) => (id && d?.state.products[id] ? d.state.products[id][locale] || d.state.products[id].en : "");

  if (phase === "denied") {
    return (
      <p role="alert" data-testid="door-denied">
        {t("denied")}
      </p>
    );
  }
  return (
    <div className="space-y-4" data-testid="door" data-phase={phase}>
      {d && (
        <header className={`space-y-1 rounded-xl border p-3 text-sm ${age?.stale ? "border-amber-500 bg-amber-100 text-amber-950" : ""}`} data-testid="door-header" data-stale={age?.stale ? "true" : "false"}>
          <p className="font-semibold break-words">
            {(d.state.products["__event"]?.[locale] ?? "") || t("title")} · {t("gate")}: {d.device.gate}
          </p>
          <p data-testid="door-sync-age">
            {online ? t("online") : t("offline")} · {age?.minutes === 0 ? t("lastSyncNow") : age?.minutes !== null && age?.minutes !== undefined ? t("lastSync", { minutes: age.minutes }) : ""}
          </p>
          {age?.neverClock && <p data-testid="door-no-clock">{t("neverClock")}</p>}
          {d.pending.length > 0 && <p data-testid="door-pending">{t("pending", { n: d.pending.length })}</p>}
        </header>
      )}

      {(phase === "unprepared" || phase === "preparing") && (
        <div className="space-y-3 rounded-xl border p-4">
          <p className="font-semibold">{t("notPrepared")}</p>
          <p className="text-sm">{t("prepareLead")}</p>
          <label className="grid gap-1 text-sm">
            {t("gate")}
            <input className="w-full rounded-md border bg-background px-2 py-2" value={gate} onChange={(e) => setGate(e.target.value)} maxLength={40} data-testid="door-gate" />
          </label>
          <button type="button" className="w-full rounded-md bg-foreground px-3 py-3 font-semibold text-background disabled:opacity-50" disabled={phase === "preparing" || !gate.trim()} onClick={() => void prepare()} data-testid="door-prepare">
            {phase === "preparing" ? t("preparing", { done: progress }) : t("prepare")}
          </button>
        </div>
      )}

      {error && (
        <p role="alert" className="text-sm" data-testid="door-error">
          {error}
        </p>
      )}

      {phase === "ready" && d && (
        <div className="space-y-3">
          <p className="text-lg font-bold" data-testid="door-ready">
            {t("ready")}
          </p>
          {hasDetector ? (
            <div className="space-y-2">
              {camera ? (
                <>
                  <video ref={videoRef} className="aspect-square w-full rounded-xl bg-black object-cover" muted playsInline />
                  <button type="button" className="w-full rounded-md border px-3 py-2 font-semibold" onClick={stopCamera}>
                    {t("stopCamera")}
                  </button>
                </>
              ) : (
                <button type="button" className="w-full rounded-md bg-foreground px-3 py-3 font-semibold text-background" onClick={() => void startCamera()} data-testid="door-camera">
                  {t("scanCamera")}
                </button>
              )}
              <label className="block w-full cursor-pointer rounded-md border px-3 py-2 text-center text-sm font-semibold">
                {t("scanImage")}
                <input type="file" accept="image/*" capture="environment" className="sr-only" onChange={(e) => void scanImage(e.target.files?.[0])} data-testid="door-image" />
              </label>
            </div>
          ) : (
            <p className="text-sm text-muted-foreground" data-testid="door-no-camera">
              {t("noCamera")}
            </p>
          )}
          <form
            className="flex gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              void handle(code);
              setCode("");
            }}
          >
            <label className="sr-only" htmlFor="door-code">
              {t("enterCode")}
            </label>
            <input id="door-code" className="min-w-0 flex-1 rounded-md border bg-background px-2 py-2 font-mono text-xs" value={code} onChange={(e) => setCode(e.target.value)} placeholder={t("enterCode")} autoComplete="off" data-testid="door-code-input" />
            <button type="submit" className="shrink-0 rounded-md bg-foreground px-3 py-2 text-sm font-semibold text-background" data-testid="door-check">
              {t("check")}
            </button>
          </form>
          <div className="flex flex-wrap gap-2 text-sm">
            <button type="button" className="rounded-md border px-3 py-1.5" disabled={!online} onClick={() => void sync()} data-testid="door-sync">
              {t("sync")}
            </button>
            <button
              type="button"
              className="rounded-md border px-3 py-1.5"
              onClick={async () => {
                await forget(eventId);
                data.current = null;
                setView(null);
                setPhase("unprepared");
              }}
            >
              {t("reset")}
            </button>
          </div>
          {d.state.conflicts.length > 0 && (
            <div className="space-y-1" data-testid="door-conflicts">
              <p className="font-semibold">{t("conflicts")}</p>
              <ul className="space-y-1 text-sm">
                {d.state.conflicts.slice(-10).reverse().map((c, i) => (
                  <li key={i}>{t("conflict", { first: fmtClock(c.first.at, locale, tz), firstGate: c.first.gate, dup: fmtClock(c.dup.at, locale, tz), dupGate: c.dup.gate })}</li>
                ))}
              </ul>
            </div>
          )}
          <div className="space-y-1">
            <p className="font-semibold">{t("log")}</p>
            <ul className="space-y-1 text-sm" data-testid="door-log">
              {d.log.slice(-10).reverse().map((l, i) => (
                <li key={i} className={l.ok ? "text-green-800" : "text-red-800"}>
                  {fmtClock(l.at, locale, tz)} · {l.ok ? t("valid") : t(`reason_${l.reason}` as "reason_invalid", { time: "", gate: "" })} {l.product ? `· ${productName(l.product)}` : ""}
                </li>
              ))}
            </ul>
          </div>
        </div>
      )}

      {result && (
        <button
          type="button"
          className={`fixed inset-0 z-50 flex flex-col items-center justify-center gap-3 p-6 text-center ${result.ok ? "bg-green-600" : "bg-red-600"} text-white`}
          onClick={() => setResult(null)}
          data-testid="door-result"
          data-ok={result.ok ? "true" : "false"}
          data-reason={result.ok ? "valid" : result.reason}
        >
          <span className="text-4xl font-black">{result.ok ? "✓" : "✕"}</span>
          <span className="text-2xl font-bold break-words" data-testid="door-result-text">
            {reasonText(result)}
          </span>
          {result.ok && (
            <span className="text-lg break-words">
              {productName(result.product)}
              {result.name ? ` · ${result.name}` : ""}
            </span>
          )}
        </button>
      )}
    </div>
  );
}
