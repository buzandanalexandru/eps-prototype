"use client";
// The builder (design D2.4, D2.8): three panes (pages and sections, the preview frame, settings),
// collapsing to tabs below 1024 px. The draft lives in client state with undo and redo; every change
// is posted to the frame (batched per animation frame) and autosaved after 800 ms with its revision.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useLocale, useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { CopyButton } from "@/components/studio/copy-button";
import { acceptFromFrame, readyMessage, renderedMessage, selectMessage } from "@/lib/storefront/frame-protocol";
import { publishProblems } from "@/lib/storefront/publish-checks";
import type { Locale, StorefrontDocument } from "@/lib/storefront/schema";
import type { LinkView, VersionView } from "@/app/studio/builder/actions";
import { TreePanel } from "./tree-panel";
import { SettingsPanel } from "./settings-panel";
import { BrandKitPanel } from "./brand-kit-panel";
import { CheckoutPanel, MessagesPanel } from "./messages-panel";
import { PublishPanel } from "./publish-panel";
import { TemplatePicker } from "./template-picker";

export type BuilderEvent = { id: string; slug: string; name: { ro: string; en: string } };
export type BuilderBrand = { id: string; name: string; slug: string; currency: "RON" | "EUR"; languages: Locale[] };

type SaveState = "saved" | "dirty" | "saving" | "conflict" | "error";
const AUTOSAVE_MS = 800;
const HISTORY_MAX = 100;
const FRAME_MIN_HEIGHT = 520;

export type BuilderProps = {
  storefrontId: string;
  brand: BuilderBrand;
  organiserName: string;
  isSample: boolean;
  hostnames: string[];
  initial: { revision: number; document: StorefrontDocument };
  live: { number: number; publishedAt: string } | null;
  versions: VersionView[];
  links: LinkView[];
  events: BuilderEvent[];
  canDesign: boolean;
  /** The platform's origin, for the absolute shop address. */
  origin: string;
};

export type Tab = "pages" | "brand" | "checkout" | "messages" | "publish";

