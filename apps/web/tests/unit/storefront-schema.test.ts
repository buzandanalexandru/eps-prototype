// Storefront builder spec: "Invalid document refused", "Footer cannot be removed", "Section limits",
// "Dangerous link", "Layout key on checkout"; the JSON Schema export for the copilot.
import { describe, expect, it } from "vitest";
import { addSection, moveSection, removeSection, toggleHidden } from "@/lib/storefront/edit";
import { documentJsonSchema, LIMITS, parseDocument } from "@/lib/storefront/schema";
import { isSafeLink, parseImageRef } from "@/lib/storefront/links";
import { clone, doc } from "./storefront-fixtures";

const home = (d: ReturnType<typeof doc>) => d.pages[0];

describe("storefront document schema", () => {
  it("every template document validates", () => {
    expect(parseDocument(doc()).ok).toBe(true);
  });

  it("refuses an unknown section type with the path of the first error", () => {
    const d = clone(doc()) as unknown as { pages: { sections: { type: string }[] }[] };
    d.pages[0].sections[1].type = "carousel3d";
    const r = parseDocument(d);
    expect(r).toMatchObject({ ok: false, code: "unknown_type" });
    expect(!r.ok && r.path).toBe("pages.0.sections.1.type");
  });

  it("refuses an unknown key anywhere", () => {
    const d = clone(doc()) as unknown as Record<string, unknown> & { tokens: Record<string, unknown>; pages: { sections: { settings: Record<string, unknown> }[] }[] };
    d.pages[0].sections[0].settings.onclick = "alert(1)";
    expect(parseDocument(d)).toMatchObject({ ok: false, code: "unknown_key", path: "pages.0.sections.0.settings" });
    const e = clone(doc()) as unknown as Record<string, unknown>;
    e.customCss = "body{}";
    expect(parseDocument(e)).toMatchObject({ ok: false, code: "unknown_key" });
    const f = clone(doc()) as unknown as { tokens: Record<string, unknown> };
    f.tokens.css = "x";
    expect(parseDocument(f)).toMatchObject({ ok: false, code: "unknown_key", path: "tokens" });
  });

  it("refuses a document over 512 KB", () => {
    const d = clone(doc());
    const text = home(d).sections.find((s) => s.type === "text") ?? home(d).sections[0];
    // Many sections of long text: each field stays under its own limit, the whole goes over.
    const big = "x".repeat(4900);
    const pages = [];
    for (let i = 0; i < 29; i++) {
      pages.push({
        ...clone(home(d)),
        id: `big-${i}`,
        kind: "info" as const,
        slug: `big-${i}`,
        sections: [
          ...Array.from({ length: 39 }, (_, j) => ({ ...clone(text), id: `t-${i}-${j}`, type: "text" as const, settings: { heading: { ro: "", en: "" }, body: { ro: big, en: big }, align: "left" }, blocks: [] })),
          { ...clone(home(d).sections.at(-1)!), id: `f-${i}` },
        ],
      });
    }
    const r = parseDocument({ ...d, pages: [home(d), ...pages] });
    expect(r).toMatchObject({ ok: false, code: "document_too_large" });
  });

  it("the footer is last, cannot be removed, moved or hidden; a document without it is refused", () => {
    const d = doc();
    const p = home(d);
    const footerId = p.sections.at(-1)!.id;
    expect(p.sections.at(-1)!.type).toBe("footer");
    expect(removeSection(d, p.id, footerId).pages[0].sections.at(-1)!.type).toBe("footer");
    expect(moveSection(d, p.id, p.sections.length - 1, 0)).toEqual(d);
    expect(moveSection(d, p.id, 0, 99).pages[0].sections.at(-1)!.type).toBe("footer");
    expect(toggleHidden(d, p.id, footerId)).toEqual(d);
    // A request that does it anyway is refused.
    const moved = clone(d);
    moved.pages[0].sections.unshift(moved.pages[0].sections.pop()!);
    expect(parseDocument(moved)).toMatchObject({ ok: false, code: "footer_last" });
    const removed = clone(d);
    removed.pages[0].sections.pop();
    expect(parseDocument(removed)).toMatchObject({ ok: false, code: "footer_last" });
    const hidden = clone(d);
    hidden.pages[0].sections.at(-1)!.hidden = true;
    expect(parseDocument(hidden)).toMatchObject({ ok: false, code: "footer_locked" });
  });

  it("section and block limits: adding stops at 40 sections, saving over the limit is refused", () => {
    let d = doc("festival", null);
    const pid = home(d).id;
    while (true) {
      const r = addSection(d, pid, "text");
      if (!r.id) break;
      d = r.doc;
    }
    expect(home(d).sections).toHaveLength(LIMITS.sectionsPerPage);
    expect(parseDocument(d).ok).toBe(true);
    const over = clone(d);
    over.pages[0].sections.splice(0, 0, { ...clone(over.pages[0].sections[0]), id: "one-more" });
    expect(parseDocument(over)).toMatchObject({ ok: false, code: "too_many_sections" });
    const faq = clone(doc());
    const f = faq.pages[0].sections.find((s) => s.type === "faq")!;
    f.blocks = Array.from({ length: 51 }, (_, i) => ({ ...clone(f.blocks[0]), id: `q${i}` }));
    expect(parseDocument(faq)).toMatchObject({ ok: false, code: "too_many_blocks" });
  });

  it("refuses javascript:, data: and other dangerous links and image sources", () => {
    for (const href of ["javascript:alert(1)", "JaVaScRiPt:alert(1)", "data:text/html,<script>alert(1)</script>", "//evil.example", "/\\evil.example", "http://plain.example", "vbscript:x", " https://x.example", "https://user:pw@x.example"]) {
      expect(isSafeLink(href), href).toBe(false);
      const d = clone(doc());
      const hero = d.pages[0].sections[0];
      hero.blocks[0].settings.href = href;
      expect(parseDocument(d), href).toMatchObject({ ok: false, code: "unsafe_link" });
    }
    for (const href of ["https://nibiru.ro/info", "mailto:hello@example.com", "/e/galaxia-2027", "/"]) expect(isSafeLink(href), href).toBe(true);
    for (const src of ["https://evil.example/x.png", "javascript:x", "sample:../../etc", "asset:not-a-uuid", "data:image/png;base64,AAAA"]) {
      expect(parseImageRef(src)).toBeNull();
      const d = clone(doc());
      d.pages[0].sections[0].settings.image = src;
      expect(parseDocument(d)).toMatchObject({ ok: false, code: "unsafe_image" });
    }
    const bad = clone(doc());
    bad.tokens.light.primary = "red; background:url(javascript:x)";
    expect(parseDocument(bad)).toMatchObject({ ok: false, code: "colour_invalid" });
  });

  it("the checkout takes only the logo switch and the two named slots", () => {
    const withSection = clone(doc()) as unknown as { checkout: Record<string, unknown> };
    withSection.checkout.sections = [{ type: "hero" }];
    expect(parseDocument(withSection)).toMatchObject({ ok: false, code: "unknown_key", path: "checkout" });
    const colour = clone(doc()) as unknown as { checkout: Record<string, unknown> };
    colour.checkout.primary = "#ff0000";
    expect(parseDocument(colour)).toMatchObject({ ok: false, code: "unknown_key" });
    const text = clone(doc()) as unknown as { checkout: { slots: Record<string, unknown> } };
    text.checkout.slots.freeText = ["hello"];
    expect(parseDocument(text)).toMatchObject({ ok: false, code: "unknown_key", path: "checkout.slots" });
    const slots = clone(doc());
    slots.checkout.slots = { afterContact: ["tshirt-size"], consents: ["newsletter"] };
    expect(parseDocument(slots).ok).toBe(true);
  });

  it("other structural rules: one home page first, unique ids and slugs, default language enabled", () => {
    const dupe = clone(doc());
    dupe.pages[0].sections[1].id = dupe.pages[0].sections[0].id;
    expect(parseDocument(dupe)).toMatchObject({ ok: false, code: "duplicate_id" });
    const lang = clone(doc());
    lang.locales = { default: "en", enabled: ["ro"] };
    expect(parseDocument(lang)).toMatchObject({ ok: false, code: "default_not_enabled" });
    const order = clone(doc());
    order.pages.reverse();
    expect(parseDocument(order).ok).toBe(false);
  });

  it("exports a JSON Schema that refuses additional properties", () => {
    const s = documentJsonSchema() as { type: string; additionalProperties: boolean; properties: Record<string, unknown> };
    expect(s.type).toBe("object");
    expect(s.additionalProperties).toBe(false);
    expect(Object.keys(s.properties)).toEqual(["schemaVersion", "tokens", "locales", "pages", "checkout", "messages"]);
  });
});
