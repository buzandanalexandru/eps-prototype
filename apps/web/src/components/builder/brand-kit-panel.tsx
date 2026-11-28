"use client";
// The brand kit (design D2.3; B1): logo, colours, fonts, radius and mode become the storefront's
// tokens. The contrast check runs on every change, shows each pair with its ratio, suggests a passing
// colour of the same hue and blocks publishing while a pair fails.
import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { Hint, Select } from "@/components/ui/field";
import { acceptSuggestion, checkContrast, paletteFromPixels } from "@/lib/storefront/contrast";
import { FONT_IDS, FONTS } from "@/lib/storefront/fonts";
import { MODES, PALETTE_KEYS, RADII, type Palette, type StorefrontDocument, type Tokens } from "@/lib/storefront/schema";
import { ImagePicker } from "./image-picker";
import { deleteAssetAction, listAssetsAction, type AssetView } from "@/app/studio/builder/actions";

type SetDoc = (fn: (d: StorefrontDocument) => StorefrontDocument) => void;

async function pixelsOf(file: File): Promise<Uint8ClampedArray | null> {
  try {
    const bmp = await createImageBitmap(file);
    const w = Math.min(160, bmp.width);
    const h = Math.max(1, Math.round((bmp.height / bmp.width) * w));
    const canvas = document.createElement("canvas");
    canvas.width = w;
    canvas.height = h;
    const ctx = canvas.getContext("2d");
    if (!ctx) return null;
    ctx.drawImage(bmp, 0, 0, w, h);
    return ctx.getImageData(0, 0, w, h).data;
  } catch {
    return null;
  }
}

function Swatch({ colour }: { colour: string }) {
  return <span aria-hidden className="inline-block size-4 shrink-0 rounded border align-middle" style={{ background: colour }} />;
}

function PaletteEditor({ scheme, palette, onChange, disabled }: { scheme: "light" | "dark"; palette: Palette; onChange: (k: keyof Palette, v: string) => void; disabled: boolean }) {
  const t = useTranslations("builder");
  return (
    <fieldset className="space-y-2" data-testid={`palette-${scheme}`}>
      <legend className="text-sm font-semibold">{t(`scheme.${scheme}`)}</legend>
      <div className="grid grid-cols-1 gap-2">
        {PALETTE_KEYS.map((k) => (
          <label key={k} className="flex min-w-0 items-center gap-2 text-sm">
            <input type="color" value={palette[k]} disabled={disabled} onChange={(e) => onChange(k, e.target.value.toLowerCase())} className="h-8 w-10 shrink-0 rounded border" aria-label={`${t(`colours.${k}`)} (${t(`scheme.${scheme}`)})`} data-testid={`colour-${scheme}-${k}`} />
            <span className="min-w-0 flex-1">{t(`colours.${k}`)}</span>
            <code className="shrink-0 text-xs text-muted-foreground">{palette[k]}</code>
          </label>
        ))}
      </div>
    </fieldset>
  );
}

