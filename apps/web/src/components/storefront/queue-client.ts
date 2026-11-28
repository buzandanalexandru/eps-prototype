// The waiting room's browser side (step 5, design D-2 and D-4): the session secret a pass is bound
// to (kept in localStorage, so every tab of this browser shares one place), the queue token and
// the pass per sale and event, and the proof of work the join sends: SHA-256 of
// "<sale>:<sha256(session) hex>:<nonce>" with 16 leading zero bits (about 50 ms on a phone).

const SESSION_KEY = "eps-session";

let memory: Record<string, string> = {};

function get(k: string): string | null {
  try {
    return window.localStorage.getItem(k);
  } catch {
    return memory[k] ?? null;
  }
}

function set(k: string, v: string | null) {
  try {
    if (v === null) window.localStorage.removeItem(k);
    else window.localStorage.setItem(k, v);
  } catch {
    if (v === null) delete memory[k];
    else memory = { ...memory, [k]: v };
  }
}

function hex(bytes: Uint8Array): string {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

/** This browser's session secret (created once). */
export function sessionSecret(): string {
  let s = get(SESSION_KEY);
  if (!s || !/^[0-9a-f]{64}$/.test(s)) {
    const b = new Uint8Array(32);
    crypto.getRandomValues(b);
    s = hex(b);
    set(SESSION_KEY, s);
  }
  return s;
}

export function queueToken(saleId: string): string | null {
  return get(`eps-qt:${saleId}`);
}

export function saveQueueToken(saleId: string, token: string | null) {
  set(`eps-qt:${saleId}`, token);
}

type StoredPass = { pass: string; exp: number };

/** The pass this browser holds for an event, while it lasts. */
export function passFor(eventId: string): string | null {
  const raw = get(`eps-pass:${eventId}`);
  if (!raw) return null;
  try {
    const p = JSON.parse(raw) as StoredPass;
    return typeof p.pass === "string" ? p.pass : null;
  } catch {
    return null;
  }
}

export function savePass(eventId: string, pass: string, exp: number) {
  set(`eps-pass:${eventId}`, JSON.stringify({ pass, exp } satisfies StoredPass));
}

export function clearPass(eventId: string) {
  set(`eps-pass:${eventId}`, null);
}

/** The headers checkout calls carry for an event (nothing when no pass is held). */
export function passHeaders(eventId: string | undefined): Record<string, string> {
  if (!eventId) return {};
  const pass = passFor(eventId);
  return pass ? { "X-EPS-Pass": pass, "X-EPS-Session": sessionSecret() } : { "X-EPS-Session": sessionSecret() };
}

// A small synchronous SHA-256 (the proof of work makes tens of thousands of hashes; WebCrypto's
// promise per hash would be far slower).
const K = new Uint32Array([
  0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5, 0xd807aa98, 0x12835b01, 0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe,
  0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc, 0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da, 0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7,
  0xc6e00bf3, 0xd5a79147, 0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85, 0xa2bfe8a1, 0xa81a664b,
  0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070, 0x19a4c116, 0x1e376c08, 0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3,
  0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208, 0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
]);

export function sha256(msg: Uint8Array): Uint8Array {
  const len = msg.length;
  const total = Math.ceil((len + 9) / 64) * 64;
  const buf = new Uint8Array(total);
  buf.set(msg);
  buf[len] = 0x80;
  const bits = len * 8;
  const view = new DataView(buf.buffer);
  view.setUint32(total - 4, bits >>> 0);
  view.setUint32(total - 8, Math.floor(bits / 0x100000000));
  const h = new Uint32Array([0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19]);
  const w = new Uint32Array(64);
  for (let off = 0; off < total; off += 64) {
    for (let i = 0; i < 16; i++) w[i] = view.getUint32(off + i * 4);
    for (let i = 16; i < 64; i++) {
      const a = w[i - 15], b = w[i - 2];
      const s0 = ((a >>> 7) | (a << 25)) ^ ((a >>> 18) | (a << 14)) ^ (a >>> 3);
      const s1 = ((b >>> 17) | (b << 15)) ^ ((b >>> 19) | (b << 13)) ^ (b >>> 10);
      w[i] = (w[i - 16] + s0 + w[i - 7] + s1) >>> 0;
    }
    let [a, b, c, d, e, f, g, hh] = h;
    for (let i = 0; i < 64; i++) {
      const S1 = ((e >>> 6) | (e << 26)) ^ ((e >>> 11) | (e << 21)) ^ ((e >>> 25) | (e << 7));
      const ch = (e & f) ^ (~e & g);
      const t1 = (hh + S1 + ch + K[i] + w[i]) >>> 0;
      const S0 = ((a >>> 2) | (a << 30)) ^ ((a >>> 13) | (a << 19)) ^ ((a >>> 22) | (a << 10));
      const maj = (a & b) ^ (a & c) ^ (b & c);
      const t2 = (S0 + maj) >>> 0;
      hh = g;
      g = f;
      f = e;
      e = (d + t1) >>> 0;
      d = c;
      c = b;
      b = a;
      a = (t1 + t2) >>> 0;
    }
    h[0] = (h[0] + a) >>> 0;
    h[1] = (h[1] + b) >>> 0;
    h[2] = (h[2] + c) >>> 0;
    h[3] = (h[3] + d) >>> 0;
    h[4] = (h[4] + e) >>> 0;
    h[5] = (h[5] + f) >>> 0;
    h[6] = (h[6] + g) >>> 0;
    h[7] = (h[7] + hh) >>> 0;
  }
  const out = new Uint8Array(32);
  const ov = new DataView(out.buffer);
  for (let i = 0; i < 8; i++) ov.setUint32(i * 4, h[i]);
  return out;
}

const enc = new TextEncoder();

export function sha256Hex(s: string): string {
  return hex(sha256(enc.encode(s)));
}

/** The nonce whose hash with this sale and session starts with 16 zero bits. */
export function solvePow(saleId: string, sid: string): string {
  const prefix = `${saleId}:${sid}:`;
  for (let i = 0; ; i++) {
    const d = sha256(enc.encode(prefix + i));
    if (d[0] === 0 && d[1] === 0) return String(i);
  }
}

/** A coarse browser print (hashed; the gateway only counts how many people share one). */
export function browserPrint(): string {
  const n = navigator;
  const parts = [n.userAgent, n.language, String(n.hardwareConcurrency ?? ""), `${screen.width}x${screen.height}x${screen.colorDepth}`, Intl.DateTimeFormat().resolvedOptions().timeZone];
  return sha256Hex(parts.join("|")).slice(0, 32);
}
