"use client";
// "Change template" (B2): replaces the pages after confirmation and keeps the brand's tokens.
import { useState } from "react";
import { useTranslations } from "next-intl";
import { Button } from "@/components/ui/button";
import { changeTemplateAction } from "@/app/(platform)/studio/builder/actions";
import { TEMPLATE_IDS, type TemplateId } from "@/lib/storefront/templates";
import type { StorefrontDocument } from "@/lib/storefront/schema";

export function TemplatePicker({ storefrontId, getRevision, flush, onApplied }: { storefrontId: string; getRevision: () => number; flush: () => Promise<{ ok: boolean; code?: string }>; onApplied: (r: { revision: number; document: StorefrontDocument }) => void }) {
  const t = useTranslations("builder");
  const te = useTranslations("errors");
  const [choice, setChoice] = useState<TemplateId>("festival");
  const [confirming, setConfirming] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  return (
    <div className="flex flex-wrap items-center gap-1" data-testid="template-picker">
      <label className="sr-only" htmlFor="template-choice">
        {t("template")}
      </label>
      <select id="template-choice" value={choice} onChange={(e) => (setChoice(e.target.value as TemplateId), setConfirming(false))} className="rounded-md border bg-background px-2 py-1 text-sm" data-testid="template-choice">
        {TEMPLATE_IDS.map((id) => (
          <option key={id} value={id}>
            {t(`templates.${id}`)}
          </option>
        ))}
      </select>
      {!confirming ? (
        <Button type="button" size="sm" variant="outline" onClick={() => setConfirming(true)} data-testid="change-template">
          {t("changeTemplate")}
        </Button>
      ) : (
        <span role="alertdialog" aria-label={t("changeTemplate")} className="flex flex-wrap items-center gap-1 rounded-md border border-amber-500/60 p-1 text-xs">
          {t("changeTemplateConfirm")}
          <Button
            type="button"
            size="xs"
            disabled={busy}
            data-testid="confirm-template"
            onClick={async () => {
              setBusy(true);
              setError(null);
              const saved = await flush();
              if (!saved.ok) {
                setBusy(false);
                setConfirming(false);
                setError(saved.code ?? "unexpected");
                return;
              }
              const r = await changeTemplateAction(storefrontId, getRevision(), choice);
              setBusy(false);
              setConfirming(false);
              if (r.ok) onApplied(r.data);
              else setError(r.code);
            }}
          >
            {t("confirm")}
          </Button>
          <Button type="button" size="xs" variant="ghost" onClick={() => setConfirming(false)}>
            {t("cancel")}
          </Button>
        </span>
      )}
      {error && (
        <span role="alert" className="text-xs text-destructive">
          {te.has(error) ? te(error) : te("invalid")}
        </span>
      )}
    </div>
  );
}
