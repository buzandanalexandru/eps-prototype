// Storefront builder spec "Changing the primary colour": every section reads only the token
// variables, so a new primary reaches buttons, links and highlights everywhere.
import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import { describe, expect, it } from "vitest";
import { tokenVars, TOKEN_VARIABLES } from "@/lib/storefront/tokens";
import { clone, doc } from "./storefront-fixtures";
import { render } from "./storefront-render";

const DIR = path.resolve(__dirname, "../../src/components/storefront");

describe("tokens", () => {
  it("compile to CSS variables for both palettes, fonts and radius, only from validated values", () => {
    const v = tokenVars(doc().tokens);
    expect(v["--eps-l-primary"]).toBe("#6d28d9");
    expect(v["--eps-d-on-surface"]).toBe("#f5f3ff");
    expect(v["--eps-radius"]).toBe("20px");
    expect(v["--eps-font-heading"]).toContain("--eps-ff-space-grotesk");
    for (const value of Object.values(v)) expect(value).not.toMatch(/[;{}<>]/);
  });

  it("no storefront component has a colour, font or radius of its own: only var(--eps-*)", () => {
    const files = readdirSync(DIR).filter((f) => f.endsWith(".tsx") && f !== "messages.tsx");
    expect(files).toEqual(expect.arrayContaining(["sections.tsx", "storefront.tsx", "countdown.tsx", "gallery.tsx", "markdown.tsx"]));
    // Tailwind colour utilities (bg-red-500, text-white, ...), hex or rgb colours, fixed font families.
    const colourClass = /\b(?:bg|text|border|outline|ring|fill|stroke|from|to|via|divide|decoration|accent|shadow)-(?:white|black|(?:slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose)-\d{2,3})\b/;
    for (const f of files) {
      const src = readFileSync(path.join(DIR, f), "utf8");
      expect(src, f).not.toMatch(colourClass);
      expect(src, f).not.toMatch(/#[0-9a-fA-F]{3,8}\b(?![^"]*eps)/);
      expect(src, f).not.toMatch(/rgba?\(|hsla?\(|oklch\(/);
      expect(src, f).not.toMatch(/font-(?:sans|serif|mono)\b/);
      expect(src, f).not.toMatch(/rounded-(?:sm|md|lg|xl|2xl|3xl|full)\b/);
      for (const m of src.matchAll(/var\((--[a-z-]+)\)/g)) expect(TOKEN_VARIABLES as readonly string[], `${f}: ${m[1]}`).toContain(m[1]);
    }
  });

  it("changing the primary colour changes only the variable: every section's buttons and links follow it", () => {
    const a = doc();
    const b = clone(a);
    b.tokens.light.primary = "#0a7d32";
    const ha = render(a);
    const hb = render(b);
    expect(hb).toContain("--eps-l-primary:#0a7d32");
    // Apart from the variable, the markup is identical: no section carries a colour of its own.
    expect(hb.replace("#0a7d32", "#6d28d9") === ha, "markup apart from the variable").toBe(true);
    expect(ha).toContain("bg-[var(--eps-primary)]");
  });
});
