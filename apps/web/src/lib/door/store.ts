// The door device's offline store (IndexedDB, design D6/D7): one record per event with the keys,
// manifest, feed state, clock offset, the device id and gate, scans waiting to upload and the log.
import { emptyState, type DoorState, type Entry, type TicketInfo } from "./verify";

export type PendingScan = { id: string; ticketId: string; generation: number; scannedAt: string };
export type LogLine = { at: number; ok: boolean; reason: string; product?: string; name?: string | null };
export type Device = { deviceId: string; gate: string; prepared: boolean };

type Stored = {
  eventId: string;
  device: Device;
  keys: [number, number[]][];
  offsetMs: number | null;
  lastSyncAt: number | null;
  cursor: number;
  tickets: [string, TicketInfo][];
  revoked: [string, { minGen: number; reason: string }][];
  suspended: string[];
  admitted: [string, Entry][];
  conflicts: DoorState["conflicts"];
  products: DoorState["products"];
  pending: PendingScan[];
  log: LogLine[];
};

const DB = "eps-door";
const STORE = "events";

function open(): Promise<IDBDatabase> {
  return new Promise((resolve, reject) => {
    const r = indexedDB.open(DB, 1);
    r.onupgradeneeded = () => r.result.createObjectStore(STORE, { keyPath: "eventId" });
    r.onsuccess = () => resolve(r.result);
    r.onerror = () => reject(r.error);
  });
}

export type Loaded = { state: DoorState; device: Device; pending: PendingScan[]; log: LogLine[] };

export async function load(eventId: string): Promise<Loaded | null> {
  const db = await open();
  const rec = await new Promise<Stored | undefined>((resolve, reject) => {
    const r = db.transaction(STORE).objectStore(STORE).get(eventId);
    r.onsuccess = () => resolve(r.result as Stored | undefined);
    r.onerror = () => reject(r.error);
  });
  db.close();
  if (!rec) return null;
  const state = emptyState(eventId);
  state.keys = new Map(rec.keys.map(([k, v]) => [k, new Uint8Array(v)]));
  state.offsetMs = rec.offsetMs;
  state.lastSyncAt = rec.lastSyncAt;
  state.cursor = rec.cursor;
  state.tickets = new Map(rec.tickets);
  state.revoked = new Map(rec.revoked);
  state.suspended = new Set(rec.suspended);
  state.admitted = new Map(rec.admitted);
  state.conflicts = rec.conflicts;
  state.products = rec.products;
  return { state, device: rec.device, pending: rec.pending, log: rec.log };
}

export async function save(l: Loaded): Promise<void> {
  const s = l.state;
  const rec: Stored = {
    eventId: s.eventId,
    device: l.device,
    keys: [...s.keys].map(([k, v]) => [k, Array.from(v)]),
    offsetMs: s.offsetMs,
    lastSyncAt: s.lastSyncAt,
    cursor: s.cursor,
    tickets: [...s.tickets],
    revoked: [...s.revoked],
    suspended: [...s.suspended],
    admitted: [...s.admitted],
    conflicts: s.conflicts.slice(-200),
    products: s.products,
    pending: l.pending,
    log: l.log.slice(-50),
  };
  const db = await open();
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE, "readwrite");
    tx.objectStore(STORE).put(rec);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
  db.close();
}

export async function forget(eventId: string): Promise<void> {
  const db = await open();
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(STORE, "readwrite");
    tx.objectStore(STORE).delete(eventId);
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
  db.close();
}
