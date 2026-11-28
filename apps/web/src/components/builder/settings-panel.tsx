"use client";
// The settings form of the selected section (or of the page), generated from the section's schema
// (design D2.2): each field descriptor picks a widget. Blocks get the same generated forms.
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Hint, Input, Select, Textarea } from "@/components/ui/field";
import { addBlock, canAddBlock, moveBlock, removeBlock, setBlockSetting, setPage, setSectionSetting } from "@/lib/storefront/edit";
import { describeFields, type FieldDescriptor } from "@/lib/storefront/fields";
import { isSafeLink } from "@/lib/storefront/links";
import { sectionDef } from "@/lib/storefront/sections";
import type { Page, Section, StorefrontDocument } from "@/lib/storefront/schema";
import type { BuilderEvent } from "./builder-client";
import { ImagePicker } from "./image-picker";
import { sectionLabel } from "./tree-panel";

type SetDoc = (fn: (d: StorefrontDocument) => StorefrontDocument) => void;

export function LocalizedInput({
  value,
  onChange,
  locales,
  label,
  multiline,
  max,
  disabled,
  testId,
}: {
  value: { ro: string; en: string };
  onChange: (v: { ro: string; en: string }) => void;
  locales: ("ro" | "en")[];
  label: string;
  multiline?: boolean;
  max?: number;
  disabled?: boolean;
  testId?: string;
}) {
  return (
    <fieldset className="space-y-1" data-testid={testId}>
      <legend className="text-sm font-medium">{label}</legend>
      {locales.map((l) => {
        const common = {
          value: value?.[l] ?? "",
          maxLength: max,
          disabled,
          "aria-label": `${label} (${l.toUpperCase()})`,
          "data-locale": l,
          onChange: (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => onChange({ ...{ ro: "", en: "" }, ...value, [l]: e.target.value }),
        };
        return (
          <div key={l} className="flex items-start gap-1">
            <span className="mt-1.5 w-6 shrink-0 text-xs font-semibold text-muted-foreground">{l.toUpperCase()}</span>
            {multiline ? <Textarea {...common} rows={4} /> : <Input {...common} />}
          </div>
        );
      })}
    </fieldset>
  );
}

function FieldInput({
  f,
  value,
  onChange,
  locales,
  events,
  disabled,
  idPrefix,
}: {
  f: FieldDescriptor;
  value: unknown;
  onChange: (v: unknown) => void;
  locales: ("ro" | "en")[];
  events: BuilderEvent[];
  disabled: boolean;
  idPrefix: string;
}) {
  const t = useTranslations("builder");
  const label = t.has(`fields.${f.key}`) ? t(`fields.${f.key}`) : f.key;
  const id = `${idPrefix}-${f.key}`;
  switch (f.kind) {
    case "localized":
    case "markdown":
      return (
        <div className="space-y-1">
          <LocalizedInput value={value as { ro: string; en: string }} onChange={onChange} locales={locales} label={label} multiline={f.kind === "markdown"} max={f.max} disabled={disabled} testId={`field-${f.key}`} />
          {f.kind === "markdown" && <Hint>{t("markdownHint")}</Hint>}
        </div>
      );
    case "enum":
      return (
        <label className="grid gap-1 text-sm font-medium" htmlFor={id}>
          {label}
          <Select id={id} value={String(value)} disabled={disabled} onChange={(e) => onChange(e.target.value)} data-testid={`field-${f.key}`}>
            {f.options!.map((o) => (
              <option key={o} value={o}>
                {t.has(`options.${o}`) ? t(`options.${o}`) : o}
              </option>
            ))}
          </Select>
        </label>
      );
    case "bool":
      return (
        <label className="inline-flex items-center gap-2 text-sm">
          <input type="checkbox" className="size-4" checked={Boolean(value)} disabled={disabled} onChange={(e) => onChange(e.target.checked)} data-testid={`field-${f.key}`} />
          {label}
        </label>
      );
    case "int":
      return (
        <label className="grid gap-1 text-sm font-medium" htmlFor={id}>
          {label}
          <Input
            id={id}
            type="number"
            min={f.min}
            max={f.max}
            value={Number(value)}
            disabled={disabled}
            onChange={(e) => {
              const n = Math.round(Number(e.target.value));
              if (Number.isFinite(n)) onChange(Math.min(f.max ?? n, Math.max(f.min ?? n, n)));
            }}
          />
        </label>
      );
    case "link": {
      const v = String(value ?? "");
      const bad = v !== "" && !isSafeLink(v);
      return (
        <label className="grid gap-1 text-sm font-medium" htmlFor={id}>
          {label}
          <Input id={id} value={v} disabled={disabled} aria-invalid={bad || undefined} onChange={(e) => onChange(e.target.value.trim())} placeholder="/e/… · https://… · mailto:…" data-testid={`field-${f.key}`} />
          {bad && (
            <span role="alert" className="text-xs text-destructive" data-testid="link-error">
              {t("unsafeLink")}
            </span>
          )}
        </label>
      );
    }
    case "image":
      return <ImagePicker label={label} value={String(value ?? "")} onChange={onChange} disabled={disabled} testId={`field-${f.key}`} />;
    case "event":
      return (
        <label className="grid gap-1 text-sm font-medium" htmlFor={id}>
          {label}
          <Select id={id} value={String(value ?? "")} disabled={disabled} onChange={(e) => onChange(e.target.value)} data-testid={`field-${f.key}`}>
            <option value="">{t("noEvent")}</option>
            {events.map((ev) => (
              <option key={ev.id} value={ev.id}>
                {ev.name.en || ev.name.ro || ev.slug}
              </option>
            ))}
          </Select>
        </label>
      );
    case "datetime":
      return (
        <label className="grid gap-1 text-sm font-medium" htmlFor={id}>
          {label}
          <Input
            id={id}
            type="datetime-local"
            disabled={disabled}
            value={value ? toLocalInput(String(value)) : ""}
            onChange={(e) => onChange(e.target.value ? new Date(e.target.value).toISOString() : "")}
          />
        </label>
      );
  }
}

function toLocalInput(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

export function SettingsPanel({
  doc,
  page,
  selected,
  events,
  setDoc,
  readOnly,
  saveError,
}: {
  doc: StorefrontDocument;
  page: Page;
  selected: string | null;
  events: BuilderEvent[];
  setDoc: SetDoc;
  readOnly: boolean;
  saveError: { code?: string; path?: string } | null;
}) {
  const t = useTranslations("builder");
  const te = useTranslations("errors");
  const locales = doc.locales.enabled;
  const section = (page.sections as Section[]).find((s) => s.id === selected) ?? null;
  const def = section ? sectionDef(section.type) : null;

  const errorBox = saveError && (
    <p role="alert" className="rounded-md border border-destructive/50 p-2 text-sm text-destructive" data-testid="save-error">
      {te.has(saveError.code ?? "") ? te(saveError.code!) : te("invalid")}
      {saveError.path ? <code className="ml-1 text-xs">{saveError.path}</code> : null}
    </p>
  );

  if (!section || !def) {
    return (
      <div className="space-y-3" data-testid="page-form">
        {errorBox}
        <h2 className="font-semibold">{t("pageSettings")}</h2>
        <LocalizedInput value={page.title} locales={locales} label={t("fields.title")} max={120} disabled={readOnly} onChange={(v) => setDoc((d) => setPage(d, page.id, { title: v }))} testId="field-title" />
        {page.kind === "info" && (
          <label className="grid gap-1 text-sm font-medium">
            {t("fields.slug")}
            <Input value={page.slug} disabled={readOnly} onChange={(e) => setDoc((d) => setPage(d, page.id, { slug: e.target.value.toLowerCase().replace(/[^a-z0-9-]/g, "-").slice(0, 60) }))} />
          </label>
        )}
        {page.kind === "event" && (
          <label className="grid gap-1 text-sm font-medium">
            {t("fields.eventId")}
            <Select value={page.eventId ?? ""} disabled={readOnly} onChange={(e) => setDoc((d) => setPage(d, page.id, { eventId: e.target.value || null }))}>
              <option value="">{t("noEvent")}</option>
              {events.map((ev) => (
                <option key={ev.id} value={ev.id}>
                  {ev.name.en || ev.name.ro || ev.slug}
                </option>
              ))}
            </Select>
          </label>
        )}
        <Hint>{t("selectHint")}</Hint>
      </div>
    );
  }

  const blockTypes = Object.keys(def.blocks);
  return (
    <div className="space-y-4" data-testid="section-form" data-section-type={section.type}>
      {errorBox}
      <h2 className="font-semibold">{sectionLabel(section, t)}</h2>
      {def.locked && <Hint>{t("footerLocked")}</Hint>}
      {describeFields(def.settings).map((f) => (
        <FieldInput
          key={f.key}
          f={f}
          idPrefix={section.id}
          value={section.settings[f.key]}
          locales={locales}
          events={events}
          disabled={readOnly}
          onChange={(v) => setDoc((d) => setSectionSetting(d, page.id, section.id, f.key, v))}
        />
      ))}
      {blockTypes.length > 0 && (
        <div className="space-y-2">
          <h3 className="text-sm font-semibold">{t("blocks")}</h3>
          <ol className="space-y-2">
            {section.blocks.map((b, bi) => {
              const bdef = def.blocks[b.type];
              return (
                <li key={b.id} className="space-y-2 rounded-md border p-2" data-testid="block-row">
                  <div className="flex items-center justify-between gap-1">
                    <span className="text-xs font-semibold text-muted-foreground">
                      {t(`blockTypes.${b.type}`)} {bi + 1}
                    </span>
                    {!readOnly && (
                      <span className="flex gap-1">
                        <Button type="button" size="icon-xs" variant="ghost" aria-label={t("blockUp")} disabled={bi === 0} onClick={() => setDoc((d) => moveBlock(d, page.id, section.id, bi, bi - 1))}>
                          ↑
                        </Button>
                        <Button type="button" size="icon-xs" variant="ghost" aria-label={t("blockDown")} disabled={bi === section.blocks.length - 1} onClick={() => setDoc((d) => moveBlock(d, page.id, section.id, bi, bi + 1))}>
                          ↓
                        </Button>
                        <Button type="button" size="icon-xs" variant="ghost" aria-label={t("blockRemove")} onClick={() => setDoc((d) => removeBlock(d, page.id, section.id, b.id))}>
                          ✕
                        </Button>
                      </span>
                    )}
                  </div>
                  {bdef &&
                    describeFields(bdef.settings).map((f) => (
                      <FieldInput
                        key={f.key}
                        f={f}
                        idPrefix={b.id}
                        value={b.settings[f.key]}
                        locales={locales}
                        events={events}
                        disabled={readOnly}
                        onChange={(v) => setDoc((d) => setBlockSetting(d, page.id, section.id, b.id, f.key, v))}
                      />
                    ))}
                </li>
              );
            })}
          </ol>
          {!readOnly &&
            blockTypes.map((bt) => (
              <Button key={bt} type="button" size="sm" variant="outline" disabled={!canAddBlock(section)} onClick={() => setDoc((d) => addBlock(d, page.id, section.id, bt))} data-testid="add-block">
                {t("addBlock", { type: t(`blockTypes.${bt}`) })}
              </Button>
            ))}
          {!canAddBlock(section) && <Hint>{t("blockLimit", { max: def.maxBlocks })}</Hint>}
        </div>
      )}
    </div>
  );
}