export function BuilderClient(props: BuilderProps) {
  const t = useTranslations("builder");
  const [doc, setDocState] = useState(props.initial.document);
  const [revision, setRevision] = useState(props.initial.revision);
  const [save, setSave] = useState<{ state: SaveState; code?: string; path?: string }>({ state: "saved" });
  const [past, setPast] = useState<StorefrontDocument[]>([]);
  const [future, setFuture] = useState<StorefrontDocument[]>([]);
  const [pageId, setPageId] = useState(props.initial.document.pages[0].id);
  const [selected, setSelected] = useState<string | null>(null);
  const [device, setDevice] = useState<"desktop" | "phone">("desktop");
  // The preview starts in the Studio language when the shop has it, and follows the language of the
  // field being edited (focus on an RO or EN input switches it).
  const uiLocale = useLocale() as Locale;
  const [locale, setLocale] = useState<Locale>(props.initial.document.locales.enabled.includes(uiLocale) ? uiLocale : props.initial.document.locales.default);
  const [scheme, setScheme] = useState<"light" | "dark" | null>(null);
  const [tab, setTab] = useState<Tab>("pages");
  const [pane, setPane] = useState<"structure" | "preview" | "settings">("preview");
  const [live, setLive] = useState(props.live);
  const [versions, setVersions] = useState(props.versions);
  const [rendered, setRendered] = useState(-1);

  const frameRef = useRef<HTMLIFrameElement>(null);
  const docRef = useRef(doc);
  const revRef = useRef(revision);
  const seq = useRef(0);
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const inFlight = useRef<Promise<void> | null>(null);
  const dirty = useRef(false);
  const viewRef = useRef({ pageId, locale, scheme, selected });
  useEffect(() => {
    viewRef.current = { pageId, locale, scheme, selected };
  }, [pageId, locale, scheme, selected]);

  const knownEvents = useMemo(() => new Set(props.events.map((e) => e.id)), [props.events]);
  const problems = useMemo(() => publishProblems(doc, { knownEvents }), [doc, knownEvents]);

  // ---------------------------------------------------------------- preview frame
  const raf = useRef<number | null>(null);
  const postToFrame = useCallback(() => {
    if (raf.current !== null) return;
    raf.current = requestAnimationFrame(() => {
      raf.current = null;
      const w = frameRef.current?.contentWindow;
      if (!w) return;
      seq.current += 1;
      const v = viewRef.current;
      w.postMessage({ type: "eps:draft", seq: seq.current, document: docRef.current, pageId: v.pageId, locale: v.locale, scheme: v.scheme, selected: v.selected }, window.location.origin);
    });
  }, []);

  useEffect(() => postToFrame(), [doc, pageId, locale, scheme, selected, postToFrame]);

  useEffect(() => {
    const onMessage = (ev: MessageEvent) => {
      const expected = { origin: window.location.origin, frame: frameRef.current?.contentWindow };
      const ready = acceptFromFrame(ev, expected, readyMessage);
      if (ready) return postToFrame();
      const r = acceptFromFrame(ev, expected, renderedMessage);
      if (r) return setRendered(r.seq);
      const sel = acceptFromFrame(ev, expected, selectMessage);
      if (sel && docRef.current.pages.some((p) => p.id === sel.pageId)) {
        setPageId(sel.pageId);
        setSelected(sel.sectionId);
        setTab("pages");
        setPane("settings");
      }
    };
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, [postToFrame]);

  // ---------------------------------------------------------------- autosave
  // `generation` changes whenever the draft is replaced from the server (reload, restore, template
  // change): a save response from an older generation is dropped, so it can never overwrite the
  // adopted revision. After a 409 the draft stays dirty and no save is sent until the user reloads.
  const generation = useRef(0);
  const conflicted = useRef(false);
  type SaveResult = { ok: boolean; code?: string };
  const saveOnce = useCallback(async (): Promise<SaveResult> => {
    const gen = generation.current;
    dirty.current = false;
    const body = JSON.stringify({ revision: revRef.current, document: docRef.current });
    setSave({ state: "saving" });
    let result: SaveResult = { ok: false, code: "unexpected" };
    inFlight.current = (async () => {
      try {
        const res = await fetch(`/api/studio/storefronts/${props.storefrontId}/draft`, { method: "PUT", headers: { "content-type": "application/json" }, body });
        const j = (await res.json().catch(() => ({}))) as { revision?: number; code?: string; path?: string };
        if (gen !== generation.current) {
          result = { ok: false, code: "stale" };
          return; // the draft was replaced meanwhile: ignore this answer
        }
        if (res.ok && j.revision) {
          revRef.current = j.revision;
          setRevision(j.revision);
          result = { ok: true };
          setSave(dirty.current ? { state: "dirty" } : { state: "saved" });
        } else if (res.status === 409) {
          dirty.current = true;
          conflicted.current = true;
          result = { ok: false, code: "draft_conflict" };
          setSave({ state: "conflict" });
        } else {
          dirty.current = true;
          result = { ok: false, code: j.code ?? "unexpected" };
          setSave({ state: "error", code: j.code ?? "unexpected", path: j.path });
        }
      } catch {
        if (gen === generation.current) {
          dirty.current = true;
          result = { ok: false, code: "network" };
          setSave({ state: "error", code: "network" });
        }
      } finally {
        inFlight.current = null;
      }
    })();
    await inFlight.current;
    return result;
  }, [props.storefrontId]);

  /** Saves now (waiting for a save in flight); ok when the server holds the current draft. */
  const flush = useCallback(async (): Promise<SaveResult> => {
    if (saveTimer.current) {
      clearTimeout(saveTimer.current);
      saveTimer.current = null;
    }
    for (let i = 0; i < 5; i++) {
      while (inFlight.current) await inFlight.current;
      if (conflicted.current) return { ok: false, code: "draft_conflict" };
      if (!dirty.current) return { ok: true };
      const r = await saveOnce();
      if (!r.ok) return r;
    }
    return dirty.current ? { ok: false, code: "unexpected" } : { ok: true };
  }, [saveOnce]);

  const schedule = useCallback(() => {
    dirty.current = true;
    setSave((s) => (s.state === "conflict" ? s : { state: "dirty" }));
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => {
      saveTimer.current = null;
      void flush();
    }, AUTOSAVE_MS);
  }, [flush]);

  const setDoc = useCallback(
    (next: StorefrontDocument | ((d: StorefrontDocument) => StorefrontDocument)) => {
      if (!props.canDesign) return;
      const cur = docRef.current;
      const n = typeof next === "function" ? next(cur) : next;
      if (n === cur) return;
      docRef.current = n;
      setPast((p) => [...p.slice(-HISTORY_MAX + 1), cur]);
      setFuture([]);
      setDocState(n);
      schedule();
    },
    [props.canDesign, schedule],
  );

  const undo = () => {
    if (!past.length) return;
    const prev = past[past.length - 1];
    setPast(past.slice(0, -1));
    setFuture([doc, ...future]);
    docRef.current = prev;
    setDocState(prev);
    schedule();
  };
  const redo = () => {
    if (!future.length) return;
    const [next, ...rest] = future;
    setFuture(rest);
    setPast([...past, doc]);
    docRef.current = next;
    setDocState(next);
    schedule();
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const el = e.target as HTMLElement;
      if (el.closest("input, textarea, select")) return;
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "z") {
        e.preventDefault();
        if (e.shiftKey) redo();
        else undo();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  useEffect(() => {
    const warn = (e: BeforeUnloadEvent) => {
      if (dirty.current || inFlight.current) e.preventDefault();
    };
    window.addEventListener("beforeunload", warn);
    return () => window.removeEventListener("beforeunload", warn);
  }, []);

  /** Starts a new generation: pending autosaves are cancelled and answers in flight are ignored. */
  const newGeneration = () => {
    generation.current += 1;
    if (saveTimer.current) {
      clearTimeout(saveTimer.current);
      saveTimer.current = null;
    }
    conflicted.current = false;
    dirty.current = false;
  };

  const reload = async () => {
    const res = await fetch(`/api/studio/storefronts/${props.storefrontId}/draft`, { cache: "no-store" });
    if (!res.ok) return;
    const j = (await res.json()) as { revision: number; document: StorefrontDocument };
    newGeneration();
    revRef.current = j.revision;
    setRevision(j.revision);
    docRef.current = j.document;
    setDocState(j.document);
    setPast([]);
    setFuture([]);
    if (!j.document.pages.some((p) => p.id === pageId)) setPageId(j.document.pages[0].id);
    setSelected(null);
    setSave({ state: "saved" });
  };

  /** Replaces the draft with a server result (restore, template change) without saving again. */
  const adopt = (next: { revision: number; document: StorefrontDocument }) => {
    newGeneration();
    revRef.current = next.revision;
    setRevision(next.revision);
    setPast((p) => [...p, docRef.current]);
    setFuture([]);
    docRef.current = next.document;
    setDocState(next.document);
    if (!next.document.pages.some((p) => p.id === pageId)) setPageId(next.document.pages[0].id);
    setSelected(null);
    setSave({ state: "saved" });
  };

  const enabledLocales = doc.locales.enabled.join(",");
  useEffect(() => {
    const onFocus = (e: FocusEvent) => {
      const l = (e.target as HTMLElement | null)?.closest?.("[data-locale]")?.getAttribute("data-locale");
      if ((l === "ro" || l === "en") && enabledLocales.split(",").includes(l)) setLocale(l);
    };
    document.addEventListener("focusin", onFocus);
    return () => document.removeEventListener("focusin", onFocus);
  }, [enabledLocales]);

  // The frame keeps its real width (1280 or 390 px) and is scaled down to fit the pane.
  const stageRef = useRef<HTMLDivElement>(null);
  const [stageWidth, setStageWidth] = useState(0);
  useEffect(() => {
    const el = stageRef.current;
    if (!el) return;
    const ro = new ResizeObserver(([e]) => setStageWidth(e.contentRect.width));
    ro.observe(el);
    return () => ro.disconnect();
  }, []);
  const frameWidth = device === "desktop" ? 1280 : 390;
  const scale = stageWidth > 0 ? Math.min(1, stageWidth / frameWidth) : 1;
  // The stage fills the viewport below the header; the frame's own height is that divided by the scale.
  const [viewport, setViewport] = useState(900);
  useEffect(() => {
    const on = () => setViewport(window.innerHeight);
    on();
    window.addEventListener("resize", on);
    return () => window.removeEventListener("resize", on);
  }, []);
  const stageHeight = Math.max(FRAME_MIN_HEIGHT, viewport - 140);
  const FRAME_HEIGHT = Math.round(stageHeight / scale);

  const page = doc.pages.find((p) => p.id === pageId) ?? doc.pages[0];
  const address = `/s/${props.brand.slug}`;
  const absoluteAddress = `${props.origin}${address}`;
  const tabs: [Tab, string][] = [
    ["pages", t("tabs.pages")],
    ["brand", t("tabs.brand")],
    ["checkout", t("tabs.checkout")],
    ["messages", t("tabs.messages")],
    ["publish", t("tabs.publish")],
  ];

  const left = (
    <div className="space-y-3">
      <div role="tablist" aria-label={t("tabs.label")} className="flex flex-wrap gap-1">
        {tabs.map(([k, label]) => (
          <button
            key={k}
            role="tab"
            type="button"
            aria-selected={tab === k}
            data-testid={`tab-${k}`}
            onClick={() => {
              setTab(k);
              if (k !== "pages") setPane("settings");
            }}
            className={`rounded-md border px-2 py-1 text-sm ${tab === k ? "bg-primary text-primary-foreground" : "bg-background"}`}
          >
            {label}
            {k === "publish" && problems.length > 0 && <span className="ml-1 rounded-full bg-destructive/15 px-1.5 text-xs text-destructive">{problems.length}</span>}
          </button>
        ))}
      </div>
      {tab === "pages" && (
        <TreePanel
          doc={doc}
          page={page}
          selected={selected}
          events={props.events}
          readOnly={!props.canDesign}
          onSelectPage={(id) => {
            setPageId(id);
            setSelected(null);
          }}
          onSelect={(id) => {
            setSelected(id);
            setPane("settings");
          }}
          setDoc={setDoc}
        />
      )}
    </div>
  );

  const right =
    tab === "pages" ? (
      <SettingsPanel doc={doc} page={page} selected={selected} events={props.events} setDoc={setDoc} readOnly={!props.canDesign} saveError={save.state === "error" ? save : null} />
    ) : tab === "brand" ? (
      <BrandKitPanel doc={doc} setDoc={setDoc} readOnly={!props.canDesign} />
    ) : tab === "checkout" ? (
      <CheckoutPanel doc={doc} brandName={props.brand.name} locale={locale} />
    ) : tab === "messages" ? (
      <MessagesPanel doc={doc} setDoc={setDoc} brand={props.brand} locale={locale} readOnly={!props.canDesign} />
    ) : (
      <PublishPanel
        storefrontId={props.storefrontId}
        problems={problems}
        doc={doc}
        getRevision={() => revRef.current}
        flush={flush}
        saving={save.state !== "saved"}
        canDesign={props.canDesign}
        live={live}
        versions={versions}
        initialLinks={props.links}
        onGoTo={(p) => {
          if (p.pageId) setPageId(p.pageId);
          setSelected(p.sectionId ?? null);
          setTab(p.path.startsWith("tokens") ? "brand" : p.path.startsWith("messages") ? "messages" : "pages");
          setPane("settings");
        }}
        onPublished={(r) => {
          setLive({ number: r.live ?? r.number, publishedAt: new Date().toISOString() });
          setVersions(r.versions);
        }}
        onRestored={(r) => {
          adopt({ revision: r.revision, document: r.document });
          setLive({ number: r.live ?? r.number, publishedAt: new Date().toISOString() });
          setVersions(r.versions);
        }}
      />
    );

  return (
    <div className="space-y-3" data-testid="builder" data-revision={revision} data-rendered={rendered} data-save={save.state}>
      <header className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3">
        <div className="min-w-0 space-y-1">
          <h1 className="truncate text-lg font-bold">{t("title", { brand: props.brand.name })}</h1>
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="text-muted-foreground">{t("address")}</span>
            <code data-testid="brand-address">{address}</code>
            <CopyButton value={absoluteAddress} />
            <span
              data-testid="address-state"
              className={`rounded-full px-2 py-0.5 text-xs font-semibold ${live ? "bg-emerald-100 text-emerald-900 dark:bg-emerald-900/40 dark:text-emerald-100" : "bg-amber-100 text-amber-900 dark:bg-amber-900/40 dark:text-amber-100"}`}
            >
              {live ? t("live", { n: live.number }) : t("notPublished")}
            </span>
            {live && (
              <span className="text-xs text-muted-foreground" data-testid="address-note">
                {t("publicShopSoon")}
              </span>
            )}
            {props.hostnames.map((h) => (
              <code key={h} className="text-xs text-muted-foreground" data-testid="brand-hostname">
                {h}
              </code>
            ))}
            {props.isSample && <span className="rounded-full border border-amber-500/60 bg-amber-100 px-2 py-0.5 text-xs font-semibold text-amber-900">{t("sample")}</span>}
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 text-sm">
          <span role="status" data-testid="save-state" className={save.state === "error" || save.state === "conflict" ? "text-destructive" : "text-muted-foreground"}>
            {t(`save.${save.state}`)}
          </span>
          <Button type="button" size="sm" variant="outline" onClick={undo} disabled={!past.length} data-testid="undo">
            {t("undo")}
          </Button>
          <Button type="button" size="sm" variant="outline" onClick={redo} disabled={!future.length} data-testid="redo">
            {t("redo")}
          </Button>
          {props.canDesign && <TemplatePicker storefrontId={props.storefrontId} getRevision={() => revRef.current} flush={flush} onApplied={adopt} />}
        </div>
      </header>

      {save.state === "conflict" && (
        <div role="alert" data-testid="conflict" className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-destructive/50 p-3 text-sm">
          <span>{t("conflict")}</span>
          <Button type="button" size="sm" onClick={reload} data-testid="conflict-reload">
            {t("reload")}
          </Button>
        </div>
      )}
      {!props.canDesign && <p className="rounded-md border p-2 text-sm text-muted-foreground">{t("readOnly")}</p>}

      <div role="tablist" aria-label={t("panes.label")} className="flex gap-1 lg:hidden">
        {(["structure", "preview", "settings"] as const).map((k) => (
          <button key={k} type="button" role="tab" aria-selected={pane === k} data-testid={`pane-${k}`} onClick={() => setPane(k)} className={`flex-1 rounded-md border px-2 py-1 text-sm ${pane === k ? "bg-primary text-primary-foreground" : ""}`}>
            {t(`panes.${k}`)}
          </button>
        ))}
      </div>

      <div className="grid items-start gap-3 lg:grid-cols-[16rem_minmax(0,1fr)_18rem]">
        <aside className={`min-w-0 rounded-lg border p-3 ${pane === "structure" ? "" : "hidden"} lg:block`} data-testid="pane-structure-body">
          {left}
        </aside>
        <section className={`min-w-0 space-y-2 ${pane === "preview" ? "" : "hidden"} lg:sticky lg:top-2 lg:block`} aria-label={t("panes.preview")} data-testid="preview-stage">
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <div className="flex overflow-hidden rounded-md border" role="group" aria-label={t("device.label")}>
              {(["desktop", "phone"] as const).map((d) => (
                <button key={d} type="button" aria-pressed={device === d} data-testid={`device-${d}`} onClick={() => setDevice(d)} className={`px-3 py-1 ${device === d ? "bg-primary text-primary-foreground" : ""}`}>
                  {t(`device.${d}`)}
                </button>
              ))}
            </div>
            <label className="flex items-center gap-1">
              <span>{t("previewLanguage")}</span>
              <select data-testid="preview-locale" value={locale} onChange={(e) => setLocale(e.target.value as Locale)} className="rounded-md border bg-background px-2 py-1">
                {doc.locales.enabled.map((l) => (
                  <option key={l} value={l}>
                    {l.toUpperCase()}
                  </option>
                ))}
              </select>
            </label>
            {doc.tokens.mode === "auto" && (
              <label className="flex items-center gap-1">
                <span>{t("previewScheme")}</span>
                <select data-testid="preview-scheme" value={scheme ?? "light"} onChange={(e) => setScheme(e.target.value as "light" | "dark")} className="rounded-md border bg-background px-2 py-1">
                  <option value="light">{t("scheme.light")}</option>
                  <option value="dark">{t("scheme.dark")}</option>
                </select>
              </label>
            )}
            <span className="text-xs text-muted-foreground">{t("pageShown", { page: page.title[locale] || page.slug || "/" })}</span>
          </div>
          <div ref={stageRef} className="overflow-hidden rounded-lg border bg-muted/40 p-2">
            <div style={{ width: frameWidth * scale, height: FRAME_HEIGHT * scale }} className="mx-auto">
              <iframe
                ref={frameRef}
                title={t("frameTitle")}
                src={`/studio/builder/frame/${props.storefrontId}?page=${props.initial.document.pages[0].id}`}
                data-testid="preview-frame"
                data-device={device}
                className="block origin-top-left rounded-md bg-white shadow-sm ring-1 ring-border"
                style={{ width: frameWidth, height: FRAME_HEIGHT, transform: scale < 1 ? `scale(${scale})` : undefined }}
                onLoad={() => postToFrame()}
              />
            </div>
          </div>
        </section>
        <aside className={`min-w-0 rounded-lg border p-3 ${pane === "settings" ? "" : "hidden"} lg:block`} data-testid="pane-settings-body">
          {right}
        </aside>
      </div>
    </div>
  );
}
