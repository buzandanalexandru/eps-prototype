// Image uploads (design D2.1): PNG, JPEG or WebP only, recognised from their bytes (never from the
// file name or the client's content type), at most 1 MB, with their pixel size read from the header.
// SVG and everything else is refused: an SVG can carry script.
import { createHash } from "node:crypto";
import type { ClientBase } from "pg";
import { DomainError } from "../errors";

export const MAX_ASSET_BYTES = 1024 * 1024;
export type ImageMime = "image/png" | "image/jpeg" | "image/webp";
export type Sniffed = { mime: ImageMime; width: number; height: number };

const u16be = (b: Uint8Array, i: number) => (b[i] << 8) | b[i + 1];
const u16le = (b: Uint8Array, i: number) => b[i] | (b[i + 1] << 8);
const u24le = (b: Uint8Array, i: number) => b[i] | (b[i + 1] << 8) | (b[i + 2] << 16);
const u32be = (b: Uint8Array, i: number) => ((b[i] << 24) >>> 0) + (b[i + 1] << 16) + (b[i + 2] << 8) + b[i + 3];
const ascii = (b: Uint8Array, i: number, n: number) => String.fromCharCode(...b.subarray(i, i + n));

function png(b: Uint8Array): Sniffed | null {
  const sig = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a];
  if (b.length < 24 || !sig.every((v, i) => b[i] === v) || ascii(b, 12, 4) !== "IHDR") return null;
  return { mime: "image/png", width: u32be(b, 16), height: u32be(b, 20) };
}

function jpeg(b: Uint8Array): Sniffed | null {
  if (b.length < 4 || b[0] !== 0xff || b[1] !== 0xd8 || b[2] !== 0xff) return null;
  let i = 2;
  while (i + 9 < b.length) {
    if (b[i] !== 0xff) return null;
    const marker = b[i + 1];
    if (marker === 0xd8 || (marker >= 0xd0 && marker <= 0xd7) || marker === 0x01) {
      i += 2;
      continue;
    }
    const len = u16be(b, i + 2);
    // Start-of-frame markers (baseline, progressive, ...), not DHT (c4), JPG (c8) or DAC (cc).
    if (marker >= 0xc0 && marker <= 0xcf && marker !== 0xc4 && marker !== 0xc8 && marker !== 0xcc) {
      return { mime: "image/jpeg", height: u16be(b, i + 5), width: u16be(b, i + 7) };
    }
    if (len < 2) return null;
    i += 2 + len;
  }
  return null;
}

function webp(b: Uint8Array): Sniffed | null {
  if (b.length < 30 || ascii(b, 0, 4) !== "RIFF" || ascii(b, 8, 4) !== "WEBP") return null;
  const chunk = ascii(b, 12, 4);
  if (chunk === "VP8 " && b[23] === 0x9d && b[24] === 0x01 && b[25] === 0x2a) {
    return { mime: "image/webp", width: u16le(b, 26) & 0x3fff, height: u16le(b, 28) & 0x3fff };
  }
  if (chunk === "VP8L" && b[20] === 0x2f) {
    const bits = b[21] | (b[22] << 8) | (b[23] << 16) | (b[24] << 24);
    return { mime: "image/webp", width: (bits & 0x3fff) + 1, height: ((bits >> 14) & 0x3fff) + 1 };
  }
  if (chunk === "VP8X") return { mime: "image/webp", width: u24le(b, 24) + 1, height: u24le(b, 27) + 1 };
  return null;
}

/** The image type and size read from the bytes; null for anything that is not PNG, JPEG or WebP. */
export function sniffImage(bytes: Uint8Array): Sniffed | null {
  const s = png(bytes) ?? jpeg(bytes) ?? webp(bytes);
  if (!s || s.width < 1 || s.height < 1 || s.width > 10000 || s.height > 10000) return null;
  return s;
}

/** The refusal reason for an upload, or null when it is acceptable. */
export function uploadProblem(bytes: Uint8Array): "file_too_large" | "empty" | "svg_refused" | "not_an_image" | null {
  if (bytes.length === 0) return "empty";
  if (bytes.length > MAX_ASSET_BYTES) return "file_too_large";
  if (sniffImage(bytes)) return null;
  const head = new TextDecoder("utf-8", { fatal: false }).decode(bytes.subarray(0, 512)).toLowerCase();
  if (head.includes("<svg") || head.includes("<?xml")) return "svg_refused";
  return "not_an_image";
}

export type AssetRow = { id: string; mime: ImageMime; width: number; height: number; sha256: string };

/** Per-organiser storage cap and per-user upload rate (review decision on step 2). */
export const ASSET_LIMITS = { files: 200, bytes: 100 * 1024 * 1024, uploadsPerUser: 30, windowMinutes: 10 } as const;

