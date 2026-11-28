// The HTTP response for /assets/<id> (design D2.1): immutable caching, the sniffed type and headers
// that stop any browser from treating the bytes as anything but an image.
import type { ImageMime } from "./assets";

export function assetResponse(a: { mime: ImageMime; bytes: Uint8Array; sha256: string } | null, ifNoneMatch: string | null): Response {
  if (!a) return new Response("Not found", { status: 404, headers: { "Cache-Control": "public, max-age=60", "Content-Type": "text/plain; charset=utf-8" } });
  const etag = `"${a.sha256}"`;
  const headers = {
    "Content-Type": a.mime,
    "Cache-Control": "public, max-age=31536000, immutable",
    ETag: etag,
    "X-Content-Type-Options": "nosniff",
    "Content-Security-Policy": "default-src 'none'; sandbox",
    "Content-Disposition": "inline",
    "Cross-Origin-Resource-Policy": "same-site",
  };
  if (ifNoneMatch === etag) return new Response(null, { status: 304, headers });
  return new Response(new Uint8Array(a.bytes), { status: 200, headers });
}
