// Brand kit on the locked checkout, the confirmation email and the ticket (design D2.6, D2.7; B6, B12).
// The checkout preview has no editing controls: only the tokens, the logo and the named slots show.
// The email is table-based HTML with inline colours from the validated tokens (mail clients ignore
// CSS variables); every text is escaped by React.
import type { ShopLabels } from "@/lib/storefront/labels";
import { imageUrl } from "@/lib/storefront/links";
import { markdownText } from "@/lib/storefront/markdown";
import type { Locale, Palette, StorefrontDocument } from "@/lib/storefront/schema";
import { formatMoney, type Currency } from "@/lib/pricing/money";
import { tokenVars } from "@/lib/storefront/tokens";
import { FONTS } from "@/lib/storefront/fonts";
import { primaryButton } from "./sections";

export type SampleOrder = {
  code: string;
  buyer: string;
  eventName: string;
  when: string;
  lines: { name: string; qty: number; allInMinor: number }[];
  currency: Currency;
};

export function sampleOrder(brandName: string, currency: Currency, locale: Locale): SampleOrder {
  return {
    code: "EPS-SAMPLE-0001",
    buyer: locale === "ro" ? "Ana Popescu" : "Ana Popescu",
    eventName: brandName,
    when: locale === "ro" ? "15 iulie 2027, 18:00" : "15 July 2027, 18:00",
    lines: [{ name: locale === "ro" ? "Abonament general" : "General pass", qty: 2, allInMinor: 55248 }],
    currency,
  };
}

function Slot({ name, label }: { name: string; label: string }) {
  return (
    <div data-testid={`slot-${name}`} className="rounded-[var(--eps-radius)] border-2 border-dashed border-[var(--eps-muted)] p-3 text-sm text-[var(--eps-muted)]">
      <span className="font-semibold">{label}</span> · <code className="text-xs">checkout.slots.{name}</code>
    </div>
  );
}

export function CheckoutPreview({ doc, labels, brandName, fontClass, scheme }: { doc: StorefrontDocument; labels: ShopLabels; brandName: string; fontClass?: string; scheme?: "light" | "dark" | null }) {
  const logo = doc.checkout.showLogo && doc.tokens.logo ? imageUrl(doc.tokens.logo) : null;
  return (
    <div
      className={`eps-shop rounded-[var(--eps-radius)] bg-[var(--eps-surface)] p-4 font-[family-name:var(--eps-font-body)] text-[var(--eps-on-surface)] sm:p-6 ${fontClass ?? ""}`}
      data-eps-mode={scheme ?? doc.tokens.mode}
      style={tokenVars(doc.tokens) as React.CSSProperties}
      data-testid="checkout-preview"
      aria-readonly="true"
    >
      <div className="mx-auto max-w-lg space-y-4">
        <div className="flex items-center gap-2">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          {logo && <img src={logo} alt="" className="h-8 w-auto max-w-28 object-contain" />}
          <span className="font-[family-name:var(--eps-font-heading)] font-bold">{brandName}</span>
        </div>
        <h2 className="font-[family-name:var(--eps-font-heading)] text-2xl font-bold">{labels.checkoutTitle}</h2>
        <fieldset disabled className="space-y-3">
          <legend className="font-semibold">{labels.contact}</legend>
          {[labels.name, labels.email].map((l) => (
            <label key={l} className="grid gap-1 text-sm">
              {l}
              <span className="h-10 rounded-[var(--eps-radius)] border border-[var(--eps-muted)]" />
            </label>
          ))}
        </fieldset>
        <Slot name="afterContact" label={labels.slotQuestions} />
        <Slot name="consents" label={labels.slotConsents} />
        <div className="flex items-center justify-between border-t border-[var(--eps-muted)] pt-3">
          <span className="font-semibold">{labels.total}</span>
          <span className="text-xl font-bold">{formatMoney(55248, "RON", "ro")}</span>
        </div>
        <span className={`${primaryButton} w-full`}>{labels.pay}</span>
      </div>
    </div>
  );
}

function palette(doc: StorefrontDocument, scheme?: "light" | "dark" | null): Palette {
  return (scheme ?? (doc.tokens.mode === "dark" ? "dark" : "light")) === "dark" ? doc.tokens.dark : doc.tokens.light;
}

