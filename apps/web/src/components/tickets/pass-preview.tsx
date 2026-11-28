"use client";
// Wallet pass preview (F4, Thin): what an Apple or Google Wallet pass of this ticket would look like,
// in the brand's colours. No pass file is made: real passes need the organiser's signing accounts.
import { useState } from "react";
import { useTranslations } from "next-intl";

export type PassLook = { brand: string; primary: string; onPrimary: string };

export function PassPreview({ look, event, product, holder, when }: { look: PassLook; event: string; product: string; holder: string | null; when: string }) {
  const t = useTranslations("ticket");
  const [open, setOpen] = useState<"apple" | "google" | null>(null);
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap gap-2">
        <button type="button" className="rounded-md bg-black px-3 py-2 text-sm font-semibold text-white" onClick={() => setOpen("apple")} data-testid="add-apple-wallet">
          {t("walletApple")}
        </button>
        <button type="button" className="rounded-md border px-3 py-2 text-sm font-semibold" onClick={() => setOpen("google")} data-testid="add-google-wallet">
          {t("walletGoogle")}
        </button>
      </div>
      {open && (
        <div role="dialog" aria-modal="true" aria-label={t("passPreview")} className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4" data-testid="pass-preview" data-wallet={open}>
          <div className="w-full max-w-sm space-y-3 rounded-2xl bg-background p-4 shadow-xl">
            <p className="text-sm font-semibold">
              {t("passPreview")} · {open === "apple" ? "Apple Wallet" : "Google Wallet"}
            </p>
            <div className={`space-y-3 p-4 ${open === "apple" ? "rounded-xl" : "rounded-3xl"}`} style={{ background: look.primary, color: look.onPrimary }}>
              <p className="text-xs uppercase tracking-wide opacity-80">{look.brand}</p>
              <p className="text-lg font-bold break-words">{event}</p>
              <div className="grid grid-cols-2 gap-2 text-sm">
                <div>
                  <p className="text-xs opacity-80">{t("productLabel")}</p>
                  <p className="font-semibold break-words">{product}</p>
                </div>
                <div>
                  <p className="text-xs opacity-80">{t("holderLabel")}</p>
                  <p className="font-semibold break-words">{holder ?? "—"}</p>
                </div>
              </div>
              <p className="text-sm">{when}</p>
              <div className="mx-auto grid size-28 place-items-center rounded-lg bg-white text-center text-xs text-black">QR</div>
            </div>
            <p className="text-sm text-muted-foreground" data-testid="pass-note">
              {t("passNote")}
            </p>
            <button type="button" className="w-full rounded-md border px-3 py-2 text-sm font-semibold" onClick={() => setOpen(null)} data-testid="pass-close">
              {t("close")}
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
