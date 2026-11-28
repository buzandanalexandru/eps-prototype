// The door's offline check (step 7, design D5 to D8): a prepared device holds the public keys, the
// event's manifest, what its feed said (revocations, suspensions, entries) and its clock offset to
// the server, and decides every scan without a network. Pure functions; the door page stores the
// state in IndexedDB.
import { verifyCode, windowOf, type TicketToken } from "../tickets/token";

export type L10n = { ro: string; en: string };

export type TicketInfo = { product: string; gen: number; state: string; name?: string | null; day?: string | null };
export type Entry = { at: number; gate: string; device?: string };

export type DoorState = {
  eventId: string;
  keys: Map<number, Uint8Array>;
  /** Server time minus device time (ms), measured at each sync; null until the first. */
  offsetMs: number | null;
  lastSyncAt: number | null;
  cursor: number;
  tickets: Map<string, TicketInfo>;
  /** Generations below minGen are invalid (a reissue or a cancellation). */
  revoked: Map<string, { minGen: number; reason: string }>;
  suspended: Set<string>;
  admitted: Map<string, Entry>;
  conflicts: { ticketId: string; first: Entry; dup: Entry }[];
  products: Record<string, L10n>;
};

export function emptyState(eventId: string): DoorState {
  return { eventId, keys: new Map(), offsetMs: null, lastSyncAt: null, cursor: 0, tickets: new Map(), revoked: new Map(), suspended: new Set(), admitted: new Map(), conflicts: [], products: {} };
}

export type ScanResult =
  | { ok: true; token: TicketToken; product?: string; name?: string | null; day?: string | null }
  | {
      ok: false;
      reason: "invalid" | "other_event" | "expired" | "not_yet" | "replaced" | "cancelled" | "suspended" | "already";
      token?: TicketToken;
      entry?: Entry;
    };

/** The corrected current time of the device. */
export function correctedNow(state: Pick<DoorState, "offsetMs">, deviceNow: number): number {
  return deviceNow + (state.offsetMs ?? 0);
}

/**
 * Decides a scan offline: the signature with the key of its key id, the event, the time window
 * (the current window and one either side, by the corrected clock), revocations, suspensions and
 * whether this device (or a synced one) already admitted the ticket.
 */
export async function checkScan(code: string, state: DoorState, deviceNow: number, opts: { fallbackOnly?: boolean } = {}): Promise<ScanResult> {
  const v = await verifyCode(code.trim(), state.keys, opts);
  if (!v.ok) return { ok: false, reason: "invalid" };
  const t = v.token;
  if (t.eventId !== state.eventId) return { ok: false, reason: "other_event", token: t };
  const w = windowOf(correctedNow(state, deviceNow));
  if (t.window < w - 1) return { ok: false, reason: "expired", token: t };
  if (t.window > w + 1) return { ok: false, reason: "not_yet", token: t };
  const info = state.tickets.get(t.ticketId);
  const rev = state.revoked.get(t.ticketId);
  if (info?.state === "cancelled" || (rev && (rev.reason === "cancelled" || rev.reason === "erased" || rev.reason.startsWith("order_")))) {
    return { ok: false, reason: "cancelled", token: t };
  }
  const minGen = Math.max(rev?.minGen ?? 1, info?.gen ?? 1);
  if (t.generation < minGen) return { ok: false, reason: "replaced", token: t };
  if (state.suspended.has(t.ticketId) || info?.state === "suspended") return { ok: false, reason: "suspended", token: t };
  const entry = state.admitted.get(t.ticketId);
  if (entry) return { ok: false, reason: "already", token: t, entry };
  return { ok: true, token: t, product: info?.product, name: info?.name ?? null, day: info?.day ?? null };
}

/** Records a local admission (the device writes it first, then uploads it). */
export function admit(state: DoorState, ticketId: string, entry: Entry): void {
  if (!state.admitted.has(ticketId)) state.admitted.set(ticketId, entry);
}

