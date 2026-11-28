// After a save or a publish: fetch and store the static map tiles the document's map sections need
// (server side, outside the save transaction; failures only mean the map shows its link alone).
import "server-only";
import { getAppPool } from "../db/pool";
import { withTx } from "../db/tx";
import type { ActiveStudio } from "../studio/context";
import { ensureTiles, mapTilesForDocument } from "./maps";

export async function storeMapTiles(s: ActiveStudio, document: unknown): Promise<void> {
  try {
    const pool = getAppPool();
    const doc = document as Parameters<typeof mapTilesForDocument>[1];
    if (!doc || !Array.isArray(doc.pages)) return;
    const tiles = await withTx(pool, { organiserId: s.active.id, userId: s.user.id }, (c) => mapTilesForDocument(c, doc));
    if (tiles.length) await withTx(pool, {}, (c) => ensureTiles(c, tiles, undefined, 3));
  } catch (err) {
    console.error("map tiles", (err as Error)?.message);
  }
}
