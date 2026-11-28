// Door scanning spec: every offline decision of a prepared door device (lib/door/verify.ts), with
// codes signed here by a test key the way the core signs them (design D2).
import { generateKeyPairSync, sign } from "node:crypto";
import { beforeEach, describe, expect, it } from "vitest";
import { admit, applyFeed, applyManifest, checkScan, emptyState, recordSync, syncAge, type DoorState } from "@/lib/door/verify";
import { fromBase64Url, windowOf } from "@/lib/tickets/token";

const EVENT = "1b4e28ba-2fa1-4d2a-883f-0016d3cca427";
const OTHER_EVENT = "2c5f39cb-3fb2-4e3b-994f-1127e4ddb538";
const TICKET = "6f1c1a3e-9b7a-4c55-b1b0-3d1f2e4a5b6c";
const { publicKey, privateKey } = generateKeyPairSync("ed25519");
const PUB = fromBase64Url(publicKey.export({ format: "jwk" }).x!)!;
const other = generateKeyPairSync("ed25519");

function uuidBytes(u: string) {
  return Buffer.from(u.replace(/-/g, ""), "hex");
}

function code(opts: { window: number; event?: string; ticket?: string; gen?: number; keyId?: number; key?: typeof privateKey }): string {
  const b = Buffer.alloc(40);
  b[0] = 1;
  b[1] = opts.keyId ?? 1;
  uuidBytes(opts.event ?? EVENT).copy(b, 2);
  uuidBytes(opts.ticket ?? TICKET).copy(b, 18);
  b.writeUInt16BE(opts.gen ?? 1, 34);
  b.writeUInt32BE(opts.window, 36);
  const sig = sign(null, b, opts.key ?? privateKey);
  return "E1." + Buffer.concat([b, sig]).toString("base64url");
}

const NOW = Date.parse("2027-07-15T21:04:00Z");
const W = windowOf(NOW);
let st: DoorState;

beforeEach(() => {
  st = emptyState(EVENT);
  st.keys.set(1, PUB);
  recordSync(st, new Date(NOW).toISOString(), NOW, NOW);
  applyManifest(st, [{ t: TICKET, p: "ga", g: 1, s: "valid", n: "Ana Pop" }]);
});

describe("Codes are verified offline", () => {
  it("accepts the live code, with the product and the holder's name", async () => {
    const r = await checkScan(code({ window: W }), st, NOW);
    expect(r).toMatchObject({ ok: true, product: "ga", name: "Ana Pop" });
  });
  it("Screenshot from a minute ago: two or more windows before is expired", async () => {
    expect(await checkScan(code({ window: W - 4 }), st, NOW)).toMatchObject({ ok: false, reason: "expired" });
    expect(await checkScan(code({ window: W - 2 }), st, NOW)).toMatchObject({ ok: false, reason: "expired" });
    expect(await checkScan(code({ window: W - 1 }), st, NOW)).toMatchObject({ ok: true });
  });
  it("Phone clock slightly fast: the next window is accepted", async () => {
    expect(await checkScan(code({ window: W + 1 }), st, NOW)).toMatchObject({ ok: true });
  });
  it("Code from further in the future: rejected as not yet valid", async () => {
    expect(await checkScan(code({ window: W + 2 }), st, NOW)).toMatchObject({ ok: false, reason: "not_yet" });
  });
  it("Ticket for another event", async () => {
    expect(await checkScan(code({ window: W, event: OTHER_EVENT }), st, NOW)).toMatchObject({ ok: false, reason: "other_event" });
  });
  it("Forged or altered code: bad signature or unknown key id", async () => {
    expect(await checkScan(code({ window: W, key: other.privateKey }), st, NOW)).toMatchObject({ ok: false, reason: "invalid" });
    expect(await checkScan(code({ window: W, keyId: 9 }), st, NOW)).toMatchObject({ ok: false, reason: "invalid" });
    const c = code({ window: W });
    const bad = c.slice(0, 50) + (c[50] === "A" ? "B" : "A") + c.slice(51);
    expect(await checkScan(bad, st, NOW)).toMatchObject({ ok: false, reason: "invalid" });
    expect(await checkScan("not a ticket", st, NOW)).toMatchObject({ ok: false, reason: "invalid" });
  });
  it("Reissued ticket: a synced revocation of generation 1", async () => {
    applyFeed(st, [{ seq: 1, kind: "revoked", ticketId: TICKET, generation: 2, data: { reason: "transferred" }, at: "" }]);
    expect(await checkScan(code({ window: W, gen: 1 }), st, NOW)).toMatchObject({ ok: false, reason: "replaced" });
    expect(await checkScan(code({ window: W, gen: 2 }), st, NOW)).toMatchObject({ ok: true });
  });
  it("Suspended ticket, and its lifting", async () => {
    applyFeed(st, [{ seq: 1, kind: "suspended", ticketId: TICKET, generation: 1, data: { reason: "instalment" }, at: "" }]);
    expect(await checkScan(code({ window: W }), st, NOW)).toMatchObject({ ok: false, reason: "suspended" });
    applyFeed(st, [{ seq: 2, kind: "lifted", ticketId: TICKET, generation: 1, data: {}, at: "" }]);
    expect(await checkScan(code({ window: W }), st, NOW)).toMatchObject({ ok: true });
  });
  it("a cancelled ticket is refused", async () => {
    applyFeed(st, [{ seq: 1, kind: "revoked", ticketId: TICKET, generation: 2, data: { reason: "cancelled" }, at: "" }]);
    expect(await checkScan(code({ window: W, gen: 2 }), st, NOW)).toMatchObject({ ok: false, reason: "cancelled" });
  });
});

