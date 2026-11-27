"use client";
import { useTranslations } from "next-intl";
import { useFormResult } from "./action-form";
import { CopyButton } from "./copy-button";

/** Shows the invitation link once, right after it is created (only its hash is stored). */
export function InviteLink() {
  const state = useFormResult();
  const t = useTranslations("team");
  const path = state.ok ? state.data?.path : undefined;
  if (!path) return null;
  const url = typeof window === "undefined" ? path : `${window.location.origin}${path}`;
  return (
    <div className="space-y-1 rounded-md border p-2 text-sm" data-testid="invite-link">
      <p>{t("linkLead")}</p>
      <div className="flex min-w-0 flex-wrap items-center gap-2">
        <code className="min-w-0 flex-1 break-all text-xs" data-testid="invite-url">
          {url}
        </code>
        <CopyButton value={url} />
      </div>
    </div>
  );
}
