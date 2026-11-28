// Uploaded images (design D2.1). Public only while a live storefront version shows the image, or
// through a valid preview link (?p=<token>) whose draft shows it; Studio members also see their own
// organiser's uploads (the builder and its frame).
import { getAppPool } from "@/lib/db/pool";
import { withTx } from "@/lib/db/tx";
import { readOwnAsset, readPublicAsset } from "@/lib/storefront/assets";
import { assetResponse } from "@/lib/storefront/asset-response";
import { hashPreviewToken } from "@/lib/storefront/domain";
import { getStudio } from "@/lib/studio/context";

export async function GET(req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const token = new URL(req.url).searchParams.get("p");
  const hash = token && /^[A-Za-z0-9_-]{22}$/.test(token) ? hashPreviewToken(token) : null;
  const pool = getAppPool();
  let a = await withTx(pool, {}, (c) => readPublicAsset(c, id, hash));
  if (!a) {
    const s = await getStudio();
    if (s?.active) a = await withTx(pool, { organiserId: s.active.id, userId: s.user.id }, (c) => readOwnAsset(c, id));
  }
  return assetResponse(a, req.headers.get("if-none-match"));
}
