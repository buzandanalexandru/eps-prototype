// Storefront builder spec "Email and ticket design preview" (task 6.1): the brand kit reaches the
// confirmation email and the ticket; the checkout preview is locked.
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { CheckoutPreview, ConfirmationEmail, sampleOrder, TicketPreview } from "@/components/storefront/messages";
import { SHOP_LABELS } from "@/lib/storefront/labels";
import { clone, doc } from "./storefront-fixtures";

describe("email, ticket and checkout previews", () => {
  it("the ticket shows the logo, the new primary colour and a placeholder QR code", () => {
    const d = clone(doc());
    d.tokens.mode = "light";
    d.tokens.light.primary = "#0a7d32";
    d.tokens.logo = "sample:logo-nibiru";
    const html = renderToStaticMarkup(<TicketPreview doc={d} labels={SHOP_LABELS.ro} brandName="NIBIRU" order={sampleOrder("NIBIRU", "RON", "ro")} />);
    expect(html.includes('data-band="#0a7d32"')).toBe(true);
    expect(html.includes("/samples/logo-nibiru.svg")).toBe(true);
    expect(html.includes('data-testid="ticket-qr"')).toBe(true);
    expect(html.includes("Cod QR de exemplu")).toBe(true);
  });

  it("the email uses the tokens' colours inline, the logo, the all-in total and the footer text per language, escaped", () => {
    const d = clone(doc());
    d.tokens.mode = "light";
    d.messages.email.footer = { ro: "Mulțumim! <b>x</b>", en: "Thanks! <script>alert(1)</script>" };
    d.tokens.logo = "sample:logo-sunwave";
    const en = renderToStaticMarkup(<ConfirmationEmail doc={d} labels={SHOP_LABELS.en} locale="en" brandName="Sunwave" order={sampleOrder("Sunwave", "EUR", "en")} origin="https://eps.example" />);
    expect(en.includes(`background:${d.tokens.light.primary}`)).toBe(true);
    expect(en.includes("https://eps.example/samples/logo-sunwave.svg")).toBe(true);
    expect(en.includes("EUR\u00a01,104.96")).toBe(true);
    expect(en.includes("&lt;script&gt;")).toBe(true);
    expect(en.match(/<script/)).toBeNull();
    const ro = renderToStaticMarkup(<ConfirmationEmail doc={d} labels={SHOP_LABELS.ro} locale="ro" brandName="Sunwave" order={sampleOrder("Sunwave", "EUR", "ro")} />);
    expect(ro.includes("Comanda ta este confirmată")).toBe(true);
    expect(ro.includes("Mulțumim!")).toBe(true);
  });

  it("the checkout preview shows the tokens, the two named slots and no editing controls", () => {
    const html = renderToStaticMarkup(<CheckoutPreview doc={doc()} labels={SHOP_LABELS.en} brandName="B" />);
    expect(html.includes("checkout.slots.afterContact")).toBe(true);
    expect(html.includes("checkout.slots.consents")).toBe(true);
    expect(html.includes("--eps-l-primary")).toBe(true);
    expect(html.match(/<input|<textarea|<select/)).toBeNull();
  });
});
