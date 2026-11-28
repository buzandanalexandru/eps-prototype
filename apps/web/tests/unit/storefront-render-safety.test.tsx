// Storefront builder spec "Organiser content cannot inject code" ("Script in text") and the escaped
// formatting subset; platform labels in both languages.
import { describe, expect, it } from "vitest";
import { parseMarkdown } from "@/lib/storefront/markdown";
import { parseDocument } from "@/lib/storefront/schema";
import { clone, doc } from "./storefront-fixtures";
import { render } from "./storefront-render";

const PAYLOADS = ["<script>alert(1)</script>", '<img src=x onerror="alert(1)">', "<svg onload=alert(1)>", '"><iframe src="javascript:alert(1)"></iframe>', "</style><style>*{display:none}"];

describe("render safety", () => {
  it("script and event-handler text in an FAQ answer, a heading and a title is rendered as visible text", () => {
    const d = clone(doc());
    const faq = d.pages[0].sections.find((s) => s.type === "faq")!;
    faq.blocks[0].settings.answer = { ro: PAYLOADS.join(" "), en: PAYLOADS.join(" ") };
    faq.blocks[0].settings.question = { ro: PAYLOADS[1], en: PAYLOADS[1] };
    d.pages[0].sections[0].settings.heading = { ro: PAYLOADS[0], en: PAYLOADS[0] };
    d.pages[1].title = { ro: PAYLOADS[2], en: PAYLOADS[2] };
    expect(parseDocument(d).ok).toBe(true); // text is text: saving is fine, rendering is safe
    const html = render(d);
    expect(html.match(/<script/i)).toBeNull();
    expect(html.match(/<img[^>]*onerror/i)).toBeNull();
    expect(html.match(/<svg[^>]*onload/i)).toBeNull();
    expect(html.match(/<iframe/i)).toBeNull();
    expect(html.match(/<\/style>/i)).toBeNull();
    expect(html.includes("&lt;script&gt;alert(1)&lt;/script&gt;")).toBe(true);
    expect(html.includes("onerror=&quot;alert(1)&quot;")).toBe(true);
  });

  it("formatting subset: bold, italic, lists and safe links only; an unsafe link becomes plain text", () => {
    expect(parseMarkdown("**Bold** and _it_\n- one\n- two")).toEqual([
      { t: "p", c: [{ t: "b", c: [{ t: "text", v: "Bold" }] }, { t: "text", v: " and " }, { t: "i", c: [{ t: "text", v: "it" }] }] },
      { t: "ul", items: [[{ t: "text", v: "one" }], [{ t: "text", v: "two" }]] },
    ]);
    const d = clone(doc());
    const text = d.pages[0].sections.find((s) => s.type === "faq")!;
    text.blocks[0].settings.answer = { ro: "[ok](https://nibiru.ro) [bad](javascript:alert(1)) [rel](/e/x)", en: "[ok](https://nibiru.ro) [bad](javascript:alert(1)) [rel](/e/x)" };
    const html = render(d);
    expect(html.includes('href="https://nibiru.ro"'), 'href="https://nibiru.ro"').toBe(true);
    expect(html.includes('href="/s/brand/en/e/x"'), 'href="/s/brand/en/e/x"').toBe(true);
    expect(!html.includes("javascript:"), "javascript:").toBe(true);
    expect(html.includes(" bad) "), "unsafe link shown as text").toBe(true);
  });

  it("platform labels are in Romanian with diacritics or English; the footer shows the sample label", () => {
    const ro = render(doc(), "ro", 1);
    expect(ro.includes("Bilete disponibile după publicare"), "Bilete disponibile după publicare").toBe(true);
    expect(ro.includes("Date de exemplu"), "Date de exemplu").toBe(true);
    expect(ro.includes("Taxe și comisioane"), "Taxe și comisioane").toBe(true);
    expect(ro.includes("552,48\u00a0RON"), "552,48 RON").toBe(true);
    const en = render(doc(), "en", 1);
    expect(en.includes("Tickets available after publishing"), "Tickets available after publishing").toBe(true);
    expect(en.includes("RON\u00a0552.48"), "RON 552.48").toBe(true);
    expect(render(doc(), "en", 1, "published").includes("Sales open soon")).toBe(true);
  });
});
