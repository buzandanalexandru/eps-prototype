// Storefront builder spec "Unsafe or oversized upload" and the asset store.
import { describe, expect, it } from "vitest";
import { createAsset, readPublicAsset, sniffImage, uploadProblem } from "@/lib/storefront/assets";
import { assetResponse } from "@/lib/storefront/asset-response";
import { describeDb, makeOrganiser, tx } from "./db";

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
  it("stores an upload with a server id and serves it from /assets/<id> with immutable caching and nosniff", async () => {
    const o = await makeOrganiser();
    const a = await tx(o.scope, (c) => createAsset(c, PNG, o.user.id));
    expect(a).toMatchObject({ mime: "image/png", width: 1, height: 1 });
    const stored = await tx({}, (c) => readPublicAsset(c, a.id)); // no organiser scope: a fan's request
    const res = assetResponse(stored, null);
    expect(res.status).toBe(200);
    expect(res.headers.get("content-type")).toBe("image/png");
    expect(res.headers.get("cache-control")).toBe("public, max-age=31536000, immutable");
    expect(res.headers.get("x-content-type-options")).toBe("nosniff");
    expect(Buffer.from(await res.arrayBuffer()).equals(PNG)).toBe(true);
    expect(assetResponse(stored, `"${a.sha256}"`).status).toBe(304);
    expect(assetResponse(await tx({}, (c) => readPublicAsset(c, "00000000-0000-4000-8000-000000000000")), null).status).toBe(404);
    expect(await tx({}, (c) => readPublicAsset(c, "not-a-uuid"))).toBeNull();
  });

  it("refuses a bad upload before writing", async () => {
    const o = await makeOrganiser();
    await expect(tx(o.scope, (c) => createAsset(c, Buffer.from("<svg/>"), null))).rejects.toMatchObject({ code: "svg_refused" });
  });
});
