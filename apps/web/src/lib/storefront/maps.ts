// Static venue maps (decision on review of step 2): a fan's browser never requests
// tile.openstreetmap.org. When a draft is saved or published, the server fetches the one tile it
// needs (fixed zoom, identifying User-Agent and Referer, per the OSM tile usage policy), stores it in
// the platform table control.map_tiles and the shop shows it from /assets/maps/<z>/<x>/<y>.png with
// the © OpenStreetMap contributors attribution and a link out.
import type { ClientBase } from "pg";

export const MAP_ZOOM = 15;
const MAX_TILE_BYTES = 256 * 1024;
const TILE_HOST = "https://tile.openstreetmap.org";
const USER_AGENT = "EPS-ticketing-prototype/1.0 (+https://eps-prototype.vercel.app/terms)";
const REFERER = "https://eps-prototype.vercel.app/";

export type Tile = { z: number; x: number; y: number };

/** OpenStreetMap tile coordinates for a point (Web Mercator). */
export function tileFor(lat: number, lng: number, zoom = MAP_ZOOM): Tile {
  const n = 2 ** zoom;
  const rad = (lat * Math.PI) / 180;
  const x = Math.floor(((lng + 180) / 360) * n);
  const y = Math.floor(((1 - Math.log(Math.tan(rad) + 1 / Math.cos(rad)) / Math.PI) / 2) * n);
  return { z: zoom, x: Math.min(Math.max(x, 0), n - 1), y: Math.min(Math.max(y, 0), n - 1) };
}

export function tileUrl(t: Tile): string {
  return `/assets/maps/${t.z}/${t.x}/${t.y}.png`;
}

export function osmLink(lat: number, lng: number): string {
  return `https://www.openstreetmap.org/?mlat=${lat}&mlon=${lng}#map=${MAP_ZOOM}/${lat}/${lng}`;
}

export type TileFetcher = (t: Tile) => Promise<Uint8Array | null>;

const PNG_SIG = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];

/** Fetches one tile from OpenStreetMap; null on any failure (the map then shows the link only). */
export const fetchOsmTile: TileFetcher = async (t) => {
  if (process.env.EPS_MAP_TILES === "off") return null;
  try {
    const res = await fetch(`${TILE_HOST}/${t.z}/${t.x}/${t.y}.png`, {
      headers: { "User-Agent": USER_AGENT, Referer: REFERER },
      signal: AbortSignal.timeout(4000),
      cache: "no-store",
    });
    if (!res.ok) return null;
    const declared = Number(res.headers.get("content-length") ?? "0");
    if (declared > MAX_TILE_BYTES) return null;
    const bytes = new Uint8Array(await res.arrayBuffer());
    if (bytes.length > MAX_TILE_BYTES || !PNG_SIG.every((v, i) => bytes[i] === v)) return null;
    return bytes;
  } catch {
    return null;
  }
};

/** Which of the tiles are stored. */
export async function storedTiles(c: ClientBase, tiles: Tile[]): Promise<Set<string>> {
  if (!tiles.length) return new Set();
  const r = await c.query<{ k: string }>(
    `select z || '/' || x || '/' || y as k from control.map_tiles t
     join unnest($1::int[], $2::int[], $3::int[]) as q(z, x, y) using (z, x, y)`,
    [tiles.map((t) => t.z), tiles.map((t) => t.x), tiles.map((t) => t.y)],
  );
  return new Set(r.rows.map((x) => x.k));
}

/** Stores the tiles that are missing (fetching at most `limit` per call). */
export async function ensureTiles(c: ClientBase, tiles: Tile[], fetcher: TileFetcher = fetchOsmTile, limit = 10): Promise<number> {
  const unique = [...new Map(tiles.map((t) => [`${t.z}/${t.x}/${t.y}`, t])).values()];
  const have = await storedTiles(c, unique);
  let stored = 0;
  for (const t of unique.filter((t) => !have.has(`${t.z}/${t.x}/${t.y}`)).slice(0, limit)) {
    const bytes = await fetcher(t);
    if (!bytes) continue;
    await c.query("insert into control.map_tiles (z, x, y, bytes) values ($1, $2, $3, $4) on conflict do nothing", [t.z, t.x, t.y, Buffer.from(bytes)]);
    stored++;
  }
  return stored;
}

/** The venues (with coordinates) of the events that visible map sections of a document show. */
export async function mapTilesForDocument(c: ClientBase, doc: { pages: { sections: { type: string; hidden: boolean; settings: Record<string, unknown> }[] }[] }): Promise<Tile[]> {
  const ids = [
    ...new Set(
      doc.pages.flatMap((p) => p.sections.filter((s) => s.type === "map" && !s.hidden).map((s) => String(s.settings.eventId ?? ""))).filter(Boolean),
    ),
  ];
  if (!ids.length) return [];
  const r = await c.query<{ lat: number; lng: number }>(
    `select v.lat, v.lng from control.events e join control.venues v on v.id = e.venue_id
     where e.id = any($1::uuid[]) and v.lat is not null and v.lng is not null`,
    [ids],
  );
  return r.rows.map((v) => tileFor(v.lat, v.lng));
}

export async function readTile(c: ClientBase, t: Tile): Promise<Uint8Array | null> {
  return (await c.query<{ bytes: Buffer }>("select bytes from control.map_tiles where z = $1 and x = $2 and y = $3", [t.z, t.x, t.y])).rows[0]?.bytes ?? null;
}
