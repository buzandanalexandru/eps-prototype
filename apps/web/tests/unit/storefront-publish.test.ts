// Storefront builder spec "Publishing creates an immutable version": blocked publishes and double
// clicks; the step 3 hook point runs inside the same transaction.
import { expect, it } from "vitest";
import { listVersions, publishDraft, saveDraft, type PublishHook } from "@/lib/storefront/domain";
import { publishProblems } from "@/lib/storefront/publish-checks";
import { describeDb, testPool, tx } from "./db";
import { clone, shopOrganiser } from "./storefront-fixtures";
import { withTx } from "@/lib/db/tx";

describeDb("storefront publish", () => {
  it("an empty English hero title on a brand that sells in English blocks publishing, with a problem pointing at the field", async () => {
    const o = await shopOrganiser();
    const d = clone(o.sf.document);
    d.pages[0].sections[0].settings.heading = { ro: "Titlu", en: "" };
    const rev = (await tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: 1, document: d }))).revision;
    const err = await tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: rev, userId: o.user.id })).catch((e) => e);
    expect(err.code).toBe("publish_blocked");
    expect(err.problems).toEqual([expect.objectContaining({ code: "missing_translation", locale: "en", path: "pages.0.sections.0.settings.heading.en", pageId: d.pages[0].id, sectionId: d.pages[0].sections[0].id })]);
    expect(await tx(o.scope, (c) => listVersions(c, o.sf.id))).toEqual([]);
  });

  it("also blocks on contrast, an event page without a ticket grid and an empty required link", () => {
    const base = { knownEvents: new Set<string>() };
    return shopOrganiser().then((o) => {
      const known = { knownEvents: new Set([o.f.ev.id]) };
      expect(publishProblems(o.sf.document, known)).toEqual([]);
      const d = clone(o.sf.document);
      d.tokens.light.onSurface = d.tokens.light.surface;
      const ev = d.pages.find((p) => p.kind === "event")!;
      ev.sections = ev.sections.filter((s) => s.type !== "ticket_grid");
      d.pages[0].sections[0].blocks[0].settings.href = "";
      const codes = publishProblems(d, known).map((p) => p.code);
      expect(codes).toEqual(expect.arrayContaining(["contrast", "event_page_needs_tickets", "required"]));
      expect(publishProblems(o.sf.document, base).some((p) => p.code === "unknown_event")).toBe(true);
    });
  });

  it("publish sent twice for the same draft revision creates exactly one version, even concurrently", async () => {
    const o = await shopOrganiser();
    const run = () => withTx(testPool(), o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: 1, userId: o.user.id }));
    const [a, b] = await Promise.all([run(), run()]);
    expect([a.created, b.created].sort()).toEqual([false, true]);
    expect(a.number).toBe(1);
    expect(b.number).toBe(1);
    const again = await run();
    expect(again).toMatchObject({ number: 1, created: false });
    expect(await tx(o.scope, (c) => listVersions(c, o.sf.id))).toHaveLength(1);
  });

  it("a publish based on an older revision is refused; the hook runs inside the transaction and can veto it", async () => {
    const o = await shopOrganiser();
    const d = clone(o.sf.document);
    await tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: 1, document: d }));
    await expect(tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: 1, userId: null }))).rejects.toMatchObject({ code: "draft_conflict" });
    const seen: number[] = [];
    const hook: PublishHook = async (_c, ctx) => {
      seen.push(ctx.number);
      throw new Error("snapshot compile failed");
    };
    await expect(tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: 2, userId: null }, [hook]))).rejects.toThrow("snapshot compile failed");
    expect(seen).toEqual([1]);
    expect(await tx(o.scope, (c) => listVersions(c, o.sf.id))).toEqual([]); // rolled back
  });
});
