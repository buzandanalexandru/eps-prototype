"use client";
// Pages and their sections: select, add, remove, hide, and reorder by dragging or with the up and
// down buttons (keyboard users). The footer is locked last (design D2.2, D2.8).
import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { addPage, addSection, canAddSection, moveSection, removePage, removeSection, toggleHidden } from "@/lib/storefront/edit";
import { ADDABLE_SECTIONS, type SectionType } from "@/lib/storefront/sections";
import type { Page, Section, StorefrontDocument } from "@/lib/storefront/schema";
import type { BuilderEvent } from "./builder-client";

type SetDoc = (fn: (d: StorefrontDocument) => StorefrontDocument) => void;

export function sectionLabel(s: Section, t: (k: string) => string, locale: "ro" | "en" = "en"): string {
  const h = (s.settings as { heading?: { ro: string; en: string } }).heading;
  const text = h?.[locale]?.trim() || h?.ro?.trim() || h?.en?.trim();
  return text ? `${t(`sections.${s.type}`)} · ${text}` : t(`sections.${s.type}`);
}

export function TreePanel({
  doc,
  page,
  selected,
  events,
  readOnly,
  onSelectPage,
  onSelect,
  setDoc,
}: {
  doc: StorefrontDocument;
  page: Page;
  selected: string | null;
  events: BuilderEvent[];
  readOnly: boolean;
  onSelectPage: (id: string) => void;
  onSelect: (id: string | null) => void;
  setDoc: SetDoc;
}) {
  const t = useTranslations("builder");
  const [addType, setAddType] = useState<SectionType>("text");
  const [drag, setDrag] = useState<number | null>(null);
  const sections = page.sections as Section[];
  const full = !canAddSection(page);

  return (
    <div className="space-y-4">
      <div className="space-y-2">
        <h2 className="text-sm font-semibold">{t("pages")}</h2>
        <ul className="space-y-1" data-testid="page-list">
          {doc.pages.map((p, i) => (
            <li key={p.id} className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => onSelectPage(p.id)}
                aria-current={p.id === page.id ? "page" : undefined}
                className={`min-w-0 flex-1 truncate rounded-md px-2 py-1 text-left text-sm ${p.id === page.id ? "bg-muted font-semibold" : "hover:bg-muted"}`}
              >
                {p.title.en || p.title.ro || p.slug} <span className="text-xs text-muted-foreground">{t(`pageKinds.${p.kind}`)}</span>
              </button>
              {i > 0 && !readOnly && (
                <Button
                  type="button"
                  size="xs"
                  variant="ghost"
                  aria-label={t("removePage", { page: p.title.en || p.slug })}
                  onClick={() => {
                    if (!window.confirm(t("removePageConfirm"))) return;
                    if (p.id === page.id) onSelectPage(doc.pages[0].id);
                    setDoc((d) => removePage(d, p.id));
                  }}
                >
                  ✕
                </Button>
              )}
            </li>
          ))}
        </ul>
        {!readOnly && (
          <div className="flex flex-wrap gap-1">
            <Button type="button" size="xs" variant="outline" data-testid="add-info-page" onClick={() => setDoc((d) => addPage(d, "info").doc)}>
              {t("addInfoPage")}
            </Button>
            {events[0] && (
              <Button type="button" size="xs" variant="outline" onClick={() => setDoc((d) => addPage(d, "event", { eventId: events[0].id, slug: events[0].slug }).doc)}>
                {t("addEventPage")}
              </Button>
            )}
          </div>
        )}
      </div>

      <div className="space-y-2">
        <div className="flex items-center justify-between gap-2">
          <h2 className="text-sm font-semibold">{t("sectionsTitle")}</h2>
          <Button type="button" size="xs" variant="ghost" onClick={() => onSelect(null)} data-testid="page-settings">
            {t("pageSettings")}
          </Button>
        </div>
        <ol className="space-y-1" data-testid="section-list">
          {sections.map((s, i) => {
            const locked = s.type === "footer";
            const label = sectionLabel(s, t);
            return (
              <li
                key={s.id}
                data-testid="section-row"
                data-section-type={s.type}
                data-section-id={s.id}
                data-hidden={s.hidden ? "true" : "false"}
                draggable={!locked && !readOnly}
                onDragStart={(e) => {
                  setDrag(i);
                  e.dataTransfer.effectAllowed = "move";
                  e.dataTransfer.setData("text/plain", s.id);
                }}
                onDragOver={(e) => {
                  if (drag !== null) e.preventDefault();
                }}
                onDrop={(e) => {
                  e.preventDefault();
                  if (drag !== null) setDoc((d) => moveSection(d, page.id, drag, i));
                  setDrag(null);
                }}
                onDragEnd={() => setDrag(null)}
                className={`flex items-center gap-1 rounded-md border px-1 py-1 text-sm ${selected === s.id ? "border-primary bg-muted" : ""} ${s.hidden ? "opacity-60" : ""}`}
              >
                {!locked && !readOnly && (
                  <span aria-hidden className="cursor-grab px-1 text-muted-foreground">
                    ⋮⋮
                  </span>
                )}
                <button type="button" onClick={() => onSelect(s.id)} className="min-w-0 flex-1 truncate text-left" data-testid="section-select">
                  {label}
                  {locked && <span className="ml-1 text-xs text-muted-foreground">({t("locked")})</span>}
                </button>
                {!locked && !readOnly && (
                  <>
                    <Button type="button" size="icon-xs" variant="ghost" aria-label={t("moveUp", { section: label })} disabled={i === 0} onClick={() => setDoc((d) => moveSection(d, page.id, i, i - 1))} data-testid="move-up">
                      ↑
                    </Button>
                    <Button
                      type="button"
                      size="icon-xs"
                      variant="ghost"
                      aria-label={t("moveDown", { section: label })}
                      disabled={i >= sections.length - 2}
                      onClick={() => setDoc((d) => moveSection(d, page.id, i, i + 1))}
                      data-testid="move-down"
                    >
                      ↓
                    </Button>
                    <Button type="button" size="icon-xs" variant="ghost" aria-pressed={s.hidden} aria-label={s.hidden ? t("show", { section: label }) : t("hide", { section: label })} onClick={() => setDoc((d) => toggleHidden(d, page.id, s.id))} data-testid="toggle-hidden">
                      {s.hidden ? "◌" : "◉"}
                    </Button>
                    <Button
                      type="button"
                      size="icon-xs"
                      variant="ghost"
                      aria-label={t("remove", { section: label })}
                      onClick={() => {
                        if (selected === s.id) onSelect(null);
                        setDoc((d) => removeSection(d, page.id, s.id));
                      }}
                      data-testid="remove-section"
                    >
                      ✕
                    </Button>
                  </>
                )}
              </li>
            );
          })}
        </ol>
        {!readOnly && (
          <div className="flex flex-wrap items-center gap-1">
            <label className="sr-only" htmlFor="add-section-type">
              {t("addSectionType")}
            </label>
            <select id="add-section-type" data-testid="add-section-type" value={addType} onChange={(e) => setAddType(e.target.value as SectionType)} className="min-w-0 flex-1 rounded-md border bg-background px-2 py-1 text-sm">
              {ADDABLE_SECTIONS.map((ty) => (
                <option key={ty} value={ty}>
                  {t(`sections.${ty}`)}
                </option>
              ))}
            </select>
            <Button
              type="button"
              size="sm"
              disabled={full}
              data-testid="add-section"
              onClick={() => {
                const r = addSection(doc, page.id, addType, { eventId: page.eventId ?? events[0]?.id });
                if (!r.id) return;
                setDoc(() => r.doc);
                onSelect(r.id);
              }}
            >
              {t("addSection")}
            </Button>
            {full && <p className="w-full text-xs text-muted-foreground">{t("sectionLimit")}</p>}
          </div>
        )}
      </div>
    </div>
  );
}
