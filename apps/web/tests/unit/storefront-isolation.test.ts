// Storefront builder spec "Storefronts stay with their organiser".
import { expect, it } from "vitest";
import { createAsset, ownedAssets } from "@/lib/storefront/assets";
import { createPreviewLink, getStorefront, getStorefrontByBrand, listPreviewLinks, listVersions, publishDraft, restoreVersion, saveDraft, validateForSave } from "@/lib/storefront/domain";
import { describeDb, tx } from "./db";
import { clone, shopOrganiser } from "./storefront-fixtures";
import { PNG } from "./storefront-assets.fixtures";

describeDb("storefront isolation", () => {
  it("a member of organiser A gets not found for organiser B's draft, versions, links and uploads", async () => {
    const a = await shopOrganiser();
    const b = await shopOrganiser();
    await tx(b.scope, (c) => publishDraft(c, { storefrontId: b.sf.id, revision: 1, userId: b.user.id }));
    const link = await tx(b.scope, (c) => createPreviewLink(c, b.sf.id, b.user.id));
    const asset = await tx(b.scope, (c) => createAsset(c, PNG, b.user.id));
    expect(await tx(a.scope, (c) => getStorefront(c, b.sf.id))).toBeNull();
    expect(await tx(a.scope, (c) => getStorefrontByBrand(c, b.brandId))).toBeNull();
    expect(await tx(a.scope, (c) => listVersions(c, b.sf.id))).toEqual([]);
    expect(await tx(a.scope, (c) => listPreviewLinks(c, b.sf.id))).toEqual([]);
    expect(await tx(a.scope, (c) => ownedAssets(c, [asset.id]))).toEqual(new Set());
    await expect(tx(a.scope, (c) => saveDraft(c, b.sf.id, { revision: 1, document: a.sf.document }))).rejects.toMatchObject({ code: "not_found" });
    await expect(tx(a.scope, (c) => publishDraft(c, { storefrontId: b.sf.id, revision: 1, userId: a.user.id }))).rejects.toMatchObject({ code: "not_found" });
    await expect(tx(a.scope, (c) => restoreVersion(c, { storefrontId: b.sf.id, number: 1, revision: 1, userId: a.user.id }))).rejects.toMatchObject({ code: "not_found" });
    await expect(tx(a.scope, (c) => createPreviewLink(c, b.sf.id, a.user.id))).rejects.toMatchObject({ code: "not_found" });
    // Raw reads in A's scope see nothing of B.
    for (const t of ["storefronts", "storefront_versions", "preview_links", "assets"]) {
      const n = await tx(a.scope, async (c) => (await c.query(`select count(*)::int as n from control.${t} where organiser_id = $1`, [b.organiserId])).rows[0].n);
      expect(n, t).toBe(0);
    }
    expect(link.token).toBeTruthy();
  });

  it("A cannot put B's event or B's upload in its own document, nor write a row for B", async () => {
    const a = await shopOrganiser();
    const b = await shopOrganiser();
    const asset = await tx(b.scope, (c) => createAsset(c, PNG, b.user.id));
    const withEvent = clone(a.sf.document);
    withEvent.pages[0].sections.find((s) => s.type === "ticket_grid")!.settings.eventId = b.f.ev.id;
    await expect(tx(a.scope, (c) => validateForSave(c, a.brandId, withEvent))).rejects.toMatchObject({ code: "unknown_event" });
    const withImage = clone(a.sf.document);
    withImage.tokens.logo = `asset:${asset.id}`;
    await expect(tx(a.scope, (c) => validateForSave(c, a.brandId, withImage))).rejects.toMatchObject({ code: "unknown_image" });
    const raw = await tx(a.scope, (c) =>
      c.query(`insert into control.storefront_versions (organiser_id, storefront_id, state, document) values ($1, $2, 'draft', '{}'::jsonb)`, [b.organiserId, b.sf.id]),
    ).catch((e) => e);
    expect(raw.code).toBe("42501");
    // A's own storefront cannot point its live version at B's version (composite foreign key).
    const v = await tx(b.scope, (c) => publishDraft(c, { storefrontId: b.sf.id, revision: 1, userId: b.user.id }));
    const fk = await tx(a.scope, (c) => c.query("update control.storefronts set live_version_id = $1 where id = $2", [v.id, a.sf.id])).catch((e) => e);
    expect(fk.code).toBe("23503");
  });
});
