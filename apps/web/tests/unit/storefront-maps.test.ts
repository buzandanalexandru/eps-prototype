// Map sections never make the fan's browser call a map service: the server stores the tile.
import { expect, it } from "vitest";
import { loadShopCatalogue } from "@/lib/storefront/catalogue-load";
import { ensureTiles, mapTilesForDocument, MAP_ZOOM, readTile, tileFor, tileUrl } from "@/lib/storefront/maps";
import { describeDb, tx } from "./db";
import { doc } from "./storefront-fixtures";
import { render } from "./storefront-render";
import { shopOrganiser } from "./storefront-fixtures";

const TILE = Buffer.from("89504e470d0a1a0a0000000d4948445200000001000000010806000000", "hex");

it("tile coordinates at the fixed zoom; the rendered shop holds no third-party map URL", () => {
  expect(tileFor(43.93, 28.63)).toEqual({ z: MAP_ZOOM, x: 18989, y: 11923 });
  expect(tileUrl({ z: 15, x: 1, y: 2 })).toBe("/assets/maps/15/1/2.png");
  const html = render(doc());
  expect(html.includes("tile.openstreetmap.org")).toBe(false);
  expect(html.includes('src="/assets/maps/')).toBe(true);
  expect(html.includes("OpenStreetMap")).toBe(true);
});

describeDb("map tiles", () => {
  it("are fetched once by the server for the venues of visible map sections, then served from our origin", async () => {
    const o = await shopOrganiser();
    // A random point, so the tile is not stored from an earlier run.
    const lat = 40 + Math.random() * 8;
    const lng = 20 + Math.random() * 8;
    await tx(o.scope, (c) => c.query("update control.venues set lat = $2, lng = $3 where id = $1", [o.f.venue.id, lat, lng]));
    const tiles = await tx(o.scope, (c) => mapTilesForDocument(c, o.sf.document));
    expect(tiles).toEqual([tileFor(lat, lng)]);
    expect((await tx(o.scope, (c) => loadShopCatalogue(c, [o.f.ev.id])))[o.f.ev.id].venue?.map).toBeNull();
    let calls = 0;
    const fake = async () => (calls++, new Uint8Array(TILE));
    await tx({}, (c) => ensureTiles(c, tiles, fake));
    await tx({}, (c) => ensureTiles(c, tiles, fake));
    expect(calls).toBe(1);
    expect(Buffer.from((await tx({}, (c) => readTile(c, tiles[0])))!).equals(TILE)).toBe(true);
    expect((await tx(o.scope, (c) => loadShopCatalogue(c, [o.f.ev.id])))[o.f.ev.id].venue?.map).toBe(tileUrl(tiles[0]));
  });
});
