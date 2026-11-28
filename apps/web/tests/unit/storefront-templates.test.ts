// Storefront builder spec "Every template is valid" and the tokens part of "Change template".
import { describe, expect, it } from "vitest";
import { contrastFailures } from "@/lib/storefront/contrast";
import { publishProblems } from "@/lib/storefront/publish-checks";
import { parseDocument, referencedEvents } from "@/lib/storefront/schema";
import { buildTemplate, changeTemplate, TEMPLATE_IDS, templateForBrand } from "@/lib/storefront/templates";
import { EVENT, EVENT_ID } from "./storefront-fixtures";

describe("templates", () => {
  it.each(TEMPLATE_IDS)("%s: validates, passes contrast in both palettes and the publish checks, for a brand with one event", (id) => {
    const d = buildTemplate(id, { brandName: "Brand", languages: ["ro", "en"], event: EVENT });
    expect(parseDocument(d).ok).toBe(true);
    expect(contrastFailures({ ...d.tokens, mode: "auto" })).toEqual([]);
    expect(publishProblems(d, { knownEvents: new Set([EVENT_ID]) })).toEqual([]);
    expect(referencedEvents(d)).toEqual([EVENT_ID]);
    const eventPage = d.pages.find((p) => p.kind === "event")!;
    expect(eventPage.sections.some((s) => s.type === "ticket_grid")).toBe(true);
    expect(d.pages.every((p) => p.sections.at(-1)!.type === "footer")).toBe(true);
  });

  it.each(TEMPLATE_IDS)("%s: also valid and publishable for a brand without events, in one language", (id) => {
    const d = buildTemplate(id, { brandName: "Brand", languages: ["en"], event: null });
    expect(parseDocument(d).ok).toBe(true);
    expect(publishProblems(d, { knownEvents: new Set() })).toEqual([]);
  });

  it("the onboarding choice picks the template; blank is a minimal home page", () => {
    const blank = templateForBrand("blank", { brandName: "B", languages: ["ro", "en"], event: EVENT });
    expect(blank.pages).toHaveLength(1);
    expect(parseDocument(blank).ok).toBe(true);
    expect(templateForBrand("club", { brandName: "B", languages: ["ro"], event: null }).tokens.fontHeading).toBe("oswald");
  });

  it("changing template replaces the pages and keeps colours, fonts, radius and logo", () => {
    const festival = buildTemplate("festival", { brandName: "B", languages: ["ro", "en"], event: EVENT });
    festival.tokens.light.primary = "#123456";
    festival.tokens.fontHeading = "lora";
    festival.tokens.radius = "none";
    festival.tokens.logo = "sample:logo-sunwave";
    const resort = changeTemplate(festival, "resort", { brandName: "B", languages: ["ro", "en"], event: EVENT });
    expect(resort.tokens).toEqual(festival.tokens);
    expect(resort.pages[0].sections.map((s) => s.type)).toEqual(buildTemplate("resort", { brandName: "B", languages: ["ro", "en"], event: EVENT }).pages[0].sections.map((s) => s.type));
    expect(parseDocument(resort).ok).toBe(true);
  });
});
