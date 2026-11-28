// Ticket entry codes (step 7, design D2): "E1." + base64url of 104 bytes: version, key id, event
// id, ticket id, generation (uint16), window = floor(unix seconds / 15) (uint32), then an Ed25519
// signature over the first 40 bytes. Shared by the fan's live ticket and the door's offline check.
// No personal data is ever in a code.
import { ed25519Verify } from "./ed25519";

export const CODE_PREFIX = "E1.";
export const WINDOW_SECONDS = 15;
const BODY = 40;
const TOKEN = 104;

export type TicketToken = { keyId: number; eventId: string; ticketId: string; generation: number; window: number };

export function windowOf(ms: number): number {
  return Math.floor(ms / 1000 / WINDOW_SECONDS);
}

export function fromBase64Url(s: string): Uint8Array | null {
  if (!/^[A-Za-z0-9_-]*$/.test(s)) return null;
  const b64 = s.replace(/-/g, "+").replace(/_/g, "/") + "===".slice((s.length + 3) % 4);
  try {
    const bin = atob(b64);
    const out = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) out[i] = bin.charCodeAt(i);
    return out;
  } catch {
    return null;
  }
}

function uuidOf(b: Uint8Array): string {
  const h = Array.from(b, (x) => x.toString(16).padStart(2, "0")).join("");
  return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
}

/** Parses a code without checking its signature; null when it is not a well-formed code. */
export function decodeCode(code: string): { token: TicketToken; body: Uint8Array; sig: Uint8Array } | null {
  if (typeof code !== "string" || !code.startsWith(CODE_PREFIX) || code.length > 200) return null;
  const b = fromBase64Url(code.slice(CODE_PREFIX.length));
  if (!b || b.length !== TOKEN || b[0] !== 1) return null;
  const dv = new DataView(b.buffer, b.byteOffset, b.byteLength);
  return {
    token: { keyId: b[1], eventId: uuidOf(b.slice(2, 18)), ticketId: uuidOf(b.slice(18, 34)), generation: dv.getUint16(34), window: dv.getUint32(36) },
    body: b.slice(0, BODY),
    sig: b.slice(BODY),
  };
}

const imported = new Map<string, Promise<CryptoKey | null>>();

function importKey(pub: Uint8Array): Promise<CryptoKey | null> {
  const k = Array.from(pub).join(",");
  let p = imported.get(k);
  if (!p) {
    p = crypto.subtle.importKey("raw", pub as BufferSource, { name: "Ed25519" }, false, ["verify"]).catch(() => null);
    imported.set(k, p);
  }
  return p;
}

/** Verifies an Ed25519 signature: WebCrypto where it supports Ed25519, else the small fallback. */
export async function verifySignature(pub: Uint8Array, body: Uint8Array, sig: Uint8Array, opts: { fallbackOnly?: boolean } = {}): Promise<boolean> {
  if (!opts.fallbackOnly) {
    const key = await importKey(pub);
    if (key) {
      try {
        return await crypto.subtle.verify({ name: "Ed25519" }, key, sig as BufferSource, body as BufferSource);
      } catch {
        // fall through to the library path
      }
    }
  }
  return ed25519Verify(pub, body, sig);
}

export type VerifyResult = { ok: true; token: TicketToken } | { ok: false; reason: "malformed" | "unknown_key" | "signature" };

/** Decodes a code and checks its signature with the public key of its key id. */
export async function verifyCode(code: string, keys: Map<number, Uint8Array>, opts: { fallbackOnly?: boolean } = {}): Promise<VerifyResult> {
  const d = decodeCode(code);
  if (!d) return { ok: false, reason: "malformed" };
  const pub = keys.get(d.token.keyId);
  if (!pub) return { ok: false, reason: "unknown_key" };
  return (await verifySignature(pub, d.body, d.sig, opts)) ? { ok: true, token: d.token } : { ok: false, reason: "signature" };
}
