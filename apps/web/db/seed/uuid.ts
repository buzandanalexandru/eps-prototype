// Stable ids for seeded rows: UUID v5 (SHA-1, RFC 9562) of a name in the EPS seed namespace, so every
// environment gets the same ids and re-running the seed updates rows instead of adding new ones.
import { createHash } from "node:crypto";

export const SEED_NAMESPACE = "6f6c9d0e-6a0b-4b8e-9f1d-3c2a5e7b8d10";

export function uuidv5(name: string, namespace = SEED_NAMESPACE): string {
  const ns = Buffer.from(namespace.replaceAll("-", ""), "hex");
  const h = createHash("sha1").update(Buffer.concat([ns, Buffer.from(name, "utf8")])).digest();
  h[6] = (h[6] & 0x0f) | 0x50;
  h[8] = (h[8] & 0x3f) | 0x80;
  const x = h.subarray(0, 16).toString("hex");
  return `${x.slice(0, 8)}-${x.slice(8, 12)}-${x.slice(12, 16)}-${x.slice(16, 20)}-${x.slice(20)}`;
}

/** Ids scoped to one sample organiser: sid("nibiru", "product", "galaxia-ga"). */
export function sid(org: string, kind: string, key: string): string {
  return uuidv5(`${org}/${kind}/${key}`);
}