/** The confirmation email body (HTML-safe React; `absolute` makes image URLs absolute for mail). */
export function ConfirmationEmail({ doc, labels, locale, brandName, order, origin = "" }: { doc: StorefrontDocument; labels: ShopLabels; locale: Locale; brandName: string; order: SampleOrder; origin?: string }) {
  const p = palette(doc);
  const font = FONTS[doc.tokens.fontBody].css.replace(/var\([^)]*\),\s*/g, "");
  const logo = doc.messages.email.showLogo && doc.tokens.logo ? imageUrl(doc.tokens.logo) : null;
  const total = order.lines.reduce((s, l) => s + l.qty * l.allInMinor, 0);
  const footer = markdownText(doc.messages.email.footer[locale] ?? "");
  return (
    <table role="presentation" width="100%" cellPadding={0} cellSpacing={0} style={{ background: p.surface, color: p.onSurface, fontFamily: font }} data-testid="email-preview">
      <tbody>
        <tr>
          <td align="center" style={{ padding: 24 }}>
            <table role="presentation" width="100%" cellPadding={0} cellSpacing={0} style={{ maxWidth: 560 }}>
              <tbody>
                <tr>
                  <td style={{ paddingBottom: 16, fontWeight: 700, fontSize: 18 }}>
                    {/* eslint-disable-next-line @next/next/no-img-element */}
                    {logo && <img src={`${origin}${logo}`} alt={brandName} height={36} style={{ height: 36, display: "block", marginBottom: 8 }} />}
                    {brandName}
                  </td>
                </tr>
                <tr>
                  <td style={{ fontSize: 22, fontWeight: 700, paddingBottom: 8 }}>{labels.emailSubject}</td>
                </tr>
                <tr>
                  <td style={{ paddingBottom: 16 }}>
                    {labels.emailHello} {order.buyer}, {labels.emailLead}
                  </td>
                </tr>
                <tr>
                  <td style={{ paddingBottom: 16, color: p.muted }}>
                    {labels.emailOrder} {order.code} · {order.eventName} · {order.when}
                  </td>
                </tr>
                {order.lines.map((l, i) => (
                  <tr key={i}>
                    <td style={{ padding: "8px 0", borderTop: `1px solid ${p.muted}` }}>
                      {l.qty} × {l.name} — {formatMoney(l.qty * l.allInMinor, order.currency, locale)}
                    </td>
                  </tr>
                ))}
                <tr>
                  <td style={{ padding: "8px 0 16px", borderTop: `1px solid ${p.muted}`, fontWeight: 700 }}>
                    {labels.total}: {formatMoney(total, order.currency, locale)}
                  </td>
                </tr>
                <tr>
                  <td style={{ paddingBottom: 24 }}>
                    <a href="#" style={{ background: p.primary, color: p.onPrimary, padding: "12px 20px", textDecoration: "none", fontWeight: 700, display: "inline-block" }}>
                      {labels.emailOpenTickets}
                    </a>
                  </td>
                </tr>
                {footer && (
                  <tr>
                    <td style={{ fontSize: 13, color: p.muted, whiteSpace: "pre-line" }} data-testid="email-footer">
                      {footer}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          </td>
        </tr>
      </tbody>
    </table>
  );
}

/** A deterministic placeholder QR pattern (not a real code: tickets are issued in step 7). */
export function PlaceholderQr({ seed, colour, label }: { seed: string; colour: string; label: string }) {
  let h = 2166136261;
  for (const ch of seed) h = Math.imul(h ^ ch.charCodeAt(0), 16777619);
  const cells: [number, number][] = [];
  for (let y = 0; y < 21; y++)
    for (let x = 0; x < 21; x++) {
      const finder = (x < 7 && y < 7) || (x > 13 && y < 7) || (x < 7 && y > 13);
      h = Math.imul(h ^ (x * 31 + y), 16777619);
      const on = finder ? x % 6 === 0 || y % 6 === 0 || (x % 7 >= 2 && x % 7 <= 4 && y % 7 >= 2 && y % 7 <= 4) || (x > 13 && (x === 20 || x === 14)) : (h >>> 7) % 2 === 0;
      if (on) cells.push([x, y]);
    }
  return (
    <svg viewBox="0 0 21 21" width={140} height={140} role="img" aria-label={label} data-testid="ticket-qr" style={{ background: "#ffffff", padding: 6 }}>
      {cells.map(([x, y]) => (
        <rect key={`${x}-${y}`} x={x} y={y} width={1} height={1} fill={colour} />
      ))}
    </svg>
  );
}

export function TicketPreview({ doc, labels, brandName, order, scheme }: { doc: StorefrontDocument; labels: ShopLabels; brandName: string; order: SampleOrder; scheme?: "light" | "dark" | null }) {
  const p = palette(doc, scheme);
  const band = doc.messages.ticket.accent === "accent" ? p.accent : p.primary;
  const onBand = doc.messages.ticket.accent === "accent" ? p.surface : p.onPrimary;
  const logo = doc.messages.ticket.showLogo && doc.tokens.logo ? imageUrl(doc.tokens.logo) : null;
  return (
    <div
      data-testid="ticket-preview"
      data-band={band}
      className="w-full max-w-sm overflow-hidden rounded-[var(--eps-radius)] border"
      style={{ ...(tokenVars(doc.tokens) as React.CSSProperties), background: p.surface, color: p.onSurface, borderColor: p.muted, fontFamily: FONTS[doc.tokens.fontBody].css }}
    >
      <div className="flex items-center gap-2 p-4" style={{ background: band, color: onBand }}>
        {/* eslint-disable-next-line @next/next/no-img-element */}
        {logo && <img src={logo} alt="" className="h-8 w-auto max-w-24 object-contain" data-testid="ticket-logo" />}
        <span className="font-bold" style={{ fontFamily: FONTS[doc.tokens.fontHeading].css }}>
          {brandName}
        </span>
      </div>
      <div className="space-y-1 p-4">
        <p className="text-lg font-bold" style={{ fontFamily: FONTS[doc.tokens.fontHeading].css }}>
          {order.eventName}
        </p>
        <p className="text-sm">{order.when}</p>
        <p className="text-sm">{order.lines[0]?.name}</p>
        <p className="text-sm" style={{ color: p.muted }}>
          {labels.ticketHolder}: {order.buyer}
        </p>
      </div>
      <div className="flex flex-col items-center gap-1 p-4 pt-0">
        <PlaceholderQr seed={order.code} colour="#000000" label={labels.ticketSampleQr} />
        <span className="text-xs" style={{ color: p.muted }}>
          {labels.ticketSampleQr} · {order.code}
        </span>
      </div>
    </div>
  );
}
