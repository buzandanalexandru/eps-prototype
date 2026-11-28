// Stored OpenStreetMap tiles for storefront map sections (served from our origin; © OpenStreetMap
// contributors, attributed on the section).
import { getAppPool } from "@/lib/db/pool";
import { withTx } from "@/lib/db/tx";
import { readTile } from "@/lib/storefront/maps";

export async function GET(_req: Request, { params }: { params: Promise<{ z: string; x: string; y: string }> }) {
  const p = await params;
  const m = /^(\d{1,7})\.png$/.exec(p.y);
  const z = Number(p.z);
  const x = Number(p.x);
  if (!m || !/^\d{1,2}$/.test(p.z) || !/^\d{1,7}$/.test(p.x) || z > 19) return new Response("Not found", { status: 404 });
  const bytes = await withTx(getAppPool(), {}, (c) => readTile(c, { z, x, y: Number(m[1]) }));
  if (!bytes) return new Response("Not found", { status: 404 });
  return new Response(new Uint8Array(bytes), {
    headers: { "Content-Type": "image/png", "Cache-Control": "public, max-age=86400", "X-Content-Type-Options": "nosniff" },
  });
}
