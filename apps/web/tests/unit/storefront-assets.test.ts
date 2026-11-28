// Storefront builder spec "Unsafe or oversized upload" and the asset store.
import { describe, expect, it } from "vitest";
import { ASSET_LIMITS, createAsset, deleteAsset, listAssets, readOwnAsset, readPublicAsset, sniffImage, uploadProblem } from "@/lib/storefront/assets";
import { createPreviewLink, hashPreviewToken, publishDraft, saveDraft } from "@/lib/storefront/domain";
import { clone, shopOrganiser } from "./storefront-fixtures";
import { assetResponse } from "@/lib/storefront/asset-response";
import { asOwner, describeDb, makeOrganiser, tx } from "./db";

import { PNG } from "./storefront-assets.fixtures";

/** A minimal JPEG header with a 2×3 frame, and a WebP (VP8X) header. */
const JPEG = Buffer.from([0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xff, 0xc0, 0x00, 0x11, 0x08, 0x00, 0x03, 0x00, 0x02, 0x03, 0x01, 0x22, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01, 0xff, 0xd9]);
function webpVp8x(w: number, h: number) {
  const b = Buffer.alloc(30);
  b.write("RIFF", 0);
  b.writeUInt32LE(22, 4);
  b.write("WEBP", 8);
  b.write("VP8X", 12);
  b.writeUIntLE(w - 1, 24, 3);
  b.writeUIntLE(h - 1, 27, 3);
  return b;
}

describe("image sniffing", () => {
  it("recognises PNG, JPEG and WebP from their bytes, with their size", () => {
    expect(sniffImage(PNG)).toEqual({ mime: "image/png", width: 1, height: 1 });
    expect(sniffImage(JPEG)).toEqual({ mime: "image/jpeg", width: 2, height: 3 });
    expect(sniffImage(webpVp8x(640, 480))).toEqual({ mime: "image/webp", width: 640, height: 480 });
  });

  it("refuses SVG, other content and files over 1 MB, with the reason", () => {
    expect(uploadProblem(Buffer.from('<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>'))).toBe("svg_refused");
    expect(uploadProblem(Buffer.from('<?xml version="1.0"?><svg/>'))).toBe("svg_refused");
    expect(uploadProblem(Buffer.from("GIF89a....."))).toBe("not_an_image");
    expect(uploadProblem(Buffer.from("<html><script>alert(1)</script></html>"))).toBe("not_an_image");
    // A PNG signature in front of HTML is still not a PNG (no IHDR).
    expect(uploadProblem(Buffer.concat([PNG.subarray(0, 8), Buffer.from("<script>")]))).toBe("not_an_image");
    const big = Buffer.concat([PNG, Buffer.alloc(1024 * 1024)]);
    expect(uploadProblem(big)).toBe("file_too_large");
    expect(uploadProblem(Buffer.alloc(0))).toBe("empty");
    expect(uploadProblem(PNG)).toBeNull();
  });
});

describeDb("asset store", () => {
  it("serves an upload only while a live version shows it or through a valid preview link; members see their own; a day of public caching", async () => {
    const o = await shopOrganiser();
    const a = await tx(o.scope, (c) => createAsset(c, PNG, o.user.id));
    expect(a).toMatchObject({ mime: "image/png", width: 1, height: 1 });
    // Not referenced anywhere: not public, but the organiser's members see it.
    expect(await tx({}, (c) => readPublicAsset(c, a.id))).toBeNull();
    expect(await tx(o.scope, (c) => readOwnAsset(c, a.id))).toMatchObject({ scope: "member" });
    const other = await shopOrganiser();
    expect(await tx(other.scope, (c) => readOwnAsset(c, a.id))).toBeNull();
    // In the draft, through a preview link only.
    const d = clone(o.sf.document);
    d.tokens.logo = `asset:${a.id}`;
    const rev = (await tx(o.scope, (c) => saveDraft(c, o.sf.id, { revision: 1, document: d }))).revision;
    expect(await tx({}, (c) => readPublicAsset(c, a.id))).toBeNull();
    const link = await tx(o.scope, (c) => createPreviewLink(c, o.sf.id, o.user.id));
    const viaLink = await tx({}, (c) => readPublicAsset(c, a.id, hashPreviewToken(link.token)));
    expect(viaLink?.scope).toBe("preview");
    expect(assetResponse(viaLink, null).headers.get("cache-control")).toMatch(/^private/);
    // Published: public, cached for a day (not immutable).
    await tx(o.scope, (c) => publishDraft(c, { storefrontId: o.sf.id, revision: rev, userId: o.user.id }));
    const live = await tx({}, (c) => readPublicAsset(c, a.id));
    expect(live?.scope).toBe("live");
    const res = assetResponse(live, null);
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("image/png");
    expect(res.headers.get("cache-control")).toBe("public, max-age=86400");
    expect(res.headers.get("x-content-type-options")).toBe("nosniff");
    expect(Buffer.from(await res.arrayBuffer()).equals(PNG)).toBe(true);
    expect(assetResponse(live, `"${a.sha256}"`).status).toBe(304);
    expect(assetResponse(await tx({}, (c) => readPublicAsset(c, "00000000-0000-4000-8000-000000000000")), null).status).toBe(404);
    expect(await tx({}, (c) => readPublicAsset(c, "not-a-uuid"))).toBeNull();
    // In use: cannot be deleted.
    await expect(tx(o.scope, (c) => deleteAsset(c, a.id))).rejects.toMatchObject({ code: "asset_in_use" });
  });

  it("dedupes the same image, deletes an unused one, and enforces the per-user rate and the organiser cap", async () => {
    const o = await shopOrganiser();
    const a = await tx(o.scope, (c) => createAsset(c, PNG, o.user.id));
    const b = await tx(o.scope, (c) => createAsset(c, PNG, o.user.id));
    expect(b.id).toBe(a.id);
    expect(await tx(o.scope, (c) => listAssets(c))).toHaveLength(1);
    await tx(o.scope, (c) => deleteAsset(c, a.id));
    expect(await tx(o.scope, (c) => listAssets(c))).toHaveLength(0);
    // Rate: ASSET_LIMITS.uploadsPerUser distinct images inside the window.
    const pngOf = (i: number) => { const p = Buffer.from(PNG); return Buffer.concat([p, Buffer.from(`pad${i}`)]); };
    for (let i = 0; i < ASSET_LIMITS.uploadsPerUser; i++) await tx(o.scope, (c) => createAsset(c, pngOf(i), o.user.id));
    await expect(tx(o.scope, (c) => createAsset(c, pngOf(999), o.user.id))).rejects.toMatchObject({ code: "upload_rate" });
    // Cap: count over the organiser (rows made old and authorless so the rate does not apply).
    await asOwner((c) => c.query("update control.assets set created_at = now() - interval '1 day' where organiser_id = $1", [o.organiserId]));
    await asOwner((c) =>
      c.query(
        `insert into control.assets (organiser_id, mime, bytes, sha256, width, height)
         select $1, 'image/png', $2, md5(g::text) || md5(g::text), 1, 1 from generate_series(1, $3) g`,
        [o.organiserId, PNG, ASSET_LIMITS.files - ASSET_LIMITS.uploadsPerUser],
      ),
    );
    await expect(tx(o.scope, (c) => createAsset(c, pngOf(1000), o.user.id))).rejects.toMatchObject({ code: "asset_quota" });
  });

  it("refuses a bad upload before writing", async () => {
    const o = await makeOrganiser();
    await expect(tx(o.scope, (c) => createAsset(c, Buffer.from("<svg/>"), null))).rejects.toMatchObject({ code: "svg_refused" });
  });
});