export type ManifestItem = { t: string; p: string; g: number; s: string; n?: string | null; d?: string | null; ea?: string | null; eg?: string | null };
export type FeedRow = { seq: number; kind: string; ticketId: string; generation?: number | null; data: Record<string, unknown>; at: string };

export function applyManifest(state: DoorState, items: ManifestItem[]): void {
  for (const m of items) {
    state.tickets.set(m.t, { product: m.p, gen: m.g, state: m.s, name: m.n ?? null, day: m.d ?? null });
    if (m.s === "suspended") state.suspended.add(m.t);
    else state.suspended.delete(m.t);
    if (m.ea && !state.admitted.has(m.t)) state.admitted.set(m.t, { at: Date.parse(m.ea), gate: m.eg ?? "" });
  }
}

function entryOf(v: unknown): Entry | null {
  if (!v || typeof v !== "object") return null;
  const o = v as Record<string, unknown>;
  const at = typeof o.at === "string" ? Date.parse(o.at) : NaN;
  return Number.isFinite(at) ? { at, gate: String(o.gate ?? ""), device: typeof o.device === "string" ? o.device : undefined } : null;
}

/** Applies feed rows in sequence order (rows at or below the cursor are skipped). */
export function applyFeed(state: DoorState, rows: FeedRow[]): void {
  for (const r of [...rows].sort((a, b) => a.seq - b.seq)) {
    if (r.seq <= state.cursor) continue;
    state.cursor = r.seq;
    const info = state.tickets.get(r.ticketId);
    switch (r.kind) {
      case "revoked": {
        const reason = String(r.data?.reason ?? "replaced");
        const gen = r.generation ?? 1;
        const prev = state.revoked.get(r.ticketId);
        state.revoked.set(r.ticketId, { minGen: Math.max(gen, prev?.minGen ?? 1), reason });
        if (info) {
          info.gen = Math.max(info.gen, gen);
          if (reason === "cancelled" || reason === "erased" || reason.startsWith("order_")) info.state = "cancelled";
          if (reason === "erased") info.name = null;
          if (typeof r.data?.day === "string") info.day = r.data.day;
        }
        break;
      }
      case "suspended":
        state.suspended.add(r.ticketId);
        break;
      case "lifted":
        state.suspended.delete(r.ticketId);
        if (info && info.state === "suspended") info.state = "valid";
        break;
      case "entry": {
        const e = entryOf(r.data);
        const cur = state.admitted.get(r.ticketId);
        if (e && (!cur || e.at < cur.at)) state.admitted.set(r.ticketId, e);
        break;
      }
      case "conflict": {
        const first = entryOf(r.data?.first);
        const dup = entryOf(r.data?.dup);
        if (first && dup) {
          state.conflicts.push({ ticketId: r.ticketId, first, dup });
          state.admitted.set(r.ticketId, first);
        }
        break;
      }
      case "manifest":
        if (info) {
          if (typeof r.data?.name === "string") info.name = r.data.name;
          if (typeof r.data?.day === "string") info.day = r.data.day;
        }
        break;
    }
  }
}

/** Records a sync: the clock offset from the server's time (half the round trip) and its moment. */
export function recordSync(state: DoorState, serverTime: string, sentAt: number, receivedAt: number): void {
  const server = Date.parse(serverTime);
  if (!Number.isFinite(server)) return;
  state.offsetMs = server - (sentAt + receivedAt) / 2;
  state.lastSyncAt = receivedAt;
}

export const STALE_AFTER_MS = 10 * 60 * 1000;

/** How long ago the device last synced, and whether that is stale (amber after 10 minutes). */
export function syncAge(state: Pick<DoorState, "lastSyncAt" | "offsetMs">, deviceNow: number): { minutes: number | null; stale: boolean; neverClock: boolean } {
  if (state.lastSyncAt === null) return { minutes: null, stale: true, neverClock: state.offsetMs === null };
  const ms = deviceNow - state.lastSyncAt;
  return { minutes: Math.max(0, Math.floor(ms / 60000)), stale: ms > STALE_AFTER_MS, neverClock: state.offsetMs === null };
}