describe("One entry per ticket", () => {
  it("Same ticket twice on one device: the time and gate of the first entry", async () => {
    const first = await checkScan(code({ window: W }), st, NOW);
    expect(first.ok).toBe(true);
    admit(st, TICKET, { at: NOW, gate: "Gate B" });
    const again = await checkScan(code({ window: W + 1 }), st, NOW + 15_000);
    expect(again).toMatchObject({ ok: false, reason: "already", entry: { at: NOW, gate: "Gate B" } });
  });
  it("an entry synced from another device counts too", async () => {
    applyFeed(st, [{ seq: 1, kind: "entry", ticketId: TICKET, generation: 1, data: { at: new Date(NOW - 60_000).toISOString(), gate: "Gate A" }, at: "" }]);
    expect(await checkScan(code({ window: W }), st, NOW)).toMatchObject({ ok: false, reason: "already", entry: { gate: "Gate A" } });
  });
});

describe("Clock correction and sync state", () => {
  it("Door phone clock is two minutes slow: after a sync the live code is accepted", async () => {
    const slow = NOW - 120_000;
    const s2 = emptyState(EVENT);
    s2.keys.set(1, PUB);
    expect(await checkScan(code({ window: W }), s2, slow)).toMatchObject({ ok: false, reason: "not_yet" });
    recordSync(s2, new Date(NOW).toISOString(), slow - 100, slow + 100);
    expect(await checkScan(code({ window: W }), s2, slow)).toMatchObject({ ok: true });
  });
  it("Stale sync warning after 10 minutes; a device that never measured its clock is flagged", () => {
    expect(syncAge(st, NOW + 9 * 60_000)).toMatchObject({ minutes: 9, stale: false });
    expect(syncAge(st, NOW + 11 * 60_000)).toMatchObject({ minutes: 11, stale: true });
    expect(syncAge(emptyState(EVENT), NOW)).toMatchObject({ minutes: null, stale: true, neverClock: true });
  });
  it("feed rows apply once, in order", () => {
    applyFeed(st, [
      { seq: 2, kind: "manifest", ticketId: TICKET, generation: 1, data: { name: "Ion Pop" }, at: "" },
      { seq: 1, kind: "manifest", ticketId: TICKET, generation: 1, data: { name: "Old" }, at: "" },
    ]);
    expect(st.tickets.get(TICKET)?.name).toBe("Ion Pop");
    applyFeed(st, [{ seq: 1, kind: "manifest", ticketId: TICKET, generation: 1, data: { name: "Replay" }, at: "" }]);
    expect(st.tickets.get(TICKET)?.name).toBe("Ion Pop");
    expect(st.cursor).toBe(2);
  });
});
