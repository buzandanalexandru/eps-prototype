"use client";
// The locked Checkout tab (D2.6) and the email and ticket design (D2.7, B12).
import { useTranslations } from "next-intl";
import { Hint, Select } from "@/components/ui/field";
import { SHOP_LABELS } from "@/lib/storefront/labels";
import type { Locale, StorefrontDocument } from "@/lib/storefront/schema";
import { CheckoutPreview, ConfirmationEmail, sampleOrder, TicketPreview } from "@/components/storefront/messages";
import { shopFontClass } from "@/components/storefront/fonts";
import { LocalizedInput } from "./settings-panel";
import type { BuilderBrand } from "./builder-client";

type SetDoc = (fn: (d: StorefrontDocument) => StorefrontDocument) => void;

export function CheckoutPanel({ doc, brandName, locale }: { doc: StorefrontDocument; brandName: string; locale: Locale }) {
  const t = useTranslations("builder");
  return (
    <div className="space-y-3" data-testid="checkout-tab">
      <h2 className="font-semibold">{t("tabs.checkout")}</h2>
      <p className="rounded-md border p-2 text-sm" data-testid="checkout-locked-note">
        {t("checkoutLocked")}
      </p>
      <CheckoutPreview doc={doc} labels={SHOP_LABELS[locale]} brandName={brandName} fontClass={shopFontClass} />
    </div>
  );
}

export function MessagesPanel({ doc, setDoc, brand, locale, readOnly }: { doc: StorefrontDocument; setDoc: SetDoc; brand: BuilderBrand; locale: Locale; readOnly: boolean }) {
  const t = useTranslations("builder");
  const order = sampleOrder(brand.name, brand.currency, locale);
  const m = doc.messages;
  const set = (patch: Partial<StorefrontDocument["messages"]>) => setDoc((d) => ({ ...d, messages: { ...d.messages, ...patch } }));
  return (
    <div className="space-y-4" data-testid="messages-tab">
      <h2 className="font-semibold">{t("tabs.messages")}</h2>
      <LocalizedInput label={t("emailFooter")} value={m.email.footer} locales={doc.locales.enabled} multiline max={500} disabled={readOnly} onChange={(v) => set({ email: { ...m.email, footer: v } })} testId="field-email-footer" />
      <label className="inline-flex items-center gap-2 text-sm">
        <input type="checkbox" className="size-4" disabled={readOnly} checked={m.email.showLogo} onChange={(e) => set({ email: { ...m.email, showLogo: e.target.checked } })} />
        {t("emailLogo")}
      </label>
      <label className="inline-flex items-center gap-2 text-sm">
        <input type="checkbox" className="size-4" disabled={readOnly} checked={m.ticket.showLogo} onChange={(e) => set({ ticket: { ...m.ticket, showLogo: e.target.checked } })} />
        {t("ticketLogo")}
      </label>
      <label className="grid gap-1 text-sm font-medium">
        {t("ticketAccent")}
        <Select value={m.ticket.accent} disabled={readOnly} onChange={(e) => set({ ticket: { ...m.ticket, accent: e.target.value as "primary" | "accent" } })}>
          <option value="primary">{t("colours.primary")}</option>
          <option value="accent">{t("colours.accent")}</option>
        </Select>
      </label>
      <Hint>{t("messagesHint")}</Hint>
      <h3 className="text-sm font-semibold">{t("emailPreview")}</h3>
      <div className="overflow-hidden rounded-md border">
        <ConfirmationEmail doc={doc} labels={SHOP_LABELS[locale]} locale={locale} brandName={brand.name} order={order} />
      </div>
      <h3 className="text-sm font-semibold">{t("ticketPreview")}</h3>
      <TicketPreview doc={doc} labels={SHOP_LABELS[locale]} brandName={brand.name} order={order} />
      <a className="text-sm underline" href={`/studio/builder/messages?brand=${brand.id}&lang=${locale}`} data-testid="messages-page-link">
        {t("openMessagesPage")}
      </a>
    </div>
  );
}
