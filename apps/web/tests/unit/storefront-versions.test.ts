// Storefront builder spec "Drafts autosave and detect conflicts", "Published version is read-only"
// and "Version history and rollback".
import { expect, it } from "vitest";
import { getStorefront, getVersionDocument, listVersions, publishDraft, restoreVersion, saveDraft } from "@/lib/storefront/domain";
import { describeDb, tx } from "./db";
import { clone, shopOrganiser } from "./storefront-fixtures";

describeDb("storefront versions", () => {
  it("editing the draft of a published storefront leaves the published version unchanged", async () => {
    const o = await shopOrganiser();
    const v1 = await tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: o.sf.draft_revision, userId: o.user.id }));
    expect(v1).toMatchObject({ number: 1, state: "published", created: true });
    const published = await tx(o.scope, (c) => getVersionDocument(c, o.sf.id, 1));
    const edited = clone(o.sf.document);
    edited.pages[0].sections[0].settings.heading = { ro: "Titlu schimbat", en: "Changed title" };
    const r = await tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: o.sf.draft_revision, document: edited }));
    expect(r.revision).toBe(o.sf.draft_revision + 1);
    expect(await tx(o.scope, (c) => getVersionDocument(c, o.sf.id, 1))).toEqual(published);
    const sf = await tx(o.scope, (c) => getStorefront(c, o.sf.id));
    expect((sf!.document.pages[0].sections[0].settings.heading as { en: string }).en).toBe("Changed title");
    expect(sf!.live_number).toBe(1);
  });

  it("two tabs: a save based on an older revision is refused with the current revision, and the newer work stays", async () => {
    const o = await shopOrganiser();
    const a = clone(o.sf.document);
    a.pages[0].sections[0].settings.heading = { ro: "Fila A", en: "Tab A" };
    const b = clone(o.sf.document);
    b.pages[0].sections[0].settings.heading = { ro: "Fila B", en: "Tab B" };
    const first = await tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: 1, document: a }));
    expect(first.revision).toBe(2);
    await expect(tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: 1, document: b }))).rejects.toMatchObject({ code: "draft_conflict", currentRevision: 2 });
    const sf = await tx(o.scope, (c) => getStorefront(c, o.sf.id));
    expect((sf!.document.pages[0].sections[0].settings.heading as { en: string }).en).toBe("Tab A");
  });

  it("an invalid save is refused with the path of the first error and the stored draft is unchanged", async () => {
    const o = await shopOrganiser();
    const bad = clone(o.sf.document) as unknown as { pages: { sections: { type: string }[] }[] };
    bad.pages[0].sections[1].type = "unknown_section";
    await expect(tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: 1, document: bad }))).rejects.toMatchObject({ code: "unknown_type", field: "pages.0.sections.1.type" });
    const sf = await tx(o.scope, (c) => getStorefront(c, o.sf.id));
    expect(sf!.draft_revision).toBe(1);
    expect(sf!.document).toEqual(o.sf.document);
  });

  it("the database refuses any change to a published version's document, and its deletion", async () => {
    const o = await shopOrganiser();
    await tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: 1, userId: o.user.id }));
    const update = await tx(o.scope, (c) => c.query(`update control.storefront_versions set document = '{}'::jsonb where storefront_id = $1 and number = 1`, [o.sf.id])).catch((e) => e);
    expect(update.message).toMatch(/read-only/);
    const del = await tx(o.scope, (c) => c.query("delete from control.storefront_versions where storefront_id = $1 and number = 1", [o.sf.id])).catch((e) => e);
    expect(del.code).toBe("42501"); // the web role has no DELETE at all
    const renumber = await tx(o.scope, (c) => c.query("update control.storefront_versions set number = 9 where storefront_id = $1 and number = 1", [o.sf.id])).catch((e) => e);
    expect(renumber.code).toBe("42501"); // nor UPDATE of the number
    // Only one draft per storefront.
    const second = await tx(o.scope, (c) =>
      c.query(`insert into control.storefront_versions (organiser_id, storefront_id, state, document) values ($1, $2, 'draft', '{}'::jsonb)`, [o.organiserId, o.sf.id]),
    ).catch((e) => e);
    expect(second.code).toBe("23505");
  });

  it("rollback: with versions 1 to 3, restoring 1 publishes version 4 with 1's document, marked restored from 1", async () => {
    const o = await shopOrganiser();
    let rev = 1;
    const titles = ["One", "Two", "Three"];
    for (const t of titles) {
      const d = clone(o.sf.document);
      d.pages[0].sections[0].settings.heading = { ro: t, en: t };
      rev = (await tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: rev, document: d }))).revision;
      await tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: rev, userId: o.user.id }));
    }
    const v4 = await tx(o.scope, (c) => restoreVersion(c, { storefrontId: o.sf.id, number: 1, revision: rev, userId: o.user.id }));
    expect(v4).toMatchObject({ number: 4, restored_from: 1, state: "published" });
    const versions = await tx(o.scope, (c) => listVersions(c, o.sf.id));
    expect(versions.map((v) => [v.number, v.state])).toEqual([
      [4, "published"],
      [3, "archived"],
      [2, "archived"],
      [1, "archived"],
    ]);
    expect(await tx(o.scope, (c) => getVersionDocument(c, o.sf.id, 4))).toEqual(await tx(o.scope, (c) => getVersionDocument(c, o.sf.id, 1)));
    const sf = await tx(o.scope, (c) => getStorefront(c, o.sf.id));
    expect(sf!.live_number).toBe(4);
    expect((sf!.document.pages[0].sections[0].settings.heading as { en: string }).en).toBe("One"); // the draft follows
    expect(versions[0].author).toMatch(/^User/);
  });

  it("publish and restore are refused when the draft has moved on; a double publish of the current revision publishes once", async () => {
    const o = await shopOrganiser();
    const rev = (await tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: 1, document: clone(o.sf.document) }))).revision;
    await tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: rev, userId: null }));
    const rev2 = (await tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: rev, document: clone(o.sf.document) }))).revision;
    // Publishing the revision already published, now that the draft moved on, is a conflict.
    await expect(tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: rev, userId: null }))).rejects.toMatchObject({ code: "draft_conflict", currentRevision: rev2 });
    // A restore based on an older revision would overwrite newer work: refused.
    await expect(tx(o.scope, (c) => restoreVersion(c, { storefrontId: o.sf.id, number: 1, revision: rev, userId: null }))).rejects.toMatchObject({ code: "draft_conflict", currentRevision: rev2 });
    const ok = await tx(o.scope, (c) => restoreVersion(c, { storefrontId: o.sf.id, number: 1, revision: rev2, userId: null }));
    expect(ok.number).toBe(2);
  });
});

