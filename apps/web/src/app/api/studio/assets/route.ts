// Image upload (design D2.1): multipart field "file"; the bytes decide the type (PNG, JPEG, WebP),
// at most 1 MB; SVG and anything else is refused with the reason.
import { NextResponse } from "next/server";
import { DomainError, fromPgError } from "@/lib/errors";
import { assertCanDesign, sameOrigin } from "@/lib/storefront/access";
import { createAsset, MAX_ASSET_BYTES } from "@/lib/storefront/assets";
import { requireActive, studioTx } from "@/lib/studio/context";

export async function POST(req: Request) {
  const headers = { "Cache-Control": "private, no-store" };
  if (!sameOrigin(req)) return NextResponse.json({ code: "forbidden" }, { status: 403, headers });
  const declared = Number(req.headers.get("content-length") ?? "0");
  if (declared > MAX_ASSET_BYTES + 64 * 1024) return NextResponse.json({ code: "file_too_large" }, { status: 413, headers });
  try {
    const s = await requireActive();
    assertCanDesign(s.active.role);
    const form = await req.formData();
    const file = form.get("file");
    if (!(file instanceof Blob)) throw new DomainError("not_an_image", "file");
    if (file.size > MAX_ASSET_BYTES) throw new DomainError("file_too_large", "file");
    const bytes = new Uint8Array(await file.arrayBuffer());
    const a = await studioTx(s, (c) => createAsset(c, bytes, s.user.id));
    return NextResponse.json({ id: a.id, ref: `asset:${a.id}`, url: `/assets/${a.id}`, width: a.width, height: a.height }, { status: 201, headers });
  } catch (err) {
    const d = err instanceof DomainError ? err : fromPgError(err);
    if (d?.code === "signed_out" || d?.code === "no_organiser") return NextResponse.json({ code: d.code }, { status: 401, headers });
    if (d?.code === "designer_only") return NextResponse.json({ code: d.code }, { status: 403, headers });
    if (d) return NextResponse.json({ code: d.code }, { status: 422, headers });
    console.error("asset upload failed", (err as Error)?.message);
    return NextResponse.json({ code: "unexpected" }, { status: 500, headers });
  }
}
