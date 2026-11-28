"use client";
// Image field: a platform sample image, an uploaded image, or none. Uploads go to /api/studio/assets,
// which accepts only PNG, JPEG or WebP up to 1 MB (checked from the bytes).
import { useId, useState } from "react";
import { useTranslations } from "next-intl";
import { Select } from "@/components/ui/field";
import { imageUrl, SAMPLE_IMAGES } from "@/lib/storefront/links";

export async function uploadImage(file: File): Promise<{ ok: true; ref: string; url: string } | { ok: false; code: string; params?: Record<string, number> }> {
  const fd = new FormData();
  fd.set("file", file);
  try {
    const res = await fetch("/api/studio/assets", { method: "POST", body: fd });
    const j = (await res.json().catch(() => ({}))) as { ref?: string; url?: string; code?: string; params?: Record<string, number> };
    if (res.ok && j.ref && j.url) return { ok: true, ref: j.ref, url: j.url };
    return { ok: false, code: j.code ?? (res.status === 413 ? "file_too_large" : "unexpected"), params: j.params };
  } catch {
    return { ok: false, code: "network" };
  }
}

export function ImagePicker({
  label,
  value,
  onChange,
  disabled,
  testId,
  onUploaded,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
  testId?: string;
  onUploaded?: (file: File, ref: string) => void;
}) {
  const t = useTranslations("builder");
  const te = useTranslations("errors");
  const id = useId();
  const [error, setError] = useState<{ code: string; params?: Record<string, number> } | null>(null);
  const [busy, setBusy] = useState(false);
  const src = value ? imageUrl(value) : null;
  return (
    <div className="space-y-1" data-testid={testId}>
      <label className="grid gap-1 text-sm font-medium" htmlFor={id}>
        {label}
        <Select id={id} value={value} disabled={disabled} onChange={(e) => onChange(e.target.value)}>
          <option value="">{t("noImage")}</option>
          {value.startsWith("asset:") && <option value={value}>{t("uploadedImage")}</option>}
          {SAMPLE_IMAGES.map((n) => (
            <option key={n} value={`sample:${n}`}>
              {t("sampleImage", { name: n })}
            </option>
          ))}
        </Select>
      </label>
      <div className="flex items-center gap-2">
        {src && (
          // eslint-disable-next-line @next/next/no-img-element
          <img src={src} alt="" className="h-10 w-16 rounded border object-cover" />
        )}
        {!disabled && (
          <label className="cursor-pointer text-xs font-medium underline">
            {busy ? t("uploading") : t("upload")}
            <input
              type="file"
              accept="image/png,image/jpeg,image/webp"
              className="sr-only"
              data-testid={testId ? `${testId}-upload` : undefined}
              onChange={async (e) => {
                const file = e.target.files?.[0];
                e.target.value = "";
                if (!file) return;
                setBusy(true);
                setError(null);
                const r = await uploadImage(file);
                setBusy(false);
                if (r.ok) {
                  onChange(r.ref);
                  onUploaded?.(file, r.ref);
                } else setError({ code: r.code, params: r.params });
              }}
            />
          </label>
        )}
      </div>
      {error && (
        <p role="alert" className="text-xs text-destructive" data-testid="upload-error">
          {te.has(error.code) ? te(error.code, error.params) : te("invalid")}
        </p>
      )}
    </div>
  );
}
