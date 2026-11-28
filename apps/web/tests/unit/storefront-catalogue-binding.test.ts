// Storefront builder spec "Catalogue sections show live catalogue data".
import { expect, it } from "vitest";
import { setLadder } from "@/lib/catalogue/domain";
import { loadShopCatalogue } from "@/lib/storefront/catalogue-load";
import { saveDraft } from "@/lib/storefront/domain";
import { referencedEvents } from "@/lib/storefront/schema";
import { describeDb, tx } from "./db";
import { clone, shopOrganiser } from "./storefront-fixtures";

describeDb("catalogue binding", () => {
  it("the ticket grid shows the all-in price from the live catalogue; a face price change shows on the next render", async () => {
    const o = await shopOrganiser();
    const ids = referencedEvents(o.sf.document);
    expect(ids).toEqual([o.f.ev.id]);
    const before = await tx(o.scope, (c) => loadShopCatalogue(c, ids));
    const ga = before[o.f.ev.id].products.find((p) => p.id === o.f.ga.id)!;
    expect(ga.allInMinor).toBe(55248); // 539.00 + 2.5% passed on
    expect(before[o.f.ev.id].currency).toBe("RON");
    expect(JSON.stringify(o.sf.document)).not.toContain("552"); // no price stored in the document
    await tx(o.scope, (c) => setLadder(c, o.f.ga.id, { mode: "by_date", steps: [{ priceMinor: 60000 }] }));
    const after = await tx(o.scope, (c) => loadShopCatalogue(c, ids));
    expect(after[o.f.ev.id].products.find((p) => p.id === o.f.ga.id)!.allInMinor).toBe(61500);
    expect(after[o.f.ev.id].days).toHaveLength(4);
  });

  it("a document referencing another organiser's event is refused as an unknown event, and its data never loads", async () => {
    const a = await shopOrganiser();
    const b = await shopOrganiser();
    const d = clone(a.sf.document);
    d.pages[0].sections.find((s) => s.type === "lineup")!.settings.eventId = b.f.ev.id;
    await expect(tx(a.scope, (c) => saveDraft(c, a.sf.id, { revision: 1, document: d }))).rejects.toMatchObject({ code: "unknown_event", field: expect.stringMatching(/^pages\.0\.sections\.\d+\.settings\.eventId$/) });
    expect(await tx(a.scope, (c) => loadShopCatalogue(c, [b.f.ev.id]))).toEqual({});
  });
});
