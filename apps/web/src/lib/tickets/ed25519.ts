// A small Ed25519 signature verifier (RFC 8032, verification only) for browsers whose WebCrypto
// lacks Ed25519 (design D7: the fallback). Public data only: it never handles a private key. It is
// cofactorless like Go's crypto/ed25519 and checked against the same vectors
// (tests/unit/ticket-token.test.ts). SHA-512 comes from WebCrypto, which every browser has.
const B0 = BigInt(0);
const B1 = BigInt(1);
const B2 = BigInt(2);
const B3 = BigInt(3);
const B4 = BigInt(4);
const B8 = BigInt(8);
const B255 = BigInt(255);
const P = B2 ** BigInt(255) - BigInt("19");
const N = B2 ** BigInt(252) + BigInt("27742317777372353535851937790883648493");

function mod(a: bigint, m = P): bigint {
  const r = a % m;
  return r >= B0 ? r : r + m;
}

function pow(b: bigint, e: bigint): bigint {
  let r = B1;
  b = mod(b);
  while (e > B0) {
    if (e & B1) r = mod(r * b);
    b = mod(b * b);
    e >>= B1;
  }
  return r;
}

const inv = (a: bigint) => pow(a, P - B2);
const D = mod(-BigInt("121665") * inv(BigInt("121666")));
const D2 = mod(B2 * D);
const SQRT_M1 = pow(B2, (P - B1) / B4);

type Point = { x: bigint; y: bigint; z: bigint; t: bigint };

const G: Point = (() => {
  const x = BigInt("15112221349535400772501151409588531511454012693041857206046113283949847762202");
  const y = BigInt("46316835694926478169428394003475163141307993866256225615783033603165251855960");
  return { x, y, z: B1, t: mod(x * y) };
})();
const ZERO: Point = { x: B0, y: B1, z: B1, t: B0 };

function add(p: Point, q: Point): Point {
  const a = mod((p.y - p.x) * (q.y - q.x));
  const b = mod((p.y + p.x) * (q.y + q.x));
  const c = mod(p.t * D2 * q.t);
  const d = mod(p.z * B2 * q.z);
  const e = b - a;
  const f = d - c;
  const g = d + c;
  const h = b + a;
  return { x: mod(e * f), y: mod(g * h), z: mod(f * g), t: mod(e * h) };
}

function mul(p: Point, k: bigint): Point {
  let r = ZERO;
  let q = p;
  while (k > B0) {
    if (k & B1) r = add(r, q);
    q = add(q, q);
    k >>= B1;
  }
  return r;
}

function leInt(b: Uint8Array): bigint {
  let n = B0;
  for (let i = b.length - 1; i >= 0; i--) n = (n << B8) | BigInt(b[i]);
  return n;
}

function encode(p: Point): Uint8Array {
  const zi = inv(p.z);
  const x = mod(p.x * zi);
  let y = mod(p.y * zi);
  const out = new Uint8Array(32);
  for (let i = 0; i < 32; i++) {
    out[i] = Number(y & B255);
    y >>= B8;
  }
  if (x & B1) out[31] |= 0x80;
  return out;
}

function decode(b: Uint8Array): Point | null {
  if (b.length !== 32) return null;
  const sign = (b[31] & 0x80) !== 0;
  const yb = b.slice();
  yb[31] &= 0x7f;
  const y = leInt(yb);
  if (y >= P) return null;
  const y2 = mod(y * y);
  const x2 = mod((y2 - B1) * inv(D * y2 + B1));
  let x = pow(x2, (P + B3) / B8);
  if (mod(x * x - x2) !== B0) x = mod(x * SQRT_M1);
  if (mod(x * x - x2) !== B0) return null;
  if (x === B0 && sign) return null;
  if ((x & B1) === B1) {
    if (!sign) x = P - x;
  } else if (sign) x = P - x;
  return { x, y, z: B1, t: mod(x * y) };
}

/** Verifies an Ed25519 signature (64 bytes) of msg with a raw 32-byte public key. */
export async function ed25519Verify(publicKey: Uint8Array, msg: Uint8Array, sig: Uint8Array): Promise<boolean> {
  if (publicKey.length !== 32 || sig.length !== 64) return false;
  const a = decode(publicKey);
  if (!a) return false;
  const rBytes = sig.slice(0, 32);
  const s = leInt(sig.slice(32));
  if (s >= N) return false;
  const buf = new Uint8Array(64 + msg.length);
  buf.set(rBytes, 0);
  buf.set(publicKey, 32);
  buf.set(msg, 64);
  const h = mod(leInt(new Uint8Array(await crypto.subtle.digest("SHA-512", buf))), N);
  const negA: Point = { x: mod(-a.x), y: a.y, z: a.z, t: mod(-a.t) };
  const r = add(mul(G, s), mul(negA, h));
  const enc = encode(r);
  for (let i = 0; i < 32; i++) if (enc[i] !== rBytes[i]) return false;
  return true;
}