/** The organiser's uploads, with delete (refused while any version shows the image). */
function UploadsLibrary({ readOnly }: { readOnly: boolean }) {
  const t = useTranslations("builder");
  const te = useTranslations("errors");
  const [items, setItems] = useState<AssetView[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  return (
    <details className="rounded-md border p-2 text-sm" data-testid="uploads" onToggle={async (e) => {
      if (!(e.currentTarget as HTMLDetailsElement).open || items) return;
      const r = await listAssetsAction();
      if (r.ok) setItems(r.data);
    }}>
      <summary className="cursor-pointer font-medium">{t("uploads")}</summary>
      {items && items.length === 0 && <p className="mt-2 text-muted-foreground">{t("noUploads")}</p>}
      <ul className="mt-2 grid grid-cols-3 gap-2">
        {(items ?? []).map((a) => (
          <li key={a.id} className="space-y-1" data-testid="upload-item">
            {/* eslint-disable-next-line @next/next/no-img-element */}
            <img src={a.url} alt="" className="aspect-square w-full rounded border object-contain" />
            {!readOnly && (
              <Button
                type="button"
                size="xs"
                variant="ghost"
                data-testid="delete-upload"
                onClick={async () => {
                  setError(null);
                  const r = await deleteAssetAction(a.id);
                  if (r.ok) setItems(r.data);
                  else setError(r.code);
                }}
              >
                {t("deleteUpload")}
              </Button>
            )}
          </li>
        ))}
      </ul>
      {error && (
        <p role="alert" className="mt-1 text-xs text-destructive">
          {te.has(error) ? te(error) : te("invalid")}
        </p>
      )}
    </details>
  );
}

export function BrandKitPanel({ doc, setDoc, readOnly }: { doc: StorefrontDocument; setDoc: SetDoc; readOnly: boolean }) {
  const t = useTranslations("builder");
  const [proposal, setProposal] = useState<Pick<Tokens, "light" | "dark"> | null>(null);
  const tokens = doc.tokens;
  const setTokens = (patch: Partial<Tokens>) => setDoc((d) => ({ ...d, tokens: { ...d.tokens, ...patch } }));
  const checks = checkContrast(tokens);
  const failing = checks.filter((c) => !c.ok);

  return (
    <div className="space-y-4" data-testid="brand-kit">
      <h2 className="font-semibold">{t("tabs.brand")}</h2>
      <ImagePicker
        label={t("logo")}
        value={tokens.logo}
        disabled={readOnly}
        testId="logo"
        onChange={(v) => setTokens({ logo: v })}
        onUploaded={async (file) => {
          const px = await pixelsOf(file);
          const p = px ? paletteFromPixels(px) : null;
          setProposal(p);
        }}
      />
      {proposal && (
        <div className="space-y-2 rounded-md border p-2" data-testid="palette-proposal">
          <p className="text-sm font-medium">{t("proposal")}</p>
          <div className="flex flex-wrap gap-1">
            {PALETTE_KEYS.map((k) => (
              <Swatch key={k} colour={proposal.light[k]} />
            ))}
          </div>
          <div className="flex gap-2">
            <Button type="button" size="sm" onClick={() => (setTokens({ light: proposal.light, dark: proposal.dark }), setProposal(null))} data-testid="accept-proposal">
              {t("useProposal")}
            </Button>
            <Button type="button" size="sm" variant="ghost" onClick={() => setProposal(null)}>
              {t("dismiss")}
            </Button>
          </div>
        </div>
      )}
      <UploadsLibrary readOnly={readOnly} />
      <label className="grid gap-1 text-sm font-medium">
        {t("mode")}
        <Select value={tokens.mode} disabled={readOnly} onChange={(e) => setTokens({ mode: e.target.value as Tokens["mode"] })} data-testid="mode">
          {MODES.map((m) => (
            <option key={m} value={m}>
              {t(`modes.${m}`)}
            </option>
          ))}
        </Select>
      </label>
      {tokens.mode !== "dark" && <PaletteEditor scheme="light" palette={tokens.light} disabled={readOnly} onChange={(k, v) => setTokens({ light: { ...tokens.light, [k]: v } })} />}
      {tokens.mode !== "light" && <PaletteEditor scheme="dark" palette={tokens.dark} disabled={readOnly} onChange={(k, v) => setTokens({ dark: { ...tokens.dark, [k]: v } })} />}

      <section className="space-y-2" aria-label={t("contrast")} data-testid="contrast">
        <h3 className="text-sm font-semibold">
          {t("contrast")} {failing.length === 0 ? <span className="text-emerald-700 dark:text-emerald-400">· {t("contrastOk")}</span> : <span className="text-destructive">· {t("contrastFailing", { n: failing.length })}</span>}
        </h3>
        <ul className="space-y-1 text-sm">
          {checks.map((c) => (
            <li
              key={`${c.scheme}-${c.fg}-${c.bg}-${c.over ?? ""}`}
              className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2 gap-y-1 rounded-md border p-1.5"
              data-testid="contrast-pair"
              data-ok={c.ok ? "true" : "false"}
            >
              <span className="flex gap-0.5">
                <Swatch colour={tokens[c.scheme][c.fg]} />
                <Swatch colour={tokens[c.scheme][c.bg]} />
              </span>
              <span className="min-w-0">
                {c.over ? t("overImage", { text: t(`colours.${c.fg}`), shade: t(`imageShade.${c.over}`) }) : `${t(`colours.${c.fg}`)} / ${t(`colours.${c.bg}`)}`}{" "}
                <span className="text-muted-foreground">({t(`scheme.${c.scheme}`)})</span>
              </span>
              <span className={`text-right tabular-nums ${c.ok ? "text-muted-foreground" : "font-semibold text-destructive"}`} data-testid="contrast-ratio">
                {c.ratio.toFixed(2)}:1 {c.ok ? "✓" : `< ${c.min}:1`}
              </span>
              {!c.ok && c.suggestion && !readOnly && (
                <Button type="button" size="xs" variant="outline" className="col-span-3 justify-self-start" onClick={() => setTokens(acceptSuggestion(tokens, c))} data-testid="accept-suggestion">
                  <Swatch colour={c.suggestion} /> {t("useSuggestion", { colour: c.suggestion })}
                </Button>
              )}
            </li>
          ))}
        </ul>
        <Hint>{t("contrastHint")}</Hint>
      </section>

      <div className="grid gap-2 sm:grid-cols-2">
        {(["fontHeading", "fontBody"] as const).map((k) => (
          <label key={k} className="grid gap-1 text-sm font-medium">
            {t(`fields.${k}`)}
            <Select value={tokens[k]} disabled={readOnly} onChange={(e) => setTokens({ [k]: e.target.value } as Partial<Tokens>)} data-testid={k}>
              {FONT_IDS.map((f) => (
                <option key={f} value={f}>
                  {FONTS[f].label}
                </option>
              ))}
            </Select>
          </label>
        ))}
      </div>
      <label className="grid gap-1 text-sm font-medium">
        {t("radius")}
        <Select value={tokens.radius} disabled={readOnly} onChange={(e) => setTokens({ radius: e.target.value as Tokens["radius"] })} data-testid="radius">
          {RADII.map((r) => (
            <option key={r} value={r}>
              {t(`radii.${r}`)}
            </option>
          ))}
        </Select>
      </label>

      <fieldset className="space-y-2">
        <legend className="text-sm font-semibold">{t("languages")}</legend>
        <div className="flex flex-wrap gap-3">
          {(["ro", "en"] as const).map((l) => (
            <label key={l} className="inline-flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="size-4"
                disabled={readOnly || (doc.locales.enabled.length === 1 && doc.locales.enabled[0] === l)}
                checked={doc.locales.enabled.includes(l)}
                onChange={(e) =>
                  setDoc((d) => {
                    const enabled = e.target.checked ? (["ro", "en"] as const).filter((x) => x === l || d.locales.enabled.includes(x)) : d.locales.enabled.filter((x) => x !== l);
                    return { ...d, locales: { enabled: [...enabled], default: enabled.includes(d.locales.default) ? d.locales.default : enabled[0] } };
                  })
                }
              />
              {t(`languageNames.${l}`)}
            </label>
          ))}
        </div>
        <label className="grid gap-1 text-sm font-medium">
          {t("defaultLanguage")}
          <Select value={doc.locales.default} disabled={readOnly} onChange={(e) => setDoc((d) => ({ ...d, locales: { ...d.locales, default: e.target.value as "ro" | "en" } }))}>
            {doc.locales.enabled.map((l) => (
              <option key={l} value={l}>
                {t(`languageNames.${l}`)}
              </option>
            ))}
          </Select>
        </label>
      </fieldset>

      <label className="inline-flex items-center gap-2 text-sm">
        <input type="checkbox" className="size-4" disabled={readOnly} checked={doc.checkout.showLogo} onChange={(e) => setDoc((d) => ({ ...d, checkout: { ...d.checkout, showLogo: e.target.checked } }))} />
        {t("checkoutLogo")}
      </label>
    </div>
  );
}
