"use client";
// The preview frame's client (design D2.4): accepts eps:draft messages only from its parent on the
// same origin, newer than the last one and valid against the document schema; renders them; answers
// eps:rendered; posts eps:select when a section is clicked. Links never navigate inside the frame.
import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { frameCatalogueAction } from "@/app/studio/builder/actions";
import { acceptDraft } from "@/lib/storefront/frame-protocol";
import type { ShopBrand, ShopCatalogue } from "@/lib/storefront/catalogue";
import { SHOP_LABELS } from "@/lib/storefront/labels";
import { referencedEvents, type Locale, type StorefrontDocument } from "@/lib/storefront/schema";
import { StorefrontView } from "@/components/storefront/storefront";
import { shopFontClass } from "@/components/storefront/fonts";

const CATALOGUE_REFRESH_MS = 2000;

export function FrameClient(props: {
  storefrontId: string;
  initialDoc: StorefrontDocument;
  initialPageId: string;
  initialLocale: Locale;
  initialCatalogue: ShopCatalogue;
  brand: ShopBrand;
}) {
  const [state, setState] = useState({ doc: props.initialDoc, pageId: props.initialPageId, locale: props.initialLocale, scheme: null as "light" | "dark" | null, selected: null as string | null });
  const [catalogue, setCatalogue] = useState(props.initialCatalogue);
  // Rendered only in the browser: dates and prices format the same way as the builder shows them.
  const mounted = useSyncExternalStore(
    () => () => {},
    () => true,
    () => false,
  );
  const lastSeq = useRef(-1);
  const requestTag = useRef(0);
  const lastFetch = useRef({ at: 0, key: Object.keys(props.initialCatalogue).sort().join(",") });

  const refreshCatalogue = useCallback(
    async (doc: StorefrontDocument) => {
      const ids = referencedEvents(doc).sort();
      const key = ids.join(",");
      const now = Date.now();
      if (key === lastFetch.current.key && now - lastFetch.current.at < CATALOGUE_REFRESH_MS) return;
      lastFetch.current = { at: now, key };
      // Answers can arrive out of order: apply only the newest request's answer.
      const tag = ++requestTag.current;
      const r = await frameCatalogueAction(props.storefrontId, ids);
      if (r.ok && tag === requestTag.current) setCatalogue(r.data);
    },
    [props.storefrontId],
  );

  useEffect(() => {
    if (window.parent === window) return;
    const onMessage = (ev: MessageEvent) => {
      const d = acceptDraft({ origin: ev.origin, source: ev.source, data: ev.data }, { origin: window.location.origin, parent: window.parent, lastSeq: lastSeq.current });
      if (!d.accept) {
        if (d.reason === "document" && typeof (ev.data as { seq?: unknown })?.seq === "number" && ev.source === window.parent && ev.origin === window.location.origin) {
          window.parent.postMessage({ type: "eps:rendered", seq: (ev.data as { seq: number }).seq, ok: false }, window.location.origin);
        }
        return;
      }
      const m = d.message;
      lastSeq.current = m.seq;
      setState({ doc: m.document, pageId: m.pageId, locale: m.locale, scheme: m.scheme, selected: m.selected });
      void refreshCatalogue(m.document);
      requestAnimationFrame(() => window.parent.postMessage({ type: "eps:rendered", seq: m.seq, ok: true }, window.location.origin));
    };
    window.addEventListener("message", onMessage);
    window.parent.postMessage({ type: "eps:ready" }, window.location.origin);
    return () => window.removeEventListener("message", onMessage);
  }, [refreshCatalogue]);

  const page = state.doc.pages.find((p) => p.id === state.pageId) ?? state.doc.pages[0];

  const onClick = (e: React.MouseEvent) => {
    const target = e.target as HTMLElement;
    const link = target.closest("a");
    if (link) {
      e.preventDefault();
      const pageId = link.getAttribute("data-eps-page");
      if (pageId && window.parent !== window) {
        window.parent.postMessage({ type: "eps:select", pageId, sectionId: null }, window.location.origin);
        return;
      }
    }
    const section = target.closest("[data-eps-section]");
    if (section && window.parent !== window) {
      window.parent.postMessage({ type: "eps:select", pageId: page.id, sectionId: section.getAttribute("data-eps-section") }, window.location.origin);
    }
  };

  if (!mounted) return <div className="min-h-screen" data-testid="frame-loading" />;
  return (
    <div onClickCapture={onClick} data-testid="frame-root" data-page={page.id}>
      <StorefrontView
        doc={state.doc}
        page={page}
        fontClass={shopFontClass}
        scheme={state.scheme}
        selectedSectionId={state.selected}
        ctx={{ locale: state.locale, labels: SHOP_LABELS[state.locale], catalogue, brand: props.brand, mode: "preview", hrefBase: "" }}
      />
    </div>
  );
}
