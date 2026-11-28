// The public shop's data (design D3.5, D3.6): a brand's live storefront version and the live sale
// snapshots of the events it sells, read only through the published-only functions
// public_storefront() and live_snapshot(). Two queries for any shop, no organiser scope, and
// nothing from drafts. The result is plain JSON, so it can be cached as is (shop-cache.ts).
import type { ClientBase } from "pg";
import type { Currency } from "../pricing/money";
import type { SaleSnapshot, SnapshotPriceStep, SnapshotProduct } from "../snapshot/types";

/** A live snapshot as public pages read it (control.public_snapshot): the shop never sees the organiser's side. */
export type PublicSnapshot = Omit<SaleSnapshot, "fee" | "products"> & {
  products: (Omit<SnapshotProduct, "priceSteps"> & { priceSteps: Omit<SnapshotPriceStep, "organiserNetMinor">[] })[];
};
import { storedTiles, tileFor, tileUrl } from "./maps";
import { LIMITS, parseDocument, referencedEvents, type Locale, type StorefrontDocument } from "./schema";

export type PublishedShop = {
  brandId: string;
  organiserId: string;
  slug: string;
  name: string;
  currency: Currency;
  languages: Locale[];
  organiserName: string;
  isSample: boolean;
  versionId: string;
  versionNumber: number;
  publishedAt: string;
  hostnames: string[];
  document: StorefrontDocument;
  /** Live snapshot per event the shop sells (only the brand's own events). */
  snapshots: Record<string, PublicSnapshot>;
  /** Stored static map tile per event venue, or null. */
  maps: Record<string, string | null>;
};

export type ShopKey = { slug: string } | { host: string };

const SLUG = /^[a-z0-9][a-z0-9-]{1,38}[a-z0-9]$/;
const HOST = /^[a-z0-9.-]{3,253}$/;

/** Normalises a lookup key; null when it can never name a shop (no query is made). */
export function normaliseKey(k: ShopKey): ShopKey | null {
  if ("slug" in k) return SLUG.test(k.slug) ? { slug: k.slug } : null;
  const host = k.host.toLowerCase().replace(/:\d+$/, "").replace(/\.$/, "");
  return HOST.test(host) ? { host } : null;
}

type Row = {
  brand_id: string;
  organiser_id: string;
  brand_slug: string;
  brand_name: string;
  currency: Currency;
  languages: Locale[];
  organiser_name: string;
  is_sample: boolean;
  version_id: string;
  version_number: number;
  published_at: Date;
  document: unknown;
  hostnames: string[];
};

export async function loadPublishedShop(c: ClientBase, key: ShopKey): Promise<PublishedShop | null> {
  const k = normaliseKey(key);
  if (!k) return null;
  const r = await c.query<Row>(
    `select brand_id::text, organiser_id::text, brand_slug, brand_name, currency, languages, organiser_name, is_sample,
            version_id::text, version_number, published_at, document, hostnames
       from control.public_storefront($1, $2)`,
    ["slug" in k ? k.slug : null, "host" in k ? k.host : null],
  );
  const row = r.rows[0];
  if (!row) return null;
  const parsed = parseDocument(row.document);
  if (!parsed.ok) return null; // a published version always validated; never render one that does not
  const doc = parsed.doc;
  const ids = referencedEvents(doc).slice(0, LIMITS.events);
  const snapshots: Record<string, PublicSnapshot> = {};
  if (ids.length) {
    // The public view of each live snapshot: no organiser net amounts, no fee settings and no
    // hidden products (public_snapshot); an event that is off sale returns nothing.
    const s = await c.query<{ event_id: string; organiser_id: string; document: PublicSnapshot }>(
      `select s.event_id::text, s.organiser_id::text, s.document from unnest($1::uuid[]) as e(id), lateral control.public_snapshot(e.id) s`,
      [ids],
    );
    for (const x of s.rows) {
      const snap = x.document;
      // Only the brand's own events appear in its shop.
      if (x.organiser_id === row.organiser_id && snap.organiserId === row.organiser_id && snap.brandId === row.brand_id && snap.eventId === x.event_id) snapshots[x.event_id] = snap;
    }
  }
  const points = Object.values(snapshots)
    .filter((s) => s.event.venue && s.event.venue.lat !== null && s.event.venue.lng !== null)
    .map((s) => [s.eventId, tileFor(s.event.venue!.lat!, s.event.venue!.lng!)] as const);
  const stored = await storedTiles(c, points.map(([, t]) => t));
  const maps: Record<string, string | null> = {};
  for (const [id, t] of points) maps[id] = stored.has(`${t.z}/${t.x}/${t.y}`) ? tileUrl(t) : null;
  return {
    brandId: row.brand_id,
    organiserId: row.organiser_id,
    slug: row.brand_slug,
    name: row.brand_name,
    currency: row.currency,
    languages: row.languages,
    organiserName: row.organiser_name,
    isSample: row.is_sample,
    versionId: row.version_id,
    versionNumber: row.version_number,
    publishedAt: new Date(row.published_at).toISOString(),
    hostnames: row.hostnames ?? [],
    document: doc,
    snapshots,
    maps,
  };
}