const ORG = "nullif(current_setting('app.organiser_id', true), '')::uuid";

/**
 * Stores an upload for the transaction's organiser (server-generated id). The same image twice is
 * stored once (deduplicated by SHA-256); the organiser's cap and the user's upload rate are checked
 * under a per-organiser lock inside this transaction.
 */
export async function createAsset(c: ClientBase, bytes: Uint8Array, userId: string | null): Promise<AssetRow> {
  const problem = uploadProblem(bytes);
  if (problem) throw new DomainError(problem, "file");
  const s = sniffImage(bytes)!;
  const sha256 = createHash("sha256").update(bytes).digest("hex");
  await c.query(`select pg_advisory_xact_lock(hashtext('assets:' || ${ORG}::text))`);
  const same = await c.query<AssetRow>("select id, mime, width, height, sha256 from control.assets where sha256 = $1", [sha256]);
  if (same.rows[0]) return same.rows[0];
  if (userId) {
    const recent = await c.query<{ n: number }>(
      "select count(*)::int as n from control.assets where created_by = $1 and created_at > now() - make_interval(mins => $2)",
      [userId, ASSET_LIMITS.windowMinutes],
    );
    if (recent.rows[0].n >= ASSET_LIMITS.uploadsPerUser) throw new DomainError("upload_rate", "file", { minutes: ASSET_LIMITS.windowMinutes });
  }
  const used = await c.query<{ n: number; b: string }>("select count(*)::int as n, coalesce(sum(octet_length(bytes)), 0)::text as b from control.assets");
  if (used.rows[0].n >= ASSET_LIMITS.files || Number(used.rows[0].b) + bytes.length > ASSET_LIMITS.bytes) {
    throw new DomainError("asset_quota", "file", { files: ASSET_LIMITS.files, mb: ASSET_LIMITS.bytes / 1024 / 1024 });
  }
  const r = await c.query<AssetRow>(
    `insert into control.assets (organiser_id, mime, bytes, sha256, width, height, created_by)
     values (${ORG}, $1, $2, $3, $4, $5, $6)
     returning id, mime, width, height, sha256`,
    [s.mime, Buffer.from(bytes), sha256, s.width, s.height, userId],
  );
  return r.rows[0];
}

export type AssetListRow = { id: string; mime: ImageMime; width: number; height: number; size: number; created_at: Date };

/** The organiser's uploads, newest first (bounded by the cap). */
export async function listAssets(c: ClientBase): Promise<AssetListRow[]> {
  return (
    await c.query<AssetListRow>(
      `select id, mime, width, height, octet_length(bytes) as size, created_at from control.assets order by created_at desc, id limit ${ASSET_LIMITS.files}`,
    )
  ).rows;
}

/** Deletes an upload that no storefront version (draft, published or archived) refers to. */
export async function deleteAsset(c: ClientBase, id: string): Promise<void> {
  if (!UUID.test(id)) throw new DomainError("not_found");
  const used = await c.query("select 1 from control.storefront_versions where strpos(document::text, $1) > 0 limit 1", [`asset:${id}`]);
  if (used.rowCount) throw new DomainError("asset_in_use");
  const r = await c.query("delete from control.assets where id = $1", [id]);
  if (!r.rowCount) throw new DomainError("not_found");
}

/** Which of the given asset ids belong to the transaction's organiser. */
export async function ownedAssets(c: ClientBase, ids: string[]): Promise<Set<string>> {
  if (!ids.length) return new Set();
  const r = await c.query<{ id: string }>("select id::text from control.assets where id = any($1::uuid[])", [ids]);
  return new Set(r.rows.map((x) => x.id));
}

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/;

export type ServedAsset = { mime: ImageMime; bytes: Buffer; sha256: string; scope: "live" | "preview" | "member" };

/**
 * The bytes behind /assets/<id> for a visitor: only while a live storefront version of the asset's
 * organiser shows it, or through a valid preview link (its token's hash) whose draft shows it.
 */
export async function readPublicAsset(c: ClientBase, id: string, tokenHash: string | null = null): Promise<ServedAsset | null> {
  if (!UUID.test(id)) return null;
  return (await c.query("select mime, bytes, sha256, scope from control.public_asset($1::uuid, $2)", [id, tokenHash])).rows[0] ?? null;
}

/** For Studio members: an asset of the transaction's organiser (row-level security). */
export async function readOwnAsset(c: ClientBase, id: string): Promise<ServedAsset | null> {
  if (!UUID.test(id)) return null;
  const r = (await c.query("select mime, bytes, sha256 from control.assets where id = $1", [id])).rows[0];
  return r ? { ...r, scope: "member" } : null;
}
