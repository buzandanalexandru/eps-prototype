// The HTTP response for /assets/<id> (design D2.1): a day of caching for live shops, the sniffed type and headers
// that stop any browser from treating the bytes as anything but an image.
import type { ServedAsset } from "./assets";

export function assetResponse(a: Pick<ServedAsset, "mime" | "bytes" | "sha256" | "scope"> | null, ifNoneMatch: string | null): Response {
  if (!a) return new Response("Not found", { status: 404, headers: { "Cache-Control": "public, max-age=60", "Content-Type": "text/plain; charset=utf-8" } });
  const etag = `"${a.sha256}"`;
  const headers = {
    "Content-Type": a.mime,
    // A day for images on a live shop; private for previews and Studio (they may stop being public).
    "Cache-Control": a.scope === "live" ? "public, max-age=86400" : "private, max-age=300",
    ETag: etag,
    "X-Content-Type-Options": "nosniff",
    "Content-Security-Policy": "default-src 'none'; sandbox",
    "Content-Disposition": "inline",
    "Cross-Origin-Resource-Policy": "same-site",
  };
  if (ifNoneMatch === etag) return new Response(null, { status: 304, headers });
  return new Response(new Uint8Array(a.bytes), { status: 200, headers });
}
