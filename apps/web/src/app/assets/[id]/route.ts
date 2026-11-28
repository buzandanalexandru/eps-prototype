// Uploaded images (design D2.1), any organiser's: fan pages show them. Read through public_asset().
import { getAppPool } from "@/lib/db/pool";
import { withTx } from "@/lib/db/tx";
import { readPublicAsset } from "@/lib/storefront/assets";
import { assetResponse } from "@/lib/storefront/asset-response";

export async function GET(req: Request, { params }: { params: Promise<{ id: string }> }) {
  const { id } = await params;
  const a = await withTx(getAppPool(), {}, (c) => readPublicAsset(c, id));
  return assetResponse(a, req.headers.get("if-none-match"));
}
